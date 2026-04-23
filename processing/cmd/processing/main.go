package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/agentorbit-tech/agentorbit/processing/internal/config"
	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
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
		Level: &logLevel,
	})
	slog.SetDefault(slog.New(logHandler))

	cfg, err := config.LoadProcessing()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.Info("running database migrations")
	if err := runMigrations(cfg.DatabaseURL); err != nil {
		slog.Error("migrations failed", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations complete")

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database config failed", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = 25
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	pool, err := pgxpool.NewWithConfig(dbCtx, poolCfg)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: fatal startup error, no cleanup needed
	}
	defer pool.Close()

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
	dashboardService := service.NewDashboardService(queries, pool, cfg.ExportRowLimit)
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
	authHandler := handler.NewAuthHandler(authService, mailer, cfg.AppBaseURL, cfg.JWTTTLDuration(), emailRateLimiter, queries)
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

	// Global middleware
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.MaxBodySize(1 << 20)) // 1MB default limit for all endpoints
	if cfg.AllowedOrigins != "" {
		r.Use(middleware.CORS(cfg.AllowedOrigins))
	}

	// Health check (verifies database connectivity)
	r.Get("/health", handler.NewHealthHandler(pool))

	// Deployment metadata — public, surfaces APP_VERSION in the UI footer
	r.Get("/meta", handler.NewMetaHandler(cfg.AppVersion))

	// Public auth routes — no authentication required, rate-limited
	authRateLimiter := middleware.NewRateLimiter(ctx, 20, 1*time.Minute, cfg.TrustedProxies)
	r.With(authRateLimiter.Middleware, middleware.RequireXHR).Mount("/auth", authHandler.Routes())

	// Invite acceptance — authenticated user, not org-scoped, rate-limited
	r.With(authRateLimiter.Middleware, middleware.RequireXHR, middleware.Authenticate(cfg.JWTSecret, cfg.HMACSecret, queries)).
		Post("/auth/accept-invite", inviteHandler.AcceptInvite)

	// Dashboard API — JWT or API-key authentication
	r.Route("/api", func(r chi.Router) {
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
	r.Route("/internal", func(r chi.Router) {
		r.Use(middleware.RequireInternalIP(cfg.InternalAllowedIPs))
		r.Use(middleware.RequireInternalToken(cfg.InternalToken))
		r.Use(middleware.MaxBodySize(3 << 20)) // 3MB for span ingest
		r.Mount("/", internalHandler.Routes())
	})

	// WebSocket endpoint — mounted before SPA catch-all so /cable is not intercepted (WS-01)
	// Rate-limited to mitigate connection-exhaustion attacks (H-12).
	wsRateLimiter := middleware.NewRateLimiter(ctx, 10, 1*time.Minute, cfg.TrustedProxies)
	r.With(wsRateLimiter.Middleware).Get("/cable", wsHandler.ServeHTTP)

	// SPA handler — serves the embedded React frontend for all non-API paths
	spaHandler, err := processingWeb.NewSPAHandler()
	if err != nil {
		slog.Warn("SPA handler unavailable, serving API only", "error", err)
	} else {
		r.NotFound(spaHandler.ServeHTTP)
	}

	// Hard-delete cron: permanently removes organizations past their 14-day grace period (ORG-08, D-12)
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := orgService.RunHardDeleteCron(ctx); err != nil {
					slog.Error("hard-delete cron error", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Session closure cron: closes idle sessions and sets terminal status (SESS-04, D-09)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := internalService.RunSessionClosureCron(ctx); err != nil {
					slog.Error("session-closure cron error", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Alert evaluation cron: evaluates alert rules every 60 seconds (ALRT-02)
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := alertService.RunEvaluationCron(ctx); err != nil {
					slog.Error("alert-evaluation cron error", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Data retention cron: purges old spans, sessions, and alert events daily
	if cfg.DataRetentionDays > 0 {
		slog.Info("data retention enabled", "days", cfg.DataRetentionDays)
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if _, err := service.RunRetentionPurge(ctx, queries, cfg.DataRetentionDays); err != nil {
						slog.Error("retention purge error", "error", err)
					}
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
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("processing listening", "port", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func runMigrations(databaseURL string) error {
	// The pgx/v5 migrate driver registers as "pgx5", not "postgres".
	// Rewrite the scheme so migrate finds the correct driver.
	migrateURL := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrations source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, migrateURL)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
