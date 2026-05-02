package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentorbit-tech/agentorbit/processing/internal/config"
	"github.com/agentorbit-tech/agentorbit/processing/internal/cron"
	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
	"github.com/agentorbit-tech/agentorbit/processing/internal/errtrack"
	"github.com/agentorbit-tech/agentorbit/processing/internal/handler"
	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/llm"
	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/migrations"
	processingWeb "github.com/agentorbit-tech/agentorbit/processing/web"
)

func main() {
	// Initialize structured logging (JSON for production, per D-19)
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo)
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		if err := logLevel.UnmarshalText([]byte(lvl)); err != nil {
			slog.Error("invalid LOG_LEVEL, using INFO", "value", lvl, "error", err)
		}
	}
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       &logLevel,
		ReplaceAttr: errtrack.SlogReplaceAttr,
	})
	slog.SetDefault(slog.New(logHandler))

	cfg, err := config.LoadProcessing()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := errtrack.Init(errtrack.Config{
		DSN:         cfg.SentryDSN,
		Environment: cfg.SentryEnvironment,
		Release:     cfg.SentryRelease,
		Service:     "processing",
		SampleRate:  cfg.SentrySampleRate,
	}); err != nil {
		slog.Error("sentry init failed, continuing without error tracking", "error", err)
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		errtrack.Capture(err, errtrack.Fields{"component": "startup", "stage": "db_config"})
		errtrack.Flush(2 * time.Second)
		slog.Error("database config failed", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	poolCfg.ConnConfig.RuntimeParams["statement_timeout"] = "30000" // 30s, ms units

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	pool, err := pgxpool.NewWithConfig(dbCtx, poolCfg)
	if err != nil {
		errtrack.Capture(err, errtrack.Fields{"component": "startup", "stage": "db_connect"})
		errtrack.Flush(2 * time.Second)
		slog.Error("database connection failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: fatal startup error, no cleanup needed
	}
	defer pool.Close()

	// Verify migrations have been applied externally (SP-3 #1).
	// Migrations now run via the dedicated agentorbit-migrate executable, not at
	// boot — this prevents bad migrations from turning every replica into a
	// crash-loop. The check below ensures the schema is at least as new as the
	// embedded migration set; refuse to start otherwise.
	if err := verifyMigrationsApplied(dbCtx, pool); err != nil {
		errtrack.Capture(err, errtrack.Fields{"component": "startup", "stage": "migration_check"})
		errtrack.Flush(2 * time.Second)
		slog.Error("migration check failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: fatal startup error, no cleanup needed
	}

	// Dedicated pool for CSV export queries (SP-1 Task 6). Long-running export
	// queries get a 180s statement_timeout vs the default 30s, isolated from the
	// main pool so they cannot starve dashboard reads.
	exportPoolCfg := poolCfg.Copy()
	exportPoolCfg.MaxConns = 3
	exportPoolCfg.MinConns = 0
	if exportPoolCfg.ConnConfig.RuntimeParams == nil {
		exportPoolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	exportPoolCfg.ConnConfig.RuntimeParams["statement_timeout"] = "180000"
	exportPool, err := pgxpool.NewWithConfig(dbCtx, exportPoolCfg)
	if err != nil {
		errtrack.Capture(err, errtrack.Fields{"component": "startup", "stage": "export_db_connect"})
		errtrack.Flush(2 * time.Second)
		slog.Error("export database pool failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: fatal startup error, no cleanup needed
	}
	defer exportPool.Close()

	queries := db.New(pool)

	mailer, err := email.NewMailer(email.MailConfig{
		SMTPHost:   cfg.SMTPHost,
		SMTPPort:   cfg.SMTPPort,
		SMTPUser:   cfg.SMTPUser,
		SMTPPass:   cfg.SMTPPass,
		SMTPFrom:   cfg.SMTPFrom,
		AppBaseURL: cfg.AppBaseURL,
	})
	if err != nil {
		slog.Error("mailer initialization failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: fatal startup error, no cleanup needed
	}

	// Cancellable context for graceful shutdown — created before services that need it (H-1).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Hub (real-time event pub/sub)
	eventHub := hub.New()

	// Services
	authService := service.NewAuthService(ctx, queries, pool, mailer, cfg.JWTSecret, cfg.HMACSecret, cfg.DeploymentMode, cfg.JWTTTLDuration(), cfg.SkipEmailVerification)
	orgService := service.NewOrgService(queries, pool, mailer, cfg.DeploymentMode)
	inviteService := service.NewInviteService(queries, pool, mailer)
	apiKeyService := service.NewAPIKeyService(queries, cfg.HMACSecret, cfg.EncryptionKey)
	internalService := service.NewInternalService(queries, pool, cfg.HMACSecret, cfg.EncryptionKey, eventHub)
	internalService.Start(ctx)
	dashboardService := service.NewDashboardService(queries, pool, exportPool, cfg.ExportRowLimit)
	alertService := service.NewAlertService(queries, pool, eventHub, mailer, cfg.AppBaseURL)

	// LLM client for intelligence pipeline (nil when PROCESSING_LLM_BASE_URL is empty)
	llmClient := llm.NewClient(cfg.LLMAPIKey, cfg.LLMModel, cfg.LLMBaseURL)
	intelligenceService := service.NewIntelligenceService(queries, pool, llmClient, eventHub)
	internalService.SetIntelligenceService(intelligenceService)
	apiKeyService.SetInternalService(internalService)
	if llmClient != nil {
		slog.Info("intelligence: LLM configured", "base_url", cfg.LLMBaseURL, "model", cfg.LLMModel)
	} else {
		slog.Info("intelligence: LLM not configured, using metadata summaries and deterministic clustering")
	}

	// Handlers
	emailRateLimiter := middleware.NewEmailRateLimiter(ctx, 3, 1*time.Hour)
	authHandler := handler.NewAuthHandler(authService, mailer, cfg.AppBaseURL, cfg.CookieDomain, cfg.JWTTTLDuration(), emailRateLimiter, queries)
	orgHandler := handler.NewOrgHandler(orgService)
	inviteHandler := handler.NewInviteHandler(inviteService, queries, mailer)
	apiKeyHandler := handler.NewAPIKeyHandler(apiKeyService, middleware.RequireActiveOrg(), middleware.RequireRole)
	internalHandler := handler.NewInternalHandler(internalService)
	dashboardHandler := handler.NewDashboardHandler(dashboardService)
	wsHandler := handler.NewWSHandler(eventHub, cfg.JWTSecret, queries, cfg.AllowedOrigins)
	userHandler := handler.NewUserHandler(authService)
	alertHandler := handler.NewAlertHandler(alertService)

	// Router
	r := chi.NewRouter()

	// Global middleware. RequestID first so the access log carries a correlation
	// ID. SlogAccess wraps errtrack so panic-recovered requests still produce an
	// access log line with status=500 (errtrack writes the response code, then
	// the deferred slog.Info in SlogAccess fires on unwind).
	r.Use(chiMiddleware.RequestID)
	r.Use(middleware.SlogAccess)
	r.Use(errtrack.Middleware)
	r.Use(middleware.SecurityHeaders(cfg.BillingURL))
	r.Use(middleware.MaxBodySize(1 << 20)) // 1MB default limit for all endpoints
	if cfg.AllowedOrigins != "" {
		r.Use(middleware.CORS(cfg.AllowedOrigins))
	}

	// Liveness: unconditional 200 so container runtimes do not kill us during DB blips.
	// Readiness: verifies database connectivity, used by load balancers to drain.
	r.Get("/health", handler.NewLivenessHandler())
	r.Get("/readyz", handler.NewReadinessHandler(pool))

	// Deployment metadata — public, surfaces APP_VERSION in the UI footer
	r.Get("/meta", handler.NewMetaHandler(cfg.AppVersion, cfg.BillingURL))

	// Public auth routes — no authentication required, rate-limited
	// Tightened to 5 req/min/IP (SP-2 #4) to slow brute-force at the edge
	// while the per-process bcrypt semaphore handles capacity at the service.
	authRateLimiter := middleware.NewRateLimiter(ctx, 5, 1*time.Minute, cfg.TrustedProxies)
	r.With(authRateLimiter.Middleware, middleware.RequireXHR).Mount("/auth", authHandler.Routes())

	// Invite acceptance — authenticated user, not org-scoped, rate-limited
	r.With(authRateLimiter.Middleware, middleware.RequireXHR, middleware.Authenticate(cfg.JWTSecret, cfg.HMACSecret, queries)).
		Post("/auth/accept-invite", inviteHandler.AcceptInvite)

	// Dashboard API — JWT or API-key authentication
	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.WithDeadline(10 * time.Second))
		r.Use(middleware.Authenticate(cfg.JWTSecret, cfg.HMACSecret, queries))
		r.Use(middleware.RequireXHR)

		// User profile routes — authenticated, not org-scoped
		r.Route("/user", func(r chi.Router) {
			r.Get("/me", userHandler.GetMe)
			r.Put("/profile", userHandler.UpdateProfile)
			r.Put("/password", userHandler.ChangePassword)
		})

		r.Route("/orgs", func(r chi.Router) {
			// Org creation and listing — no org context needed
			r.Post("/", orgHandler.Create)
			r.Get("/", orgHandler.List)

			// Org-scoped routes — RequireOrg verifies membership and loads org context
			r.Route("/{orgID}", func(r chi.Router) {
				r.Use(middleware.RequireOrg(queries))
				r.Get("/", orgHandler.Get)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Put("/settings", orgHandler.UpdateSettings)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Get("/privacy-settings", orgHandler.GetPrivacySettings)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Put("/privacy-settings", orgHandler.UpdatePrivacySettings)
				r.Get("/spans/{spanID}/masking-maps", orgHandler.GetSpanMaskingMaps)
				r.With(middleware.RequireRole("owner")).
					Delete("/", orgHandler.InitiateDeletion)
				r.With(middleware.RequireRole("owner")).
					Post("/restore", orgHandler.CancelDeletion)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner")).
					Post("/transfer", orgHandler.TransferOwnership)
				r.With(middleware.RequireActiveOrg()).
					Post("/leave", orgHandler.Leave)
				r.Get("/members", orgHandler.ListMembers)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Put("/members/{memberID}/role", orgHandler.UpdateMemberRole)
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Delete("/members/{memberID}", orgHandler.RemoveMember)
				r.Mount("/invites", inviteHandler.Routes())
				r.Mount("/keys", apiKeyHandler.Routes())
				r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).
					Mount("/alerts", alertHandler.Routes())
				r.Mount("/", dashboardHandler.Routes())
			})
		})
	})

	// Internal API — X-Internal-Token authentication (Proxy -> Processing)
	// Override body limit to 3MB for span ingest payloads.
	// Optional IP allowlist via INTERNAL_ALLOWED_IPS env var.
	dbBreakerProvider := middleware.NewPgxAdapter(func() (acquired, max int32) {
		st := pool.Stat()
		return st.AcquiredConns(), st.MaxConns()
	})
	r.Route("/internal", func(r chi.Router) {
		r.Use(middleware.RequireInternalIP(cfg.InternalAllowedIPs))
		r.Use(middleware.RequireInternalToken(cfg.InternalToken))
		r.Use(middleware.MaxBodySize(3 << 20)) // 3MB for span ingest
		// /spans/ingest gets per-key rate limit + DB pool circuit breaker
		// so a runaway key or pool saturation cannot starve other endpoints.
		r.With(
			middleware.PerKeyIngestRateLimit(ctx, 600, 1*time.Minute),
			middleware.DBPoolBreaker(dbBreakerProvider, 0.8),
		).Post("/spans/ingest", internalHandler.Ingest)
		r.Post("/auth/verify", internalHandler.Verify)
		// Debug endpoints — only available when built with -tags pprof (D-09).
		// Reuse InternalHandler.Routes() exclusively for the pprof mount; we
		// duplicate the explicit Verify/Ingest registrations above with the
		// per-route middleware.
		r.Mount("/_pprof", internalHandler.PprofRoutes())
	})

	// WebSocket endpoint — mounted before SPA catch-all so /cable is not intercepted (WS-01)
	// Rate-limited to mitigate connection-exhaustion attacks (H-12).
	wsRateLimiter := middleware.NewRateLimiter(ctx, 10, 1*time.Minute, cfg.TrustedProxies)
	r.With(wsRateLimiter.Middleware).Get("/cable", wsHandler.ServeHTTP)

	// SPA handler — serves the embedded React frontend for all non-API paths
	spaHandler, err := processingWeb.NewSPAHandler(cfg.BillingURL)
	if err != nil {
		slog.Warn("SPA handler unavailable, serving API only", "error", err)
	} else {
		r.NotFound(spaHandler.ServeHTTP)
	}

	// Cron busy-flags: each cron has its own atomic.Bool guarding against
	// overlapping ticks within this process. Cross-replica overlap is prevented
	// by cron.WithAdvisoryLock.
	var (
		hardDeleteBusy      atomic.Bool
		sessionCloserBusy   atomic.Bool
		alertEvaluationBusy atomic.Bool
		retentionPurgeBusy  atomic.Bool
	)

	// Hard-delete cron: permanently removes organizations past their 14-day grace period (ORG-08, D-12)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("worker panic", "error", r, "worker", "hard_delete_cron")
				errtrack.CapturePanic(r, errtrack.Fields{"component": "worker", "worker": "hard_delete_cron"})
			}
		}()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cron.Singleflight("hard_delete_cron", &hardDeleteBusy, func() {
					if err := cron.WithAdvisoryLock(ctx, pool, cron.LockHardDelete, func(ctx context.Context) error {
						return orgService.RunHardDeleteCron(ctx)
					}); err != nil {
						slog.Error("hard-delete cron error", "error", err)
						errtrack.Capture(err, errtrack.Fields{"component": "worker", "worker": "hard_delete_cron"})
					}
				})
			case <-ctx.Done():
				return
			}
		}
	}()

	// Session closure cron: closes idle sessions and sets terminal status (SESS-04, D-09)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("worker panic", "error", r, "worker", "session_closer")
				errtrack.CapturePanic(r, errtrack.Fields{"component": "worker", "worker": "session_closer"})
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cron.Singleflight("session_closer", &sessionCloserBusy, func() {
					if err := cron.WithAdvisoryLock(ctx, pool, cron.LockSessionClosure, func(ctx context.Context) error {
						return internalService.RunSessionClosureCron(ctx)
					}); err != nil {
						slog.Error("session-closure cron error", "error", err)
						errtrack.Capture(err, errtrack.Fields{"component": "worker", "worker": "session_closer"})
					}
				})
			case <-ctx.Done():
				return
			}
		}
	}()

	// Alert evaluation cron: evaluates alert rules every 60 seconds (ALRT-02)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("worker panic", "error", r, "worker", "alert_evaluation")
				errtrack.CapturePanic(r, errtrack.Fields{"component": "worker", "worker": "alert_evaluation"})
			}
		}()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cron.Singleflight("alert_evaluation", &alertEvaluationBusy, func() {
					if err := cron.WithAdvisoryLock(ctx, pool, cron.LockAlertEvaluation, func(ctx context.Context) error {
						return alertService.RunEvaluationCron(ctx)
					}); err != nil {
						slog.Error("alert-evaluation cron error", "error", err)
						errtrack.Capture(err, errtrack.Fields{"component": "worker", "worker": "alert_evaluation"})
					}
				})
			case <-ctx.Done():
				return
			}
		}
	}()

	// Data retention cron: purges old spans, sessions, and alert events daily
	if cfg.DataRetentionDays > 0 {
		slog.Info("data retention enabled", "days", cfg.DataRetentionDays)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("worker panic", "error", r, "worker", "retention_purge")
					errtrack.CapturePanic(r, errtrack.Fields{"component": "worker", "worker": "retention_purge"})
				}
			}()
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					cron.Singleflight("retention_purge", &retentionPurgeBusy, func() {
						if err := cron.WithAdvisoryLock(ctx, pool, cron.LockRetentionPurge, func(ctx context.Context) error {
							_, err := service.RunRetentionPurge(ctx, queries, cfg.DataRetentionDays)
							return err
						}); err != nil {
							slog.Error("retention purge error", "error", err)
							errtrack.Capture(err, errtrack.Fields{"component": "worker", "worker": "retention_purge"})
						}
					})
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Reactive alert subscription for new_failure_cluster events (ALRT-03, D-07)
	alertService.StartReactiveSubscription(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		slog.Info("shutting down")

		// Phase 1: stop accepting new connections, wait for in-flight handlers.
		// Background workers (crons, intelligence, alert eval) keep running with
		// their existing ctx so handlers in flight have all dependencies live.
		httpCtx, httpCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := srv.Shutdown(httpCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
		httpCancel()

		// Phase 2: cancel background workers.
		cancel()

		// Phase 3: flush last_used_at batcher (best-effort, 2s).
		if internalService != nil {
			if err := internalService.WaitFlush(2 * time.Second); err != nil {
				slog.Warn("last_used_at flush incomplete", "err", err)
			}
		}

		// Phase 4: sentry.
		errtrack.Flush(5 * time.Second)
	}()

	slog.Info("processing listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// verifyMigrationsApplied reads the highest version embedded in this binary
// and the highest applied version in `schema_migrations`. It returns an error
// when migrations have not been applied or the schema is dirty. Processing no
// longer runs migrations itself — that is delegated to `agentorbit-migrate`.
func verifyMigrationsApplied(ctx context.Context, pool *pgxpool.Pool) error {
	maxV, err := migrations.MaxEmbeddedVersion()
	if err != nil {
		return fmt.Errorf("scan embedded migrations: %w", err)
	}
	if maxV == 0 {
		// No embedded migrations — nothing to enforce.
		return nil
	}

	// schema_migrations is created by golang-migrate on first up. Tolerate
	// both "table missing" and "row missing" by querying via to_regclass.
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return fmt.Errorf("check schema_migrations existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("schema_migrations table missing — run `agentorbit-migrate up` before starting processing (expected version >= %d)", maxV)
	}

	var (
		appliedVersion int64
		dirty          bool
	)
	row := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1")
	if err := row.Scan(&appliedVersion, &dirty); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("schema_migrations is empty — run `agentorbit-migrate up` before starting processing (expected version >= %d)", maxV)
		}
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	if dirty {
		return fmt.Errorf("schema_migrations is dirty at version %d — investigate and run `agentorbit-migrate force` once safe", appliedVersion)
	}
	if uint(appliedVersion) < maxV {
		return fmt.Errorf("schema_migrations version %d is older than embedded version %d — run `agentorbit-migrate up`", appliedVersion, maxV)
	}
	slog.Info("migrations verified", "applied_version", appliedVersion, "max_embedded_version", maxV)
	return nil
}
