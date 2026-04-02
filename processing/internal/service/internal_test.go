//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentspan/processing/internal/hub"
	"github.com/agentspan/processing/internal/service"
	"github.com/agentspan/processing/internal/testutil"
)

func TestInternalService_IngestSpan(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	// Create org + api key
	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "ingest@example.com", "Ingest User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Ingest Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Ingest Agent", "openai", "sk-ingest", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       apiKeyResult.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "test input",
		Output:         "test output",
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

	// Verify session was created by listing sessions
	dashSvc := service.NewDashboardService(queries)
	result, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(result.Sessions))
	}
}

func TestInternalService_SessionGrouping(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "grouping@example.com", "Grouping User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Grouping Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Grouping Agent", "openai", "sk-grouping", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Ingest two spans quickly — should be grouped into the same session
	for i := 0; i < 2; i++ {
		err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "input",
			Output:         "output",
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

	dashSvc := service.NewDashboardService(queries)
	result, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session (grouped), got %d", len(result.Sessions))
	}
}

func TestInternalService_SessionTimeout_SplitsSession(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "timeout@example.com", "Timeout User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Timeout Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Timeout Agent", "openai", "sk-timeout", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Ingest first span
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       apiKeyResult.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "first input",
		Output:         "first output",
		InputTokens:    10,
		OutputTokens:   10,
		DurationMs:     100,
		HTTPStatus:     200,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("ingest first span: %v", err)
	}

	// Push the session's last_span_at back beyond the timeout (default 60s)
	_, err = pool.Exec(ctx, `UPDATE sessions SET last_span_at = NOW() - interval '120 seconds' WHERE organization_id = $1`, org.ID)
	if err != nil {
		t.Fatalf("backdate session: %v", err)
	}

	// Ingest second span — should create a NEW session since the first timed out
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       apiKeyResult.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "second input",
		Output:         "second output",
		InputTokens:    10,
		OutputTokens:   10,
		DurationMs:     100,
		HTTPStatus:     200,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("ingest second span: %v", err)
	}

	dashSvc := service.NewDashboardService(queries)
	result, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("expected 2 sessions (split by timeout), got %d", len(result.Sessions))
	}
}

func TestInternalService_ExplicitSession(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "explicit@example.com", "Explicit User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Explicit Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Explicit Agent", "openai", "sk-explicit", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Ingest with explicit session ID
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:          apiKeyResult.ID.String(),
		OrganizationID:    org.ID.String(),
		ProviderType:      "openai",
		Model:             "gpt-4",
		Input:             "input",
		Output:            "output",
		InputTokens:       10,
		OutputTokens:      10,
		DurationMs:        100,
		HTTPStatus:        200,
		StartedAt:         time.Now().Format(time.RFC3339Nano),
		FinishReason:      "stop",
		ExternalSessionID: "my-custom-session-123",
	})
	if err != nil {
		t.Fatalf("ingest span with explicit session: %v", err)
	}

	// Ingest second span with same explicit session ID
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:          apiKeyResult.ID.String(),
		OrganizationID:    org.ID.String(),
		ProviderType:      "openai",
		Model:             "gpt-4",
		Input:             "input 2",
		Output:            "output 2",
		InputTokens:       20,
		OutputTokens:      20,
		DurationMs:        200,
		HTTPStatus:        200,
		StartedAt:         time.Now().Format(time.RFC3339Nano),
		FinishReason:      "stop",
		ExternalSessionID: "my-custom-session-123",
	})
	if err != nil {
		t.Fatalf("ingest second span with explicit session: %v", err)
	}

	dashSvc := service.NewDashboardService(queries)
	result, err := dashSvc.ListSessions(ctx, org.ID, service.ListSessionsParams{})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(result.Sessions) != 1 {
		t.Fatalf("expected 1 session (explicit grouping), got %d", len(result.Sessions))
	}
	if result.Sessions[0].ExternalID == nil || *result.Sessions[0].ExternalID != "my-custom-session-123" {
		t.Error("expected session to have external_id 'my-custom-session-123'")
	}
}

func TestInternalService_SpanQuota_ExactLimit(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "quota@example.com", "Quota User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Quota Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Quota Agent", "openai", "sk-quota", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Insert 3000 spans (the free-plan limit).
	for i := 0; i < 3000; i++ {
		err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "in",
			Output:         "out",
			InputTokens:    1,
			OutputTokens:   1,
			DurationMs:     10,
			HTTPStatus:     200,
			StartedAt:      time.Now().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest span %d: %v", i+1, err)
		}
	}

	// The 3001st should be rejected.
	err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
		APIKeyID:       apiKeyResult.ID.String(),
		OrganizationID: org.ID.String(),
		ProviderType:   "openai",
		Model:          "gpt-4",
		Input:          "over limit",
		Output:         "over limit",
		InputTokens:    1,
		OutputTokens:   1,
		DurationMs:     10,
		HTTPStatus:     200,
		StartedAt:      time.Now().Format(time.RFC3339Nano),
		FinishReason:   "stop",
	})
	if err == nil {
		t.Fatal("expected quota exceeded error, got nil")
	}
	var quotaErr *service.SpanQuotaExceededError
	if !errors.As(err, &quotaErr) {
		t.Fatalf("expected SpanQuotaExceededError, got %T: %v", err, err)
	}
}

func TestInternalService_SpanQuota_ConcurrentEnforcement(t *testing.T) {
	truncate(t)
	pool, queries := sharedPool, sharedQueries

	ctx := context.Background()
	mailer := &testutil.MockMailer{}
	encKey := hex.EncodeToString(make([]byte, 32))
	h := hub.New()

	orgSvc := service.NewOrgService(queries, pool, mailer, "cloud")
	user := createTestUser(t, ctx, queries, "concurrent-quota@example.com", "CQ User")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "CQ Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	apiKeySvc := service.NewAPIKeyService(queries, "test-hmac-secret", encKey)
	apiKeyResult, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "CQ Agent", "openai", "sk-cq", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	internalSvc := service.NewInternalService(queries, sharedPool, "test-hmac-secret", encKey, h)

	// Insert 2995 spans to get close to the limit.
	for i := 0; i < 2995; i++ {
		err = internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
			APIKeyID:       apiKeyResult.ID.String(),
			OrganizationID: org.ID.String(),
			ProviderType:   "openai",
			Model:          "gpt-4",
			Input:          "in",
			Output:         "out",
			InputTokens:    1,
			OutputTokens:   1,
			DurationMs:     10,
			HTTPStatus:     200,
			StartedAt:      time.Now().Format(time.RFC3339Nano),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("ingest span %d: %v", i+1, err)
		}
	}

	// Launch 10 goroutines each trying to insert 1 span. Only 5 should succeed.
	var wg sync.WaitGroup
	var succeeded atomic.Int64
	var failed atomic.Int64

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := internalSvc.IngestSpan(ctx, &service.SpanIngestRequest{
				APIKeyID:       apiKeyResult.ID.String(),
				OrganizationID: org.ID.String(),
				ProviderType:   "openai",
				Model:          "gpt-4",
				Input:          fmt.Sprintf("concurrent-%d", idx),
				Output:         "out",
				InputTokens:    1,
				OutputTokens:   1,
				DurationMs:     10,
				HTTPStatus:     200,
				StartedAt:      time.Now().Format(time.RFC3339Nano),
				FinishReason:   "stop",
			})
			if err != nil {
				failed.Add(1)
			} else {
				succeeded.Add(1)
			}
		}(i)
	}
	wg.Wait()

	// Exactly 5 should succeed (2995 + 5 = 3000), the rest should fail.
	if succeeded.Load() != 5 {
		t.Errorf("expected exactly 5 successful inserts, got %d (failed=%d)", succeeded.Load(), failed.Load())
	}
	if failed.Load() != 5 {
		t.Errorf("expected exactly 5 failures, got %d", failed.Load())
	}
}
