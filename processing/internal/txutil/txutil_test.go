//go:build integration

package txutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentspan/processing/internal/testutil"
	"github.com/jackc/pgx/v5"
)

func TestWithTx_Commit(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Run a statement inside a committed transaction.
	err := WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "CREATE TABLE _txutil_test_commit (id int)")
		return err
	})
	if err != nil {
		t.Fatalf("WithTx commit: %v", err)
	}

	// Verify the table exists (transaction was committed).
	var exists bool
	err = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = '_txutil_test_commit')").Scan(&exists)
	if err != nil {
		t.Fatalf("check table: %v", err)
	}
	if !exists {
		t.Error("expected table to exist after committed transaction")
	}
}

func TestWithTx_Rollback(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create a table first so we can test rollback of an INSERT.
	_, err := pool.Exec(ctx, "CREATE TABLE _txutil_test_rollback (id int)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	sentinel := errors.New("rollback me")
	err = WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO _txutil_test_rollback (id) VALUES (1)")
		if err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got: %v", err)
	}

	// Verify the INSERT was rolled back.
	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM _txutil_test_rollback").Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows after rollback, got %d", count)
	}
}

func TestWithTx_CancelledContext(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := WithTx(ctx, pool, func(tx pgx.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}
