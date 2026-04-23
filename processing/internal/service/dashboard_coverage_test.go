//go:build integration

package service_test

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/agentorbit-tech/agentorbit/processing/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// setupDashOrg creates a user + org + api key and returns all three plus a span inserter.
// Many dashboard tests need the same minimal graph.
type dashFixture struct {
	userID   uuid.UUID
	orgID    uuid.UUID
	apiKeyID uuid.UUID
	svc      *service.DashboardService
}

func makeDashFixture(t *testing.T, emailPrefix string) *dashFixture {
	t.Helper()
	ctx := context.Background()

	user := createTestUser(t, ctx, sharedQueries, emailPrefix+"@example.com", "Dash User")
	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Dash Org "+emailPrefix)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)
	ak, err := apiKeySvc.CreateAPIKey(ctx, org.ID, emailPrefix+" Agent", "openai", "sk-"+emailPrefix, nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	return &dashFixture{
		userID:   user.ID,
		orgID:    org.ID,
		apiKeyID: ak.ID,
		svc:      service.NewDashboardService(sharedQueries, sharedPool, 100000),
	}
}

func (f *dashFixture) insertSpan(t *testing.T, sessionID uuid.UUID, httpStatus int32, model string) {
	t.Helper()
	_, err := sharedQueries.InsertSpan(context.Background(), db.InsertSpanParams{
		SessionID:      sessionID,
		OrganizationID: f.orgID,
		ProviderType:   "openai",
		Model:          model,
		DurationMs:     200,
		HttpStatus:     httpStatus,
		StartedAt:      time.Now(),
		FinishReason:   "stop",
	})
	if err != nil {
		t.Fatalf("insert span: %v", err)
	}
	_ = sharedQueries.UpdateSessionAfterSpan(context.Background(), db.UpdateSessionAfterSpanParams{
		ID:      sessionID,
		CostUsd: pgtype.Numeric{Valid: false},
	})
}

func (f *dashFixture) newSession(t *testing.T) uuid.UUID {
	t.Helper()
	id, err := sharedQueries.CreateSession(context.Background(), db.CreateSessionParams{
		OrganizationID: f.orgID,
		ApiKeyID:       f.apiKeyID,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return id
}

// --- Cursor encode/decode ---

func TestDashboardService_DecodeCursor_Valid(t *testing.T) {
	// Since encodeCursor is private, we test via round-trip through ListSessions
	// with an explicit cursor. Here we just verify DecodeCursor exported API.
	_, _, err := service.DecodeCursor("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestDashboardService_DecodeCursor_InvalidJSON(t *testing.T) {
	// "bm90LWpzb24=" is base64 of "not-json"
	_, _, err := service.DecodeCursor("bm90LWpzb24=")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDashboardService_DecodeCursor_InvalidTimestamp(t *testing.T) {
	// base64 of {"ts":"bad","id":"00000000-0000-0000-0000-000000000000"}
	cur := "eyJ0cyI6ImJhZCIsImlkIjoiMDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAwIn0="
	_, _, err := service.DecodeCursor(cur)
	if err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}

func TestDashboardService_DecodeCursor_InvalidID(t *testing.T) {
	// base64 of {"ts":"2026-04-22T00:00:00Z","id":"not-a-uuid"}
	cur := "eyJ0cyI6IjIwMjYtMDQtMjJUMDA6MDA6MDBaIiwiaWQiOiJub3QtYS11dWlkIn0="
	_, _, err := service.DecodeCursor(cur)
	if err == nil {
		t.Fatal("expected error for bad UUID")
	}
}

func TestDashboardService_DecodeSortCursor_Invalid(t *testing.T) {
	_, _, err := service.DecodeSortCursor("!!!not-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
	_, _, err = service.DecodeSortCursor("bm90LWpzb24=")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	// base64 of {"v":"x","id":"not-a-uuid"}
	_, _, err = service.DecodeSortCursor("eyJ2IjoieCIsImlkIjoibm90LWEtdXVpZCJ9")
	if err == nil {
		t.Fatal("expected error for bad UUID")
	}
}

// --- GetAgentStats / GetDailyStats / GetFinishReasonDistribution ---

func TestDashboardService_GetAgentStats_Empty(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "agent-stats")
	stats, err := f.svc.GetAgentStats(context.Background(), f.orgID, time.Now().Add(-24*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("GetAgentStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("expected 0 rows, got %d", len(stats))
	}
}

func TestDashboardService_GetAgentStats_WithData(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "agent-stats-data")
	sess := f.newSession(t)
	f.insertSpan(t, sess, 200, "gpt-4")

	stats, err := f.svc.GetAgentStats(context.Background(), f.orgID, time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetAgentStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 row, got %d", len(stats))
	}
	if stats[0].APIKeyID != f.apiKeyID {
		t.Errorf("apikey id mismatch")
	}
	if stats[0].SpanCount != 1 {
		t.Errorf("expected 1 span, got %d", stats[0].SpanCount)
	}
}

func TestDashboardService_GetDailyStats_Default(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "daily-stats")

	// days <= 0 should default to 30
	stats, err := f.svc.GetDailyStats(context.Background(), f.orgID, 0)
	if err != nil {
		t.Fatalf("GetDailyStats(0): %v", err)
	}
	_ = stats

	// days > 365 should cap at 365
	stats, err = f.svc.GetDailyStats(context.Background(), f.orgID, 1000)
	if err != nil {
		t.Fatalf("GetDailyStats(1000): %v", err)
	}
	_ = stats
}

func TestDashboardService_GetDailyStats_WithData(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "daily-stats-data")
	sess := f.newSession(t)
	f.insertSpan(t, sess, 200, "gpt-4")

	stats, err := f.svc.GetDailyStats(context.Background(), f.orgID, 7)
	if err != nil {
		t.Fatalf("GetDailyStats: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("expected at least 1 day row")
	}
}

func TestDashboardService_GetFinishReasonDistribution(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "finish-reason")
	sess := f.newSession(t)
	f.insertSpan(t, sess, 200, "gpt-4")

	ctx := context.Background()
	rows, err := f.svc.GetFinishReasonDistribution(ctx, f.orgID, time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("GetFinishReasonDistribution: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.FinishReason == "stop" && r.Count >= 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find finish_reason=stop, got %+v", rows)
	}
}

// --- System prompts ---

func TestDashboardService_ListSystemPrompts_Empty(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "sys-list")
	prompts, err := f.svc.ListSystemPrompts(context.Background(), f.orgID)
	if err != nil {
		t.Fatalf("ListSystemPrompts: %v", err)
	}
	if len(prompts) != 0 {
		t.Errorf("expected 0, got %d", len(prompts))
	}
}

func TestDashboardService_GetSystemPrompt_NotFound(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "sys-nf")
	_, err := f.svc.GetSystemPrompt(context.Background(), f.orgID, uuid.New())
	if err == nil {
		t.Fatal("expected not found error")
	}
	svcErr, ok := err.(*service.ServiceError)
	if !ok {
		t.Fatalf("expected ServiceError, got %T", err)
	}
	if svcErr.Status != 404 {
		t.Errorf("expected 404, got %d", svcErr.Status)
	}
}

// --- Failure clusters ---

func TestDashboardService_ListFailureClusters_Empty(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "fc-empty")
	clusters, err := f.svc.ListFailureClusters(context.Background(), f.orgID)
	if err != nil {
		t.Fatalf("ListFailureClusters: %v", err)
	}
	if len(clusters) != 0 {
		t.Errorf("expected 0, got %d", len(clusters))
	}
}

func TestDashboardService_ListSessionsByCluster_Empty(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "fc-sess-empty")
	sessions, err := f.svc.ListSessionsByCluster(context.Background(), f.orgID, uuid.New())
	if err != nil {
		t.Fatalf("ListSessionsByCluster: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0, got %d", len(sessions))
	}
}

// --- Exports ---

func TestDashboardService_ExportSessions_Empty(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "exp-sess")
	rows, trunc, err := f.svc.ExportSessions(context.Background(), f.orgID, service.ExportParams{
		FromTime: time.Now().Add(-24 * time.Hour),
		ToTime:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ExportSessions: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
	if trunc {
		t.Error("expected truncated=false")
	}
}

func TestDashboardService_ExportSessions_WithData(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "exp-sess-data")
	sess := f.newSession(t)
	f.insertSpan(t, sess, 200, "gpt-4")

	rows, _, err := f.svc.ExportSessions(context.Background(), f.orgID, service.ExportParams{
		FromTime: time.Now().Add(-24 * time.Hour),
		ToTime:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ExportSessions: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least 1 row")
	}
	if rows[0].ID == "" {
		t.Error("expected non-empty session ID")
	}
}

func TestDashboardService_ExportSpans_WithData(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "exp-spans-data")
	sess := f.newSession(t)
	f.insertSpan(t, sess, 200, "gpt-4")

	rows, _, err := f.svc.ExportSpans(context.Background(), f.orgID, service.ExportParams{
		FromTime: time.Now().Add(-24 * time.Hour),
		ToTime:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least 1 row")
	}
	if rows[0].Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", rows[0].Model)
	}
}

func TestDashboardService_ExportSessions_LimitTruncates(t *testing.T) {
	truncate(t)
	ctx := context.Background()
	user := createTestUser(t, ctx, sharedQueries, "exp-trunc@example.com", "Exp Trunc")
	mailer := &testutil.MockMailer{}
	orgSvc := service.NewOrgService(sharedQueries, sharedPool, mailer, "cloud")
	org, err := orgSvc.CreateOrganization(ctx, user.ID, "Trunc Org")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	encKey := hex.EncodeToString(make([]byte, 32))
	apiKeySvc := service.NewAPIKeyService(sharedQueries, "test-hmac-secret", encKey)
	ak, err := apiKeySvc.CreateAPIKey(ctx, org.ID, "Agent", "openai", "sk-trunc", nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Insert 3 sessions with spans
	for i := 0; i < 3; i++ {
		s, _ := sharedQueries.CreateSession(ctx, db.CreateSessionParams{
			OrganizationID: org.ID,
			ApiKeyID:       ak.ID,
		})
		_, err := sharedQueries.InsertSpan(ctx, db.InsertSpanParams{
			SessionID:      s,
			OrganizationID: org.ID,
			ProviderType:   "openai",
			Model:          "gpt-4",
			DurationMs:     200,
			HttpStatus:     200,
			StartedAt:      time.Now(),
			FinishReason:   "stop",
		})
		if err != nil {
			t.Fatalf("insert span: %v", err)
		}
		_ = sharedQueries.UpdateSessionAfterSpan(ctx, db.UpdateSessionAfterSpanParams{
			ID:      s,
			CostUsd: pgtype.Numeric{Valid: false},
		})
	}

	// Limit to 2 rows -> should truncate
	svc := service.NewDashboardService(sharedQueries, sharedPool, 2)
	rows, trunc, err := svc.ExportSessions(ctx, org.ID, service.ExportParams{
		FromTime: time.Now().Add(-24 * time.Hour),
		ToTime:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ExportSessions: %v", err)
	}
	if !trunc {
		t.Error("expected truncated=true")
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

// --- ListSessions with sort (covers listSessionsSorted) ---

func TestDashboardService_ListSessions_SortByCost(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "sort-cost")
	for i := 0; i < 3; i++ {
		sess := f.newSession(t)
		f.insertSpan(t, sess, 200, "gpt-4")
	}

	result, err := f.svc.ListSessions(context.Background(), f.orgID, service.ListSessionsParams{
		SortBy:    "total_cost_usd",
		SortOrder: "desc",
	})
	if err != nil {
		t.Fatalf("ListSessions sorted: %v", err)
	}
	if len(result.Sessions) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(result.Sessions))
	}
}

func TestDashboardService_ListSessions_SortBySpanCountAsc(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "sort-span")
	for i := 0; i < 2; i++ {
		sess := f.newSession(t)
		f.insertSpan(t, sess, 200, "gpt-4")
	}

	result, err := f.svc.ListSessions(context.Background(), f.orgID, service.ListSessionsParams{
		SortBy:    "span_count",
		SortOrder: "asc",
	})
	if err != nil {
		t.Fatalf("ListSessions sorted asc: %v", err)
	}
	if len(result.Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(result.Sessions))
	}
}

// --- GetUsage ---

func TestDashboardService_GetUsage_FreePlan_Default(t *testing.T) {
	truncate(t)
	f := makeDashFixture(t, "usage-free")
	usage, err := f.svc.GetUsage(context.Background(), f.orgID)
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if usage.Plan != "free" {
		t.Errorf("expected plan free, got %s", usage.Plan)
	}
	if usage.SpansLimit != int64(service.FreeSpanLimit) {
		t.Errorf("expected free span limit, got %d", usage.SpansLimit)
	}
	if usage.SpansUsed != 0 {
		t.Errorf("expected 0 spans used, got %d", usage.SpansUsed)
	}
}
