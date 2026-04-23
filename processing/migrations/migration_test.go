//go:build integration

package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestMigrationDownUp verifies that all migrations can be applied, fully rolled back,
// and re-applied. This proves all down migrations are valid SQL.
func TestMigrationDownUp(t *testing.T) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("agentorbit_migration_test"),
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
	t.Cleanup(func() { _ = pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Create migrate source
	d, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatalf("failed to create migration source: %v", err)
	}

	migrateURL := "pgx5://" + connStr[len("postgres://"):]

	m, err := migrate.NewWithSourceInstance("iofs", d, migrateURL)
	if err != nil {
		t.Fatalf("failed to create migrator: %v", err)
	}

	// Step 1: Migrate up (apply all migrations)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up failed: %v", err)
	}
	t.Log("migrate up: success")

	// Step 2: Migrate down (roll back all migrations)
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate down failed: %v", err)
	}
	t.Log("migrate down: success")

	// Need to recreate migrator after Down() (driver state is closed)
	d2, err := iofs.New(FS, ".")
	if err != nil {
		t.Fatalf("failed to create migration source for re-up: %v", err)
	}
	m2, err := migrate.NewWithSourceInstance("iofs", d2, migrateURL)
	if err != nil {
		t.Fatalf("failed to create migrator for re-up: %v", err)
	}

	// Step 3: Migrate up again (proves down migrations cleaned up correctly)
	if err := m2.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate re-up failed: %v", err)
	}
	t.Log("migrate re-up: success — full down/up cycle verified")
}
