package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DashboardService provides session listing, session detail, KPI stats, and daily stats
// for the dashboard REST API. All methods are organization-scoped (DAPI-01, SEC-05).
type DashboardService struct {
	queries        *db.Queries
	pool           *pgxpool.Pool
	exportRowLimit int
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(queries *db.Queries, pool *pgxpool.Pool, exportRowLimit int) *DashboardService {
	return &DashboardService{queries: queries, pool: pool, exportRowLimit: exportRowLimit}
}

// ExportRowLimit returns the configured maximum export row count.
func (s *DashboardService) ExportRowLimit() int {
	return s.exportRowLimit
}

// --- Request/Response types ---

// ListSessionsParams are the parsed query parameters for session listing.
type ListSessionsParams struct {
	CursorStartedAt *time.Time
	CursorID        *uuid.UUID
	CursorSortVal   *string // for non-default sort: the sort column value from DecodeSortCursor
	Status          *string
	APIKeyID        *uuid.UUID
	AgentName       *string
	FromTime        *time.Time
	ToTime          *time.Time
	ProviderType    *string
	SortBy          string // "started_at" (default), "total_cost_usd", "span_count", "last_span_at"
	SortOrder       string // "desc" (default), "asc"
	Limit           int32
}

// ExportParams are the parsed query parameters for CSV export.
type ExportParams struct {
	Status       *string
	APIKeyID     *uuid.UUID
	AgentName    *string
	FromTime     time.Time
	ToTime       time.Time
	ProviderType *string
}


// ValidSortColumns is the allowlist of columns that can be sorted on.
var ValidSortColumns = map[string]string{
	"started_at":     "s.started_at",
	"total_cost_usd": "s.total_cost_usd",
	"span_count":     "s.span_count",
	"last_span_at":   "s.last_span_at",
}

// ValidSortOrders is the allowlist of sort directions.
var ValidSortOrders = map[string]bool{
	"asc":  true,
	"desc": true,
}

// ListSessionsResult is the paginated session list response.
type ListSessionsResult struct {
	Sessions   []SessionListItem `json:"data"`
	NextCursor *string           `json:"next_cursor,omitempty"`
}

// SessionListItem is a single session in the list response.
type SessionListItem struct {
	ID           uuid.UUID  `json:"id"`
	APIKeyID     uuid.UUID  `json:"api_key_id"`
	APIKeyName   string     `json:"api_key_name"`
	ExternalID   *string    `json:"external_id,omitempty"`
	AgentName    *string    `json:"agent_name,omitempty"`
	Status       string     `json:"status"`
	TotalCostUsd *float64   `json:"total_cost_usd"`
	SpanCount    int32      `json:"span_count"`
	StartedAt    time.Time  `json:"started_at"`
	LastSpanAt   time.Time  `json:"last_span_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
}

// SessionDetail is the full session response including all spans.
type SessionDetail struct {
	ID           uuid.UUID  `json:"id"`
	APIKeyID     uuid.UUID  `json:"api_key_id"`
	APIKeyName   string     `json:"api_key_name"`
	ExternalID   *string    `json:"external_id,omitempty"`
	AgentName    *string    `json:"agent_name,omitempty"`
	Status       string     `json:"status"`
	Narrative    *string    `json:"narrative,omitempty"`
	TotalCostUsd *float64   `json:"total_cost_usd"`
	SpanCount    int32      `json:"span_count"`
	StartedAt    time.Time  `json:"started_at"`
	LastSpanAt   time.Time  `json:"last_span_at"`
	ClosedAt     *time.Time `json:"closed_at,omitempty"`
	Spans        []SpanItem `json:"spans"`
}

// SpanItem is a single span in the session detail response.
type SpanItem struct {
	ID             uuid.UUID  `json:"id"`
	ProviderType   string     `json:"provider_type"`
	Model          string     `json:"model"`
	Input          *string    `json:"input,omitempty"`
	Output         *string    `json:"output,omitempty"`
	InputTokens    *int32     `json:"input_tokens,omitempty"`
	OutputTokens   *int32     `json:"output_tokens,omitempty"`
	CostUsd        *float64   `json:"cost_usd,omitempty"`
	DurationMs     int32      `json:"duration_ms"`
	HTTPStatus     int32      `json:"http_status"`
	FinishReason   string     `json:"finish_reason"`
	StartedAt      time.Time  `json:"started_at"`
	CreatedAt      time.Time  `json:"created_at"`
	SystemPromptID  *uuid.UUID `json:"system_prompt_id,omitempty"`
	AnomalyReason   *string    `json:"anomaly_reason,omitempty"`
	AnomalyCategory *string    `json:"anomaly_category,omitempty"`
}

// StatsResult contains KPI totals for the organization (DAPI-03).
type StatsResult struct {
	TotalSessions int64   `json:"total_sessions"`
	TotalSpans    int64   `json:"total_spans"`
	TotalCostUsd  float64 `json:"total_cost_usd"`
	AvgDurationMs float64 `json:"avg_duration_ms"`
	ErrorRate     float64 `json:"error_rate"`
}

// AgentStatsRow is per-agent (API key) stats.
type AgentStatsRow struct {
	APIKeyID      uuid.UUID `json:"api_key_id"`
	APIKeyName    string    `json:"api_key_name"`
	SessionCount  int64     `json:"session_count"`
	SpanCount     int64     `json:"span_count"`
	TotalCostUsd  float64   `json:"total_cost_usd"`
	AvgDurationMs float64   `json:"avg_duration_ms"`
	ErrorRate     float64   `json:"error_rate"`
	AvgTokenRatio float64   `json:"avg_token_ratio"`
}

// FinishReasonCount is a single row in the finish reason distribution.
type FinishReasonCount struct {
	FinishReason string `json:"finish_reason"`
	Count        int64  `json:"count"`
}

// FailureClusterItem is a single failure cluster in the list response.
type FailureClusterItem struct {
	ID           uuid.UUID `json:"id"`
	Label        string    `json:"label"`
	SessionCount int32     `json:"session_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ClusterSessionItem is a session within a failure cluster.
type ClusterSessionItem struct {
	ID         uuid.UUID  `json:"id"`
	APIKeyID   uuid.UUID  `json:"api_key_id"`
	APIKeyName string     `json:"api_key_name"`
	AgentName  *string    `json:"agent_name,omitempty"`
	Status     string     `json:"status"`
	SpanCount  int32      `json:"span_count"`
	StartedAt  time.Time  `json:"started_at"`
	LastSpanAt time.Time  `json:"last_span_at"`
	ClosedAt   *time.Time `json:"closed_at,omitempty"`
}

// DailyStatsRow is one day's aggregated stats (DAPI-04).
type DailyStatsRow struct {
	Day             string  `json:"day"`
	SessionCount    int64   `json:"session_count"`
	SpanCount       int64   `json:"span_count"`
	CostUsd         float64 `json:"cost_usd"`
	CompletedCount  int64   `json:"completed_count"`
	WithErrorsCount int64   `json:"with_errors_count"`
	FailedCount     int64   `json:"failed_count"`
	AbandonedCount  int64   `json:"abandoned_count"`
	InProgressCount int64   `json:"in_progress_count"`
}

// SystemPromptListItem is a single system prompt in the list response.
type SystemPromptListItem struct {
	ID             uuid.UUID  `json:"id"`
	ShortUID       string     `json:"short_uid"`
	ContentPreview string     `json:"content_preview"`
	ContentLength  int32      `json:"content_length"`
	SpanCount      int32      `json:"span_count"`
	SessionCount   int32      `json:"session_count"`
	CreatedAt      time.Time  `json:"created_at"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
}

// SystemPromptDetail is the full system prompt response.
type SystemPromptDetail struct {
	ID           uuid.UUID  `json:"id"`
	ShortUID     string     `json:"short_uid"`
	Content      string     `json:"content"`
	SpanCount    int32      `json:"span_count"`
	SessionCount int32      `json:"session_count"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

// --- Cursor encoding ---

type cursorPayload struct {
	Ts string `json:"ts"`
	ID string `json:"id"`
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	payload := cursorPayload{
		Ts: t.Format(time.RFC3339Nano),
		ID: id.String(),
	}
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeCursor parses a base64-encoded cursor into its components.
func DecodeCursor(cursor string) (*time.Time, *uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor encoding")
	}
	var payload cursorPayload
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, payload.Ts)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor timestamp")
	}
	id, err := uuid.Parse(payload.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor id")
	}
	return &t, &id, nil
}

// sortCursorPayload encodes the sort value + ID for keyset pagination on any sort column.
type sortCursorPayload struct {
	Val string `json:"v"`
	ID  string `json:"id"`
}

func encodeSortCursor(val string, id uuid.UUID) string {
	p := sortCursorPayload{Val: val, ID: id.String()}
	b, _ := json.Marshal(p)
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeSortCursor parses a sort cursor into its value string and UUID.
func DecodeSortCursor(cursor string) (string, *uuid.UUID, error) {
	b, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", nil, fmt.Errorf("invalid cursor encoding")
	}
	var p sortCursorPayload
	if err := json.Unmarshal(b, &p); err != nil {
		return "", nil, fmt.Errorf("invalid cursor format")
	}
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return "", nil, fmt.Errorf("invalid cursor id")
	}
	return p.Val, &id, nil
}

// --- pgtype.Numeric -> *float64 helper ---

func numericToFloat64Ptr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f8, err := n.Float64Value()
	if err != nil || !f8.Valid {
		return nil
	}
	return &f8.Float64
}

func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f8, err := n.Float64Value()
	if err != nil || !f8.Valid {
		return 0
	}
	return f8.Float64
}

// --- sql.NullTime -> *time.Time helper ---

func nullTimeToPtr(nt sql.NullTime) *time.Time {
	if nt.Valid {
		return &nt.Time
	}
	return nil
}

// --- Service methods ---

// ListSessions returns a paginated, filtered list of sessions for the organization (DAPI-02).
func (s *DashboardService) ListSessions(ctx context.Context, orgID uuid.UUID, params ListSessionsParams) (*ListSessionsResult, error) {
	// Default limit to 50, max 100.
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// Resolve sort defaults.
	sortBy := params.SortBy
	if sortBy == "" {
		sortBy = "started_at"
	}
	sortOrder := params.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Default sort uses the sqlc-generated query (keyset cursor on started_at DESC).
	if sortBy == "started_at" && sortOrder == "desc" {
		return s.listSessionsDefault(ctx, orgID, params, limit)
	}
	return s.listSessionsSorted(ctx, orgID, params, sortBy, sortOrder, limit)
}

// listSessionsDefault uses the sqlc-generated query for the default sort (started_at DESC).
func (s *DashboardService) listSessionsDefault(ctx context.Context, orgID uuid.UUID, params ListSessionsParams, limit int32) (*ListSessionsResult, error) {
	dbParams := db.ListSessionsParams{
		OrgID:     orgID,
		PageLimit: limit,
	}

	if params.CursorStartedAt != nil {
		dbParams.CursorStartedAt = sql.NullTime{Time: *params.CursorStartedAt, Valid: true}
	}
	if params.CursorID != nil {
		dbParams.CursorID = pgtype.UUID{Bytes: *params.CursorID, Valid: true}
	}

	dbParams.Status = params.Status
	dbParams.AgentName = params.AgentName
	dbParams.ProviderType = params.ProviderType

	if params.APIKeyID != nil {
		dbParams.ApiKeyID = pgtype.UUID{Bytes: *params.APIKeyID, Valid: true}
	}
	if params.FromTime != nil {
		dbParams.FromTime = sql.NullTime{Time: *params.FromTime, Valid: true}
	}
	if params.ToTime != nil {
		dbParams.ToTime = sql.NullTime{Time: *params.ToTime, Valid: true}
	}

	rows, err := s.queries.ListSessions(ctx, dbParams)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	sessions := make([]SessionListItem, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, SessionListItem{
			ID:           row.ID,
			APIKeyID:     row.ApiKeyID,
			APIKeyName:   row.ApiKeyName,
			ExternalID:   row.ExternalID,
			AgentName:    row.AgentName,
			Status:       row.Status,
			TotalCostUsd: numericToFloat64Ptr(row.TotalCostUsd),
			SpanCount:    row.SpanCount,
			StartedAt:    row.StartedAt,
			LastSpanAt:   row.LastSpanAt,
			ClosedAt:     nullTimeToPtr(row.ClosedAt),
		})
	}

	var nextCursor *string
	if int32(len(rows)) == limit && len(rows) > 0 { //nolint:gosec // limit is bounded to <=100, no overflow risk
		last := rows[len(rows)-1]
		c := encodeCursor(last.StartedAt, last.ID)
		nextCursor = &c
	}

	return &ListSessionsResult{
		Sessions:   sessions,
		NextCursor: nextCursor,
	}, nil
}

// listSessionsSorted handles non-default sort orders using a dynamic query.
// Sort column and direction come from ValidSortColumns/ValidSortOrders allowlists,
// never from raw user input — safe against SQL injection.
func (s *DashboardService) listSessionsSorted(ctx context.Context, orgID uuid.UUID, params ListSessionsParams, sortBy, sortOrder string, limit int32) (*ListSessionsResult, error) {
	sqlCol := ValidSortColumns[sortBy]
	if sqlCol == "" {
		return nil, fmt.Errorf("invalid sort_by: %s", sortBy)
	}

	// Determine cursor comparison operator: DESC uses <, ASC uses >.
	cursorOp := "<"
	if sortOrder == "asc" {
		cursorOp = ">"
	}

	// Build query with parameterized values and allowlist-injected ORDER BY.
	// $1 = org_id, $2 = cursor_value (text), $3 = cursor_id (uuid),
	// $4 = status, $5 = api_key_id, $6 = agent_name, $7 = from_time, $8 = to_time,
	// $9 = provider_type, $10 = page_limit
	query := fmt.Sprintf(`
SELECT s.id, s.organization_id, s.api_key_id, s.external_id, s.agent_name,
       s.status, s.total_cost_usd, s.span_count,
       s.started_at, s.last_span_at, s.closed_at, s.created_at, s.updated_at,
       k.name AS api_key_name
FROM sessions s
JOIN api_keys k ON k.id = s.api_key_id
WHERE s.organization_id = $1
  AND ($2::text IS NULL
       OR %[1]s %[2]s $2::%[3]s
       OR (%[1]s = $2::%[3]s AND s.id %[2]s $3::uuid))
  AND ($4::text IS NULL OR s.status = $4)
  AND ($5::uuid IS NULL OR s.api_key_id = $5)
  AND ($6::text IS NULL OR s.agent_name = $6)
  AND ($7::timestamptz IS NULL OR s.started_at >= $7)
  AND ($8::timestamptz IS NULL OR s.started_at <= $8)
  AND ($9::text IS NULL OR EXISTS (SELECT 1 FROM spans WHERE spans.session_id = s.id AND spans.provider_type = $9))
ORDER BY %[1]s %[4]s, s.id %[4]s
LIMIT $10`,
		sqlCol,                       // %[1]s — sort column
		cursorOp,                     // %[2]s — < or >
		sortColumnCastType(sortBy),   // %[3]s — cast type for cursor value
		sortOrder,                    // %[4]s — ASC or DESC
	)

	// Build args.
	var cursorVal, cursorID any
	if params.CursorStartedAt != nil && params.CursorID != nil {
		// For the default sort, the caller passes CursorStartedAt/CursorID from DecodeCursor.
		// For custom sorts, the handler will use DecodeSortCursor and set CursorSortVal/CursorID.
		// We handle both cases: if CursorSortVal is set, use it; otherwise derive from CursorStartedAt.
		cursorVal = params.CursorStartedAt.Format(time.RFC3339Nano)
		cursorID = *params.CursorID
	} else if params.CursorSortVal != nil && params.CursorID != nil {
		cursorVal = *params.CursorSortVal
		cursorID = *params.CursorID
	}

	args := []any{
		orgID,                 // $1
		cursorVal,             // $2
		cursorID,              // $3
		nilIfEmpty(params.Status),       // $4
		nilIfEmptyUUID(params.APIKeyID), // $5
		nilIfEmpty(params.AgentName),    // $6
		nilIfEmptyTime(params.FromTime), // $7
		nilIfEmptyTime(params.ToTime),   // $8
		nilIfEmpty(params.ProviderType), // $9
		limit,                 // $10
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions sorted: %w", err)
	}
	defer rows.Close()

	sessions := make([]SessionListItem, 0, limit)
	for rows.Next() {
		var (
			id             uuid.UUID
			orgIDOut       uuid.UUID
			apiKeyID       uuid.UUID
			externalID     *string
			agentName      *string
			status         string
			totalCostUsd   pgtype.Numeric
			spanCount      int32
			startedAt      time.Time
			lastSpanAt     time.Time
			closedAt       sql.NullTime
			createdAt      time.Time
			updatedAt      time.Time
			apiKeyName     string
		)
		if err := rows.Scan(
			&id, &orgIDOut, &apiKeyID, &externalID, &agentName,
			&status, &totalCostUsd, &spanCount,
			&startedAt, &lastSpanAt, &closedAt, &createdAt, &updatedAt,
			&apiKeyName,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		_ = orgIDOut
		_ = createdAt
		_ = updatedAt
		sessions = append(sessions, SessionListItem{
			ID:           id,
			APIKeyID:     apiKeyID,
			APIKeyName:   apiKeyName,
			ExternalID:   externalID,
			AgentName:    agentName,
			Status:       status,
			TotalCostUsd: numericToFloat64Ptr(totalCostUsd),
			SpanCount:    spanCount,
			StartedAt:    startedAt,
			LastSpanAt:   lastSpanAt,
			ClosedAt:     nullTimeToPtr(closedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions sorted rows: %w", err)
	}

	var nextCursor *string
	if int32(len(sessions)) == limit && len(sessions) > 0 { //nolint:gosec // limit is bounded to <=100, no overflow risk
		last := sessions[len(sessions)-1]
		val := sortCursorValue(last, sortBy)
		c := encodeSortCursor(val, last.ID)
		nextCursor = &c
	}

	return &ListSessionsResult{
		Sessions:   sessions,
		NextCursor: nextCursor,
	}, nil
}

// sortColumnCastType returns the PostgreSQL cast type for cursor value comparison.
func sortColumnCastType(sortBy string) string {
	switch sortBy {
	case "total_cost_usd":
		return "numeric"
	case "span_count":
		return "int"
	case "started_at", "last_span_at":
		return "timestamptz"
	default:
		return "text"
	}
}

// sortCursorValue extracts the cursor value string for the given sort column.
func sortCursorValue(s SessionListItem, sortBy string) string {
	switch sortBy {
	case "total_cost_usd":
		if s.TotalCostUsd != nil {
			return fmt.Sprintf("%f", *s.TotalCostUsd)
		}
		return "0"
	case "span_count":
		return fmt.Sprintf("%d", s.SpanCount)
	case "last_span_at":
		return s.LastSpanAt.Format(time.RFC3339Nano)
	default: // started_at
		return s.StartedAt.Format(time.RFC3339Nano)
	}
}

func nilIfEmpty(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func nilIfEmptyUUID(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return *u
}

func nilIfEmptyTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// GetSession returns full session detail including all spans (DAPI-02).
func (s *DashboardService) GetSession(ctx context.Context, orgID, sessionID uuid.UUID) (*SessionDetail, error) {
	row, err := s.queries.GetSessionByID(ctx, db.GetSessionByIDParams{
		ID:             sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ServiceError{Status: 404, Code: "not_found", Message: "Session not found"}
		}
		return nil, fmt.Errorf("get session: %w", err)
	}

	spans, err := s.queries.GetSpansBySessionID(ctx, db.GetSpansBySessionIDParams{
		SessionID:      sessionID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("get session spans: %w", err)
	}

	// Convert spans.
	spanItems := make([]SpanItem, 0, len(spans))
	for _, sp := range spans {
		item := SpanItem{
			ID:           sp.ID,
			ProviderType: sp.ProviderType,
			Model:        sp.Model,
			Input:        sp.Input,
			Output:       sp.Output,
			InputTokens:  sp.InputTokens,
			OutputTokens: sp.OutputTokens,
			CostUsd:      numericToFloat64Ptr(sp.CostUsd),
			DurationMs:   sp.DurationMs,
			HTTPStatus:   sp.HttpStatus,
			FinishReason: sp.FinishReason,
			StartedAt:    sp.StartedAt,
			CreatedAt:    sp.CreatedAt,
		}
		if sp.SystemPromptID.Valid {
			id := sp.SystemPromptID.Bytes
			uid := uuid.UUID(id)
			item.SystemPromptID = &uid
		}
		item.AnomalyReason = sp.AnomalyReason
		item.AnomalyCategory = sp.AnomalyCategory
		spanItems = append(spanItems, item)
	}

	return &SessionDetail{
		ID:           row.ID,
		APIKeyID:     row.ApiKeyID,
		APIKeyName:   row.ApiKeyName,
		ExternalID:   row.ExternalID,
		AgentName:    row.AgentName,
		Status:       row.Status,
		Narrative:    row.Narrative,
		TotalCostUsd: numericToFloat64Ptr(row.TotalCostUsd),
		SpanCount:    row.SpanCount,
		StartedAt:    row.StartedAt,
		LastSpanAt:   row.LastSpanAt,
		ClosedAt:     nullTimeToPtr(row.ClosedAt),
		Spans:        spanItems,
	}, nil
}

// GetStats returns KPI totals for the organization within a time range (DAPI-03).
func (s *DashboardService) GetStats(ctx context.Context, orgID uuid.UUID, from, to time.Time) (*StatsResult, error) {
	row, err := s.queries.GetOrgStats(ctx, db.GetOrgStatsParams{
		OrganizationID: orgID,
		FromTime:       from,
		ToTime:         to,
	})
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return &StatsResult{
		TotalSessions: row.TotalSessions,
		TotalSpans:    row.TotalSpans,
		TotalCostUsd:  numericToFloat64(row.TotalCostUsd),
		AvgDurationMs: row.AvgDurationMs,
		ErrorRate:     row.ErrorRate,
	}, nil
}

// GetAgentStats returns per-agent (API key) stats for the organization within a time range.
func (s *DashboardService) GetAgentStats(ctx context.Context, orgID uuid.UUID, from, to time.Time) ([]AgentStatsRow, error) {
	rows, err := s.queries.GetAgentStats(ctx, db.GetAgentStatsParams{
		OrganizationID: orgID,
		FromTime:       from,
		ToTime:         to,
	})
	if err != nil {
		return nil, fmt.Errorf("get agent stats: %w", err)
	}

	result := make([]AgentStatsRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, AgentStatsRow{
			APIKeyID:      row.ApiKeyID,
			APIKeyName:    row.ApiKeyName,
			SessionCount:  row.SessionCount,
			SpanCount:     row.SpanCount,
			TotalCostUsd:  numericToFloat64(row.TotalCostUsd),
			AvgDurationMs: row.AvgDurationMs,
			ErrorRate:     row.ErrorRate,
			AvgTokenRatio: row.AvgTokenRatio,
		})
	}

	return result, nil
}

// GetDailyStats returns per-day breakdown of sessions, spans, and cost (DAPI-04).
func (s *DashboardService) GetDailyStats(ctx context.Context, orgID uuid.UUID, days int32) ([]DailyStatsRow, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	rows, err := s.queries.GetOrgDailyStats(ctx, db.GetOrgDailyStatsParams{
		OrganizationID: orgID,
		Days:           days,
	})
	if err != nil {
		return nil, fmt.Errorf("get daily stats: %w", err)
	}

	result := make([]DailyStatsRow, 0, len(rows))
	for _, row := range rows {
		day := ""
		if row.Day.Valid {
			day = row.Day.Time.Format("2006-01-02")
		}
		result = append(result, DailyStatsRow{
			Day:             day,
			SessionCount:    row.SessionCount,
			SpanCount:       row.SpanCount,
			CostUsd:         numericToFloat64(row.CostUsd),
			CompletedCount:  row.CompletedCount,
			WithErrorsCount: row.WithErrorsCount,
			FailedCount:     row.FailedCount,
			AbandonedCount:  row.AbandonedCount,
			InProgressCount: row.InProgressCount,
		})
	}

	return result, nil
}

// GetFinishReasonDistribution returns the distribution of finish reasons for spans in the time range.
func (s *DashboardService) GetFinishReasonDistribution(ctx context.Context, orgID uuid.UUID, from, to time.Time) ([]FinishReasonCount, error) {
	rows, err := s.queries.GetFinishReasonDistribution(ctx, db.GetFinishReasonDistributionParams{
		OrganizationID: orgID,
		FromTime:       from,
		ToTime:         to,
	})
	if err != nil {
		return nil, fmt.Errorf("get finish reason distribution: %w", err)
	}

	result := make([]FinishReasonCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, FinishReasonCount{
			FinishReason: row.FinishReason,
			Count:        row.Count,
		})
	}

	return result, nil
}

// ListSystemPrompts returns all system prompts for the organization with usage stats.
func (s *DashboardService) ListSystemPrompts(ctx context.Context, orgID uuid.UUID) ([]SystemPromptListItem, error) {
	rows, err := s.queries.ListSystemPrompts(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list system prompts: %w", err)
	}

	result := make([]SystemPromptListItem, 0, len(rows))
	for _, row := range rows {
		item := SystemPromptListItem{
			ID:             row.ID,
			ShortUID:       row.ShortUid,
			ContentPreview: row.ContentPreview,
			ContentLength:  row.ContentLength,
			SpanCount:      row.SpanCount,
			SessionCount:   row.SessionCount,
			CreatedAt:      row.CreatedAt,
		}
		if !row.LastSeenAt.IsZero() {
			item.LastSeenAt = &row.LastSeenAt
		}
		result = append(result, item)
	}

	return result, nil
}

// GetSystemPrompt returns a single system prompt with full content and usage stats.
func (s *DashboardService) GetSystemPrompt(ctx context.Context, orgID, promptID uuid.UUID) (*SystemPromptDetail, error) {
	row, err := s.queries.GetSystemPromptByID(ctx, db.GetSystemPromptByIDParams{
		ID:             promptID,
		OrganizationID: orgID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ServiceError{Status: 404, Code: "not_found", Message: "System prompt not found"}
		}
		return nil, fmt.Errorf("get system prompt: %w", err)
	}

	detail := &SystemPromptDetail{
		ID:           row.ID,
		ShortUID:     row.ShortUid,
		Content:      row.Content,
		SpanCount:    row.SpanCount,
		SessionCount: row.SessionCount,
		CreatedAt:    row.CreatedAt,
	}
	if !row.LastSeenAt.IsZero() {
		detail.LastSeenAt = &row.LastSeenAt
	}

	return detail, nil
}

// ListFailureClusters returns all failure clusters for the organization.
func (s *DashboardService) ListFailureClusters(ctx context.Context, orgID uuid.UUID) ([]FailureClusterItem, error) {
	rows, err := s.queries.ListFailureClusters(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list failure clusters: %w", err)
	}

	items := make([]FailureClusterItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, FailureClusterItem{
			ID:           r.ID,
			Label:        r.Label,
			SessionCount: r.SessionCount,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
		})
	}
	return items, nil
}

// ListSessionsByCluster returns sessions linked to a specific failure cluster.
func (s *DashboardService) ListSessionsByCluster(ctx context.Context, orgID, clusterID uuid.UUID) ([]ClusterSessionItem, error) {
	rows, err := s.queries.ListSessionsByCluster(ctx, db.ListSessionsByClusterParams{
		FailureClusterID: clusterID,
		OrganizationID:   orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions by cluster: %w", err)
	}

	items := make([]ClusterSessionItem, 0, len(rows))
	for _, r := range rows {
		item := ClusterSessionItem{
			ID:         r.ID,
			APIKeyID:   r.ApiKeyID,
			APIKeyName: r.ApiKeyName,
			AgentName:  r.AgentName,
			Status:     r.Status,
			SpanCount:  r.SpanCount,
			StartedAt:  r.StartedAt,
			LastSpanAt: r.LastSpanAt,
		}
		if r.ClosedAt.Valid {
			item.ClosedAt = &r.ClosedAt.Time
		}
		items = append(items, item)
	}
	return items, nil
}

// --- Export helpers ---

// ptrOrEmpty returns the string value of a *string or "" if nil.
func ptrOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// nullTimeToString formats a sql.NullTime as RFC3339 or "" if not valid.
func nullTimeToString(nt sql.NullTime) string {
	if !nt.Valid {
		return ""
	}
	return nt.Time.Format(time.RFC3339)
}

// numericToString converts a pgtype.Numeric to a decimal string, or "" if invalid.
func numericToString(n pgtype.Numeric) string {
	if !n.Valid {
		return ""
	}
	f8, err := n.Float64Value()
	if err != nil || !f8.Valid {
		return ""
	}
	return fmt.Sprintf("%.10f", f8.Float64)
}

// nullNumericToString converts a pgtype.Numeric to a decimal string, or "" if invalid.
func nullNumericToString(n pgtype.Numeric) string {
	return numericToString(n)
}

// nullInt32ToString converts a *int32 to a string or "" if nil.
func nullInt32ToString(v *int32) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

// uuidToNullable returns the UUID string or "" for a uuid.UUID.
func uuidToNullable(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}

// --- Export service methods ---

// SessionExportRow is one row of the sessions CSV export.
type SessionExportRow struct {
	ID            string
	ExternalID    string
	Status        string
	AgentName     string
	APIKeyName    string
	ProviderTypes string
	SpanCount     int32
	TotalCostUSD  string
	StartedAt     string
	LastSpanAt    string
	ClosedAt      string
	Narrative     string
}

// SpanExportRow is one row of the spans CSV export.
type SpanExportRow struct {
	SessionID        string
	SpanID           string
	SessionStatus    string
	AgentName        string
	APIKeyName       string
	ProviderType     string
	Model            string
	InputTokens      string
	OutputTokens     string
	CostUSD          string
	DurationMs       int32
	HTTPStatus       int32
	FinishReason     string
	StartedAt        string
	SessionStartedAt string
}

// ExportSessions fetches sessions for CSV export, respecting the row limit.
func (s *DashboardService) ExportSessions(ctx context.Context, orgID uuid.UUID, params ExportParams) ([]SessionExportRow, bool, error) {
	var apiKeyID pgtype.UUID
	if params.APIKeyID != nil {
		apiKeyID = pgtype.UUID{Bytes: *params.APIKeyID, Valid: true}
	}

	rows, err := s.queries.ExportSessions(ctx, db.ExportSessionsParams{
		OrgID:        orgID,
		Status:       params.Status,
		ApiKeyID:     apiKeyID,
		AgentName:    params.AgentName,
		FromTime:     params.FromTime,
		ToTime:       params.ToTime,
		ProviderType: params.ProviderType,
	})
	if err != nil {
		return nil, false, fmt.Errorf("export sessions: %w", err)
	}

	truncated := false
	limit := s.exportRowLimit
	if limit <= 0 {
		limit = 100000
	}
	if len(rows) >= limit {
		rows = rows[:limit]
		truncated = true
	}

	out := make([]SessionExportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SessionExportRow{
			ID:            r.ID.String(),
			ExternalID:    ptrOrEmpty(r.ExternalID),
			Status:        r.Status,
			AgentName:     ptrOrEmpty(r.AgentName),
			APIKeyName:    r.ApiKeyName,
			ProviderTypes: fmt.Sprintf("%v", r.ProviderTypes),
			SpanCount:     r.SpanCount,
			TotalCostUSD:  numericToString(r.TotalCostUsd),
			StartedAt:     r.StartedAt.Format(time.RFC3339),
			LastSpanAt:    r.LastSpanAt.Format(time.RFC3339),
			ClosedAt:      nullTimeToString(r.ClosedAt),
			Narrative:     ptrOrEmpty(r.Narrative),
		})
	}
	return out, truncated, nil
}

// ExportSpans fetches spans for CSV export, respecting the row limit.
// Returns the rows, a truncated flag, and an error.
func (s *DashboardService) ExportSpans(ctx context.Context, orgID uuid.UUID, params ExportParams) ([]SpanExportRow, bool, error) {
	var apiKeyID pgtype.UUID
	if params.APIKeyID != nil {
		apiKeyID = pgtype.UUID{Bytes: *params.APIKeyID, Valid: true}
	}

	rows, err := s.queries.ExportSpans(ctx, db.ExportSpansParams{
		OrgID:        orgID,
		Status:       params.Status,
		ApiKeyID:     apiKeyID,
		AgentName:    params.AgentName,
		FromTime:     params.FromTime,
		ToTime:       params.ToTime,
		ProviderType: params.ProviderType,
	})
	if err != nil {
		return nil, false, fmt.Errorf("export spans: %w", err)
	}

	truncated := false
	limit := s.exportRowLimit
	if limit <= 0 {
		limit = 100000
	}
	if len(rows) >= limit {
		rows = rows[:limit]
		truncated = true
	}

	out := make([]SpanExportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, SpanExportRow{
			SessionID:        r.SessionID.String(),
			SpanID:           r.SpanID.String(),
			SessionStatus:    r.SessionStatus,
			AgentName:        ptrOrEmpty(r.AgentName),
			APIKeyName:       r.ApiKeyName,
			ProviderType:     r.ProviderType,
			Model:            r.Model,
			InputTokens:      nullInt32ToString(r.InputTokens),
			OutputTokens:     nullInt32ToString(r.OutputTokens),
			CostUSD:          nullNumericToString(r.CostUsd),
			DurationMs:       r.DurationMs,
			HTTPStatus:       r.HttpStatus,
			FinishReason:     r.FinishReason,
			StartedAt:        r.StartedAt.Format(time.RFC3339),
			SessionStartedAt: r.SessionStartedAt.Format(time.RFC3339),
		})
	}
	return out, truncated, nil
}

// --- Usage & Test Span ---

// UsageResponse is the response for GET /api/orgs/{orgID}/usage.
type UsageResponse struct {
	SpansUsed   int64  `json:"spans_used"`
	SpansLimit  int64  `json:"spans_limit"`
	Plan        string `json:"plan"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
}

// GetUsage returns the current month's span usage for the organization.
func (s *DashboardService) GetUsage(ctx context.Context, orgID uuid.UUID) (*UsageResponse, error) {
	org, err := s.queries.GetOrganizationByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("get usage: get org: %w", err)
	}

	count, err := s.queries.CountSpansThisMonth(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("get usage: count spans: %w", err)
	}

	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0).Add(-time.Second)

	var limit int64
	if org.Plan == "free" {
		limit = FreeSpanLimit
	}

	return &UsageResponse{
		SpansUsed:   count,
		SpansLimit:  limit,
		Plan:        org.Plan,
		PeriodStart: periodStart.Format(time.RFC3339),
		PeriodEnd:   periodEnd.Format(time.RFC3339),
	}, nil
}

