//go:build integration

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	migrations "github.com/agentorbit-tech/agentorbit/processing/migrations"
)

// SetupTestDB creates a real Postgres container, runs all migrations, and returns
// a pgxpool.Pool, db.Queries, and cleanup function. Call cleanup in t.Cleanup().
func SetupTestDB(t *testing.T) (*pgxpool.Pool, *db.Queries, func()) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Run migrations
	d, err := iofs.New(migrations.FS, ".")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to create migration source: %v", err)
	}
	// Convert connStr for migrate (pgx5:// prefix)
	migrateURL := "pgx5://" + connStr[len("postgres://"):]
	m, err := migrate.NewWithSourceInstance("iofs", d, migrateURL)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to create migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to run migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		t.Fatalf("failed to create pool: %v", err)
	}

	queries := db.New(pool)

	cleanup := func() {
		pool.Close()
		_ = pgContainer.Terminate(ctx)
	}
	return pool, queries, cleanup
}

// TruncateAll truncates all tables (for test isolation between tests in same container).
func TruncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		"span_system_prompts", "system_prompts", "spans", "sessions",
		"alert_rules", "alert_events", "failure_clusters", "invites",
		"api_keys", "memberships", "organizations",
		"email_verification_tokens", "password_reset_tokens", "users",
	}
	for _, table := range tables {
		_, err := pool.Exec(ctx, "TRUNCATE "+table+" CASCADE")
		if err != nil {
			t.Fatalf("failed to truncate %s: %v", table, err)
		}
	}
}
