//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/hub"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
)

func TestIntelligenceService_ExtractSystemPrompts(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "sysprompt@example.com", "SysPrompt User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "SysPrompt Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "SysPrompt Agent", "openai", "sk-sysprompt", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Common system prompt prefix (>= 100 chars)
	systemPrompt := strings.Repeat("You are a helpful assistant. ", 5) // 145 chars

	// Ingest two spans with the same long prefix but different user messages
	for i, userMsg := range []string{"What is Go?", "What is Rust?"} {
		err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          systemPrompt + userMsg,
			Output:         "response " + userMsg,
			InputTokens:    100,
			OutputTokens:   50,
			DurationMs:     500,
			HTTPStatus:     200,
			StartedAt:      time.Now().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest span %d: %v", i, err)
		}
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

	// Run the intelligence pipeline
	intellSvc := service.NewIntelligenceService(queries, sharedPool, nil, h)
	err = intellSvc.RunPipeline(ctx, sessionID, org.ID)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	// Verify system prompt was extracted
	var spCount int64
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM system_prompts WHERE organization_id = $1", org.ID).Scan(&spCount)
	if err != nil {
		t.Fatalf("count system prompts: %v", err)
	}
	if spCount != 1 {
		t.Fatalf("expected 1 system prompt, got %d", spCount)
	}

	// Verify content
	var content, shortUID string
	err = pool.QueryRow(ctx, "SELECT content, short_uid FROM system_prompts WHERE organization_id = $1", org.ID).Scan(&content, &shortUID)
	if err != nil {
		t.Fatalf("get system prompt: %v", err)
	}
	if !strings.HasPrefix(content, "You are a helpful assistant.") {
		t.Errorf("unexpected system prompt content: %q", content[:50])
	}
	if shortUID != "SP-1" {
		t.Errorf("expected short_uid SP-1, got %q", shortUID)
	}

	// Verify span inputs were replaced (prefix removed, placeholder inserted)
	var spanInput string
	err = pool.QueryRow(ctx, "SELECT input FROM spans WHERE organization_id = $1 ORDER BY created_at LIMIT 1", org.ID).Scan(&spanInput)
	if err != nil {
		t.Fatalf("get span input: %v", err)
	}
	if !strings.HasPrefix(spanInput, "[System Prompt #SP-1]") {
		t.Errorf("expected span input to start with placeholder, got: %q", spanInput[:50])
	}
}

func TestIntelligenceService_ExtractSystemPrompts_ShortPrefix(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "shortprefix@example.com", "Short User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Short Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Short Agent", "openai", "sk-short", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Ingest two spans with a short common prefix (< 100 chars)
	for i, msg := range []string{"Hello world: question 1", "Hello world: question 2"} {
		err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          msg,
			Output:         "response",
			InputTokens:    10,
			OutputTokens:   10,
			DurationMs:     100,
			HTTPStatus:     200,
			StartedAt:      time.Now().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest span %d: %v", i, err)
		}
	}

	sessions, err := service.NewDashboardService(queries, pool, 100000).ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	sessionID := sessions.Sessions[0].ID

	intellSvc := service.NewIntelligenceService(queries, sharedPool, nil, h)
	err = intellSvc.RunPipeline(ctx, sessionID, org.ID)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	// Should NOT create a system prompt (prefix < 100 chars)
	var spCount int64
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM system_prompts WHERE organization_id = $1", org.ID).Scan(&spCount)
	if err != nil {
		t.Fatalf("count system prompts: %v", err)
	}
	if spCount != 0 {
		t.Errorf("expected 0 system prompts for short prefix, got %d", spCount)
	}
}

func TestIntelligenceService_ExtractSystemPrompts_SingleSpan(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "single@example.com", "Single User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Single Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Single Agent", "openai", "sk-single", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Only one span — can't extract system prompt (need >= 2 to compare)
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       apiKeyResult.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          strings.Repeat("System prompt text. ", 10) + "user query",
		Output:         "response",
		InputTokens:    100,
		OutputTokens:   50,
		DurationMs:     500,
		HTTPStatus:     200,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("ingest span: %v", err)
	}

	sessions, _ := service.NewDashboardService(queries, pool, 100000).ListSessions(ctx, org.ID, service.ListSessionsParams{})
	sessionID := sessions.Sessions[0].ID

	intellSvc := service.NewIntelligenceService(queries, sharedPool, nil, h)
	err = intellSvc.RunPipeline(ctx, sessionID, org.ID)
	if err != nil {
		t.Fatalf("run pipeline: %v", err)
	}

	// Should NOT create system prompt with only 1 span
	var spCount int64
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM system_prompts WHERE organization_id = $1", org.ID).Scan(&spCount)
	if err != nil {
		t.Fatalf("count system prompts: %v", err)
	}
	if spCount != 0 {
		t.Errorf("expected 0 system prompts for single span, got %d", spCount)
	}
}
