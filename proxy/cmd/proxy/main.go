package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/auth"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/config"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/handler"
	"github.com/agentorbit-tech/agentorbit/proxy/internal/span"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
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

	cfg, err := config.LoadProxy()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Context with signal cancellation for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Auth cache — validates AgentOrbit API keys via Processing
	cache := auth.NewAuthCache(
		ctx,
		cfg.ProcessingURL,
		cfg.InternalToken,
		cfg.HMACSecret,
		cfg.AuthCacheTTL,
		&http.Client{Timeout: 5 * time.Second},
		cfg.CacheEvictInterval,
	)

	// Span dispatcher — async send to Processing
	dispatcher := span.NewSpanDispatcher(
		cfg.ProcessingURL,
		cfg.InternalToken,
		cfg.SpanBufferSize,
		&http.Client{Timeout: cfg.SpanSendTimeout},
		cfg.SpanSendTimeout,
		cfg.DrainTimeout,
		cfg.SpanWorkers,
	)
	dispatcher.Start(ctx)

	// Proxy handler
	proxyHandler := handler.NewProxyHandler(ctx, cache, dispatcher, cfg.ProviderTimeout, cfg.DefaultAnthropicVersion, cfg.AllowPrivateProviderIPs, cfg.PerKeyRateLimit)

	// Router
	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "ok",
			"spans_dropped": dispatcher.Dropped(),
			"goroutines":    runtime.NumGoroutine(),
			"heap_alloc_mb": m.HeapAlloc / 1024 / 1024,
		})
	})

	// Proxy endpoints
	r.Post("/v1/chat/completions", proxyHandler.ServeHTTP)
	r.Post("/v1/messages", proxyHandler.ServeHTTP)

	slog.Info("proxy listening", "port", cfg.Port)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		slog.Info("shutting down proxy")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: process is terminating, defers are for graceful shutdown path only
	}
}
