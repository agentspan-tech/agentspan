//go:build integration

package service_test

import (
	"context"
	"fmt"
	"os"
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

var (
	sharedPool    *pgxpool.Pool
	sharedQueries *db.Queries
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_svc_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	// Run migrations
	d, _ := iofs.New(migrations.FS, ".")
	migrateURL := "pgx5://" + connStr[len("postgres://"):]
	mig, _ := migrate.NewWithSourceInstance("iofs", d, migrateURL)
	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		_ = pgContainer.Terminate(ctx)
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		_ = pgContainer.Terminate(ctx)
		fmt.Fprintf(os.Stderr, "pool failed: %v\n", err)
		os.Exit(1)
	}

	sharedPool = pool
	sharedQueries = db.New(pool)

	code := m.Run()

	pool.Close()
	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

// truncate cleans all tables for test isolation.
func truncate(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	tables := []string{
		"span_system_prompts", "system_prompts", "spans", "sessions",
		"alert_rules", "alert_events", "failure_clusters", "invites",
		"api_keys", "memberships", "organizations",
		"email_verification_tokens", "password_reset_tokens", "users",
	}
	for _, table := range tables {
		if _, err := sharedPool.Exec(ctx, "TRUNCATE "+table+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
}
