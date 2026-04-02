package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/agentspan/processing/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// DashboardService provides session listing, session detail, KPI stats, and daily stats
// for the dashboard REST API. All methods are organization-scoped (DAPI-01, SEC-05).
type DashboardService struct {
	queries *db.Queries
}

// NewDashboardService creates a new DashboardService.
func NewDashboardService(queries *db.Queries) *DashboardService {
	return &DashboardService{queries: queries}
}

// --- Request/Response types ---

// ListSessionsParams are the parsed query parameters for session listing.
type ListSessionsParams struct {
	CursorStartedAt *time.Time
	CursorID        *uuid.UUID
	Status          *string
	APIKeyID        *uuid.UUID
	AgentName       *string
	FromTime        *time.Time
	ToTime          *time.Time
	ProviderType    *string
	Limit           int32
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
	ID           uuid.UUID  `json:"id"`
	ProviderType string     `json:"provider_type"`
	Model        string     `json:"model"`
	Input        *string    `json:"input,omitempty"`
	Output       *string    `json:"output,omitempty"`
	InputTokens  *int32     `json:"input_tokens,omitempty"`
	OutputTokens *int32     `json:"output_tokens,omitempty"`
	CostUsd      *float64   `json:"cost_usd,omitempty"`
	DurationMs   int32      `json:"duration_ms"`
	HTTPStatus   int32      `json:"http_status"`
	FinishReason string     `json:"finish_reason"`
	StartedAt    time.Time  `json:"started_at"`
	CreatedAt    time.Time  `json:"created_at"`
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

// DailyStatsRow is one day's aggregated stats (DAPI-04).
type DailyStatsRow struct {
	Day          string  `json:"day"`
	SessionCount int64   `json:"session_count"`
	SpanCount    int64   `json:"span_count"`
	CostUsd      float64 `json:"cost_usd"`
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

	// Build sqlc params.
	dbParams := db.ListSessionsParams{
		OrgID:     orgID,
		PageLimit: limit,
	}

	// Cursor params.
	if params.CursorStartedAt != nil {
		dbParams.CursorStartedAt = sql.NullTime{Time: *params.CursorStartedAt, Valid: true}
	}
	if params.CursorID != nil {
		dbParams.CursorID = pgtype.UUID{Bytes: *params.CursorID, Valid: true}
	}

	// Optional filters.
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

	// Convert rows.
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

	// Build next cursor if we got a full page.
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
		spanItems = append(spanItems, SpanItem{
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
		})
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
			Day:          day,
			SessionCount: row.SessionCount,
			SpanCount:    row.SpanCount,
			CostUsd:      numericToFloat64(row.CostUsd),
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
