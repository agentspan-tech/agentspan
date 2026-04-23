//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

// TestIntelligenceService_RunPipeline_DeterministicCluster exercises clusterFailure
// and deterministicCluster by ingesting a failed span and running the pipeline.
// Plan is "free" by default so LLM path is skipped (deterministic clustering).
func TestIntelligenceService_RunPipeline_DeterministicCluster(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "det-cluster@example.com", "Det Cluster")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Det Cluster Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	ak, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-det", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, pool, "test-hmac-secret", encKey, h)

	// Ingest a failed span (HTTP 500)
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       ak.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "user question",
		Output:         "internal server error from provider",
		DurationMs:     500,
		HTTPStatus:     500,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "error",
	})
	if err != nil {
		t.Fatalf("ingest span: %v", err)
	}

	// Get the session
	dashSvc := service.NewDashboardService(queries, pool, 100000)
	sessions, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions.Sessions))
	}
	sessionID := sessions.Sessions[0].ID

	// Force session status to "failed" so clustering happens
	_, err = pool.Exec(ctx, "UPDATE sessions SET status='failed', closed_at=NOW() WHERE id=$1", sessionID)
	if err != nil {
		t.Fatalf("update session: %v", err)
	}

	// Run pipeline with no LLM (free plan behavior) — triggers deterministicCluster
	intellSvc := service.NewIntelligenceService(queries, pool, nil, h)
	if err := intellSvc.RunPipeline(ctx, sessionID, org.ID); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	// Verify a failure cluster was created and session is linked
	clusters, err := dashSvc.ListFailureClusters(ctx, org.ID)
	if err != nil {
		t.Fatalf("list clusters: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("expected 1 failure cluster, got %d", len(clusters))
	}
	if clusters[0].SessionCount != 1 {
		t.Errorf("expected cluster session_count=1, got %d", clusters[0].SessionCount)
	}
}

func TestIntelligenceService_RunPipeline_NonFailedSession_NoCluster(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	pool, queries := sharedPool, sharedQueries

	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "ok-cluster@example.com", "OK User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "OK Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	ak, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-ok", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, pool, "test-hmac-secret", encKey, h)

	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       ak.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "hi",
		Output:         "hello",
		DurationMs:     200,
		HTTPStatus:     200,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Get session, mark as completed (not failed)
	dashSvc := service.NewDashboardService(queries, pool, 100000)
	sessions, _ := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if len(sessions.Sessions) != 1 {
		t.Fatalf("expected 1 session")
	}
	sessionID := sessions.Sessions[0].ID
	_, _ = pool.Exec(ctx, "UPDATE sessions SET status='completed', closed_at=NOW() WHERE id=$1", sessionID)

	intellSvc := service.NewIntelligenceService(queries, pool, nil, h)
	if err := intellSvc.RunPipeline(ctx, sessionID, org.ID); err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	clusters, _ := dashSvc.ListFailureClusters(ctx, org.ID)
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for completed session, got %d", len(clusters))
	}
}
