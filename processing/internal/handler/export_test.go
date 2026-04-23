//go:build integration

package handler_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ingestSpan is a helper that POSTs one span to the internal ingest endpoint
// and fails the test if the response is not 202.
// opts can override any default field in the span payload.
func ingestSpan(t *testing.T, env *testEnv, keyID, orgID string, opts map[string]interface{}) {
	t.Helper()
	span := map[string]interface{}{
		"api_key_id":      keyID,
		"organization_id": orgID,
		"provider_type":   "openai",
		"model":           "gpt-4",
		"input":           "user: Hello",
		"output":          "Hi there",
		"input_tokens":    10,
		"output_tokens":   5,
		"duration_ms":     150,
		"http_status":     200,
		"started_at":      time.Now().UTC().Format(time.RFC3339Nano),
		"finish_reason":   "stop",
	}
	for k, v := range opts {
		span[k] = v
	}
	body := jsonBody(t, span)
	req := httptest.NewRequest("POST", "/internal/spans/ingest", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", testInternalToken)
	rr := do(t, env, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("ingestSpan: expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// createAPIKeyForExport creates an API key in the given org and returns its ID.
func createAPIKeyForExport(t *testing.T, env *testEnv, token, orgID, name string) string {
	t.Helper()
	rr := do(t, env, authReq(t, "POST", "/orgs/"+orgID+"/api-keys", map[string]interface{}{
		"name": name, "provider_type": "openai", "provider_key": "sk-test",
	}, token))
	if rr.Code != http.StatusCreated {
		t.Fatalf("createAPIKeyForExport: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var key map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&key)
	return key["id"].(string)
}

// buildExportURL constructs the export endpoint URL with the given query params.
func buildExportURL(orgID string, params map[string]string) string {
	u := "/orgs/" + orgID + "/sessions/export"
	var parts []string
	for k, v := range params {
		parts = append(parts, k+"="+v)
	}
	if len(parts) > 0 {
		u += "?" + strings.Join(parts, "&")
	}
	return u
}

// parseExportCSV reads the export response body as CSV, filtering out "#" comment
// lines that the endpoint appends when truncated. Returns all rows including header.
func parseExportCSV(t *testing.T, body string) [][]string {
	t.Helper()
	lines := strings.Split(body, "\n")
	var kept []string
	for _, l := range lines {
		if !strings.HasPrefix(l, "#") {
			kept = append(kept, l)
		}
	}
	r := csv.NewReader(strings.NewReader(strings.Join(kept, "\n")))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parseExportCSV: %v\nbody: %q", err, body)
	}
	return rows
}

// --- Tests ---

// TestExportSessions_CSV verifies a successful session-level CSV export:
// correct status, headers, and CSV structure.
func TestExportSessions_CSV(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportsess@example.com", "Export User", "Password1")
	orgID := createOrg(t, env, token, "Export Sessions Org")
	keyID := createAPIKeyForExport(t, env, token, orgID, "Export Key")

	// Ingest 2 spans so at least one session is created.
	ingestSpan(t, env, keyID, orgID, nil)
	ingestSpan(t, env, keyID, orgID, nil)

	rr := do(t, env, authReq(t, "GET", buildExportURL(orgID, map[string]string{
		"format": "csv",
		"level":  "sessions",
	}), nil, token))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type: expected 'text/csv; charset=utf-8', got %q", ct)
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=") {
		t.Errorf("Content-Disposition missing or wrong: %q", cd)
	}

	if rr.Header().Get("X-AgentOrbit-Row-Limit") == "" {
		t.Error("expected X-AgentOrbit-Row-Limit header to be present")
	}

	rows := parseExportCSV(t, rr.Body.String())
	if len(rows) < 1 {
		t.Fatal("expected at least a header row")
	}

	// Verify header column names.
	wantHeader := []string{
		"session_id", "external_id", "status", "agent_name", "api_key_name",
		"provider_types", "span_count", "total_cost_usd",
		"started_at", "last_span_at", "closed_at", "narrative",
	}
	if len(rows[0]) != len(wantHeader) {
		t.Fatalf("header column count: want %d, got %d: %v", len(wantHeader), len(rows[0]), rows[0])
	}
	for i, col := range wantHeader {
		if rows[0][i] != col {
			t.Errorf("header[%d]: want %q, got %q", i, col, rows[0][i])
		}
	}

	// At least one data row should be present.
	if len(rows) < 2 {
		t.Fatal("expected at least one data row after the header")
	}
}

// TestExportSpans_CSV verifies a span-level CSV export returns span-level columns
// and the row count matches the number of ingested spans.
func TestExportSpans_CSV(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportspans@example.com", "Export Spans", "Password1")
	orgID := createOrg(t, env, token, "Export Spans Org")
	keyID := createAPIKeyForExport(t, env, token, orgID, "Spans Key")

	// Ingest exactly 3 spans with distinct session IDs so they are easy to count.
	for i := 0; i < 3; i++ {
		ingestSpan(t, env, keyID, orgID, map[string]interface{}{
			"external_session_id": "span-sess",
		})
	}

	rr := do(t, env, authReq(t, "GET", buildExportURL(orgID, map[string]string{
		"format": "csv",
		"level":  "spans",
	}), nil, token))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type: expected 'text/csv; charset=utf-8', got %q", ct)
	}

	rows := parseExportCSV(t, rr.Body.String())
	if len(rows) < 1 {
		t.Fatal("expected at least a header row")
	}

	// Verify span-level header column names.
	wantHeader := []string{
		"session_id", "span_id", "session_status", "agent_name", "api_key_name",
		"provider_type", "model", "input_tokens", "output_tokens", "cost_usd",
		"duration_ms", "http_status", "finish_reason",
		"started_at", "session_started_at",
	}
	if len(rows[0]) != len(wantHeader) {
		t.Fatalf("header column count: want %d, got %d: %v", len(wantHeader), len(rows[0]), rows[0])
	}
	for i, col := range wantHeader {
		if rows[0][i] != col {
			t.Errorf("header[%d]: want %q, got %q", i, col, rows[0][i])
		}
	}

	// 3 data rows, one per ingested span.
	dataRows := len(rows) - 1
	if dataRows != 3 {
		t.Errorf("expected 3 data rows, got %d", dataRows)
	}
}

// TestExportSessions_FiltersApplied verifies that status= filters are applied:
// only sessions with the matching status appear in the output.
func TestExportSessions_FiltersApplied(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportfilter@example.com", "Filter User", "Password1")
	orgID := createOrg(t, env, token, "Filter Org")
	keyID := createAPIKeyForExport(t, env, token, orgID, "Filter Key")

	// Ingest spans for two distinct explicit sessions.
	ingestSpan(t, env, keyID, orgID, map[string]interface{}{
		"external_session_id": "sess-completed",
	})
	ingestSpan(t, env, keyID, orgID, map[string]interface{}{
		"external_session_id": "sess-failed",
	})

	ctx := context.Background()

	// Directly update session statuses so we don't need to wait for the cron.
	_, err := sharedPool.Exec(ctx,
		"UPDATE sessions SET status = 'completed', closed_at = NOW() WHERE external_id = $1 AND organization_id = $2::uuid",
		"sess-completed", orgID,
	)
	if err != nil {
		t.Fatalf("set completed status: %v", err)
	}
	_, err = sharedPool.Exec(ctx,
		"UPDATE sessions SET status = 'failed', closed_at = NOW() WHERE external_id = $1 AND organization_id = $2::uuid",
		"sess-failed", orgID,
	)
	if err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	// Export with status=completed filter.
	rr := do(t, env, authReq(t, "GET", buildExportURL(orgID, map[string]string{
		"format": "csv",
		"level":  "sessions",
		"status": "completed",
	}), nil, token))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rows := parseExportCSV(t, rr.Body.String())
	dataRows := rows[1:] // skip header
	if len(dataRows) != 1 {
		t.Fatalf("expected 1 completed session row, got %d (rows: %v)", len(dataRows), dataRows)
	}

	// status is column index 2 in the sessions header.
	if dataRows[0][2] != "completed" {
		t.Errorf("expected status='completed', got %q", dataRows[0][2])
	}
}

// extractErrorCode parses the standard error envelope {"error":{"code":"..."}}
// and returns the code string.
func extractErrorCode(t *testing.T, body string) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("extractErrorCode: unmarshal failed: %v (body: %q)", err, body)
	}
	return resp.Error.Code
}

// TestExportSessions_MissingFormat asserts that omitting the format param returns
// 400 with error code "missing_format".
func TestExportSessions_MissingFormat(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportmissing@example.com", "Missing Fmt", "Password1")
	orgID := createOrg(t, env, token, "Missing Fmt Org")

	rr := do(t, env, authReq(t, "GET", "/orgs/"+orgID+"/sessions/export?level=sessions", nil, token))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if code := extractErrorCode(t, rr.Body.String()); code != "missing_format" {
		t.Errorf("expected error code 'missing_format', got %q", code)
	}
}

// TestExportSessions_UnsupportedFormat asserts that format=json returns 400 with
// error code "unsupported_format".
func TestExportSessions_UnsupportedFormat(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportbadformat@example.com", "Bad Fmt", "Password1")
	orgID := createOrg(t, env, token, "Bad Fmt Org")

	rr := do(t, env, authReq(t, "GET", "/orgs/"+orgID+"/sessions/export?format=json&level=sessions", nil, token))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if code := extractErrorCode(t, rr.Body.String()); code != "unsupported_format" {
		t.Errorf("expected error code 'unsupported_format', got %q", code)
	}
}

// TestExportSessions_InvalidLevel asserts that level=foo returns 400 with error
// code "invalid_level".
func TestExportSessions_InvalidLevel(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportbadlevel@example.com", "Bad Level", "Password1")
	orgID := createOrg(t, env, token, "Bad Level Org")

	rr := do(t, env, authReq(t, "GET", "/orgs/"+orgID+"/sessions/export?format=csv&level=foo", nil, token))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}

	if code := extractErrorCode(t, rr.Body.String()); code != "invalid_level" {
		t.Errorf("expected error code 'invalid_level', got %q", code)
	}
}

// TestExportSessions_DefaultDateRange verifies that the default 30-day window
// excludes sessions older than 30 days.
func TestExportSessions_DefaultDateRange(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env, "exportdates@example.com", "Date Range", "Password1")
	orgID := createOrg(t, env, token, "Date Range Org")
	keyID := createAPIKeyForExport(t, env, token, orgID, "Date Key")

	// Ingest a "recent" span (10 days ago) using an explicit session ID.
	recent := time.Now().UTC().Add(-10 * 24 * time.Hour).Format(time.RFC3339Nano)
	ingestSpan(t, env, keyID, orgID, map[string]interface{}{
		"external_session_id": "sess-recent",
		"started_at":          recent,
	})

	// Ingest an "old" span (60 days ago) using a different explicit session ID.
	old := time.Now().UTC().Add(-60 * 24 * time.Hour).Format(time.RFC3339Nano)
	ingestSpan(t, env, keyID, orgID, map[string]interface{}{
		"external_session_id": "sess-old",
		"started_at":          old,
	})

	// Backdate the old session so its started_at/last_span_at fall outside the
	// default 30-day export window.
	ctx := context.Background()
	oldTime := time.Now().UTC().Add(-60 * 24 * time.Hour)
	_, err := sharedPool.Exec(ctx,
		"UPDATE sessions SET started_at = $1, last_span_at = $1 WHERE external_id = $2 AND organization_id = $3::uuid",
		oldTime, "sess-old", orgID,
	)
	if err != nil {
		t.Fatalf("backdate old session: %v", err)
	}

	// Export without from/to — default window covers the last 30 days.
	rr := do(t, env, authReq(t, "GET", buildExportURL(orgID, map[string]string{
		"format": "csv",
		"level":  "sessions",
	}), nil, token))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	rows := parseExportCSV(t, rr.Body.String())
	dataRows := rows[1:] // skip header
	if len(dataRows) != 1 {
		t.Fatalf("expected 1 session in 30-day window, got %d (rows: %v)", len(dataRows), dataRows)
	}
}
