//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agentspan/processing/internal/db"
	"github.com/agentspan/processing/internal/service"
	"github.com/agentspan/processing/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDashboardService_GetSessions(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()

	// Setup: user + org + api key + session + span
	user := createTestUser(t, ctx, queries, "dash-sessions@example.com", "Dash User")
	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Dash Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Dash Agent", "openai", "sk-dash", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	sessionID, err := queries.CreateSession(ctx, db.CreateSessionParams{
		OrganizationID: org.ID,
		ApiKeyID:       apiKeyResult.ID,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	_, err = queries.InsertSpan(ctx, db.InsertSpanParams{
		SessionID:      sessionID,
		OrganizationID: org.ID,
		ProviderType:   "openai",
		Model:          "gpt-4",
		DurationMs:     500,
		HttpStatus:     200,
		StartedAt:      time.Now(),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("insert span: %v", err)
	}

	// Update session counters
	_ = queries.UpdateSessionAfterSpan(ctx, db.UpdateSessionAfterSpanParams{
		ID:      sessionID,
		CostUsd: pgtype.Numeric{Valid: false},
	})

	dashSvc := service.NewDashboardService(queries)
	result, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
	if result.Sessions[0].ID != sessionID {
		t.Error("session ID mismatch")
	}
}

func TestDashboardService_GetSessionDetail(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "dash-detail@example.com", "Detail User")
	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Detail Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Detail Agent", "openai", "sk-detail", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	sessionID, err := queries.CreateSession(ctx, db.CreateSessionParams{
		OrganizationID: org.ID,
		ApiKeyID:       apiKeyResult.ID,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	input := "test input"
	output := "test output"
	_, err = queries.InsertSpan(ctx, db.InsertSpanParams{
		SessionID:      sessionID,
		OrganizationID: org.ID,
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          &input,
		Output:         &output,
		DurationMs:     300,
		HttpStatus:     200,
		StartedAt:      time.Now(),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("insert span: %v", err)
	}

	_ = queries.UpdateSessionAfterSpan(ctx, db.UpdateSessionAfterSpanParams{
		ID:      sessionID,
		CostUsd: pgtype.Numeric{Valid: false},
	})

	dashSvc := service.NewDashboardService(queries)
	detail, err := dashSvc.GetSession(ctx, org.ID, sessionID)
	if err != nil {
		t.Fatalf("get session detail: %v", err)
	}
	if detail.ID != sessionID {
		t.Error("session ID mismatch")
	}
	if len(detail.Spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(detail.Spans))
	}
}

func TestDashboardService_GetStats(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()

	user := createTestUser(t, ctx, queries, "dash-stats@example.com", "Stats User")
	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Stats Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Get stats on empty org (should return zeros, not errors)
	dashSvc := service.NewDashboardService(queries)
	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	stats, err := dashSvc.GetStats(ctx, org.ID, from, to)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 sessions, got %d", stats.TotalSessions)
	}
	if stats.TotalSpans != 0 {
		t.Errorf("expected 0 spans, got %d", stats.TotalSpans)
	}

	// Get stats for non-existent org (should still work without error)
	_, err = dashSvc.GetStats(ctx, uuid.New(), from, to)
	if err != nil {
		t.Fatalf("get stats for non-existent org: %v", err)
	}
}
