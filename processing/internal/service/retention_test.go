//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
)

func TestRetentionPurge_EmptyDB(t *testing.T) {
	truncate(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	total, err := service.RunRetentionPurge(ctx, sharedQueries, 30)
	if err != nil {
		t.Fatalf("retention purge: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 deleted, got %d", total)
	}
}
