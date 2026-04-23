//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/llm"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

// mockLLMClient is a simple mock that returns a fixed response.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return m.response, m.err
}

func TestIntelligencePipeline_WithMockLLM(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()

	// Create self_host org (triggers LLM narrative path).
	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "intel-llm@example.com", "Intel LLM")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Intel LLM Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hmacSecret := "test-hmac-secret-32chars-minimum!"
	apiKeySvc := service.NewAPIKeyService(queries, hmacSecret, encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "LLM Agent", "openai", "sk-llm-test", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	mockClient := &mockLLMClient{response: "The agent answered 3 questions about Go programming."}
	internalSvc := service.NewInternalService(queries, pool, hmacSecret, encKey, h)
	intelSvc := service.NewIntelligenceService(queries, pool, mockClient, h)
	internalSvc.SetIntelligenceService(intelSvc)

	// Shared system prompt prefix > 100 chars
	sharedPrefix := "system: You are a helpful AI assistant that answers questions about software engineering. Please be thorough and precise in your responses.\nuser: "

	// Ingest spans
	for i := 0; i < 3; i++ {
		err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          sharedPrefix + fmt.Sprintf("Question %d", i),
			Output:         fmt.Sprintf("Answer %d", i),
			InputTokens:    100,
			OutputTokens:   50,
			DurationMs:     200,
			HTTPStatus:     200,
			StartedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest span %d: %v", i, err)
		}
	}

	// Close session
	if err := internalSvc.RunSessionClosureCron(ctx); err != nil {
		t.Fatalf("session closure: %v", err)
	}

	// Get session
	dashSvc := service.NewDashboardService(queries, pool, 100000)
	sessions, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions.Sessions) == 0 {
		t.Fatal("no sessions found")
	}
	sessionID := sessions.Sessions[0].ID

	// Run pipeline with mock LLM
	err = intelSvc.RunPipeline(ctx, sessionID, org.ID)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	// Verify LLM-generated narrative was stored
	detail, err := dashSvc.GetSession(ctx, org.ID, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if detail.Narrative == nil || *detail.Narrative != "The agent answered 3 questions about Go programming." {
		t.Errorf("narrative = %v, want LLM response", detail.Narrative)
	}
}

func TestIntelligencePipeline_LLMError_FallsBackToDeterministic(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "intel-llmerr@example.com", "Intel LLMErr")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Intel LLMErr Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hmacSecret := "test-hmac-secret-32chars-minimum!"
	apiKeySvc := service.NewAPIKeyService(queries, hmacSecret, encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Err Agent", "openai", "sk-err", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Mock that returns an error — should fall back to deterministic
	mockClient := &mockLLMClient{err: fmt.Errorf("LLM unavailable")}
	internalSvc := service.NewInternalService(queries, pool, hmacSecret, encKey, h)
	intelSvc := service.NewIntelligenceService(queries, pool, mockClient, h)
	internalSvc.SetIntelligenceService(intelSvc)

	for i := 0; i < 2; i++ {
		err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "user: hello",
			Output:         "hi there",
			InputTokens:    10,
			OutputTokens:   5,
			DurationMs:     100,
			HTTPStatus:     200,
			StartedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	if err := internalSvc.RunSessionClosureCron(ctx); err != nil {
		t.Fatalf("session closure: %v", err)
	}

	ds := service.NewDashboardService(sharedQueries, sharedPool, 100000)
	sessions, _ := ds.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if len(sessions.Sessions) == 0 {
		t.Fatal("no sessions")
	}

	err = intelSvc.RunPipeline(ctx, sessions.Sessions[0].ID, org.ID)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	// Should still have a narrative (fallback to deterministic)
	detail, _ := ds.GetSession(ctx, org.ID, sessions.Sessions[0].ID)
	if detail.Narrative == nil || *detail.Narrative == "" {
		t.Error("expected fallback narrative")
	}
}

func TestIntelligencePipeline_FailureCluster_WithMockLLM(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries
	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "self_host")
	user := createTestUser(t, ctx, queries, "intel-cluster-llm@example.com", "Cluster LLM")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Cluster LLM Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hmacSecret := "test-hmac-secret-32chars-minimum!"
	apiKeySvc := service.NewAPIKeyService(queries, hmacSecret, encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Cluster LLM Agent", "openai", "sk-clust-llm", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Mock LLM that returns a "NEW: " label
	mockClient := &mockLLMClient{response: "NEW: Rate limit exceeded on GPT-4"}
	internalSvc := service.NewInternalService(queries, pool, hmacSecret, encKey, h)
	intelSvc := service.NewIntelligenceService(queries, pool, mockClient, h)
	internalSvc.SetIntelligenceService(intelSvc)

	// Ingest 2 failed spans
	for i := 0; i < 2; i++ {
		err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "user: do something",
			Output:         "rate limited",
			InputTokens:    10,
			OutputTokens:   5,
			DurationMs:     50,
			HTTPStatus:     429,
			StartedAt:      time.Now().UTC().Format(time.RFC3339Nano),
			FinishReason:   "error",
		})
		if err != nil {
			t.Fatalf("ingest: %v", err)
		}
	}

	if err := internalSvc.RunSessionClosureCron(ctx); err != nil {
		t.Fatalf("session closure: %v", err)
	}

	ds := service.NewDashboardService(sharedQueries, sharedPool, 100000)
	sessions, _ := ds.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if len(sessions.Sessions) == 0 {
		t.Fatal("no sessions")
	}

	err = intelSvc.RunPipeline(ctx, sessions.Sessions[0].ID, org.ID)
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}
}
