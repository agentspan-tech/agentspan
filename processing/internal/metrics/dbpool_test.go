//go:build integration

package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/agentspan/processing/internal/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestStartDBPoolCollector(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartDBPoolCollector(ctx, pool, 100*time.Millisecond)

	// Wait for at least one collection cycle.
	time.Sleep(250 * time.Millisecond)

	// MaxConns should be set immediately (not zero).
	var m dto.Metric
	if err := DBPoolMaxConns.Write(&m); err != nil {
		t.Fatalf("read DBPoolMaxConns: %v", err)
	}
	if m.GetGauge().GetValue() <= 0 {
		t.Error("expected DBPoolMaxConns > 0")
	}

	// TotalConns should have been updated by the ticker.
	var total dto.Metric
	if err := DBPoolTotalConns.Write(&total); err != nil {
		t.Fatalf("read DBPoolTotalConns: %v", err)
	}
	// Just verify it doesn't panic and has a non-negative value.
	if total.GetGauge().GetValue() < 0 {
		t.Error("expected DBPoolTotalConns >= 0")
	}
}

func TestStartDBPoolCollector_DefaultInterval(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	// Pass zero interval — should default to 15s without panicking.
	StartDBPoolCollector(ctx, pool, 0)
	cancel()
}

func TestStartDBPoolCollector_CancelStops(t *testing.T) {
	pool, _, cleanup := testutil.SetupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	StartDBPoolCollector(ctx, pool, 50*time.Millisecond)

	// Cancel and verify no panic.
	cancel()
	time.Sleep(100 * time.Millisecond)
}
