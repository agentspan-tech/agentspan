package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/google/uuid"
)

// ExportSessions handles GET /api/orgs/{orgID}/sessions/export.
// Query params: format (csv only), level (sessions|spans), status, api_key_id,
//
//	agent_name, from, to, provider_type.
//
// Returns a streaming CSV response. If the result is truncated, a trailing
// comment line "#TRUNCATED" is appended (CSV comment — clients should check).
func (h *DashboardHandler) ExportSessions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	q := r.URL.Query()

	// --- format validation (required) ---
	format := q.Get("format")
	if format == "" {
		WriteError(w, http.StatusBadRequest, "missing_format", "format query parameter is required")
		return
	}
	if format != "csv" {
		WriteError(w, http.StatusBadRequest, "unsupported_format", "Only csv format is supported")
		return
	}

	// --- level validation (required) ---
	level := q.Get("level")
	if level == "" {
		WriteError(w, http.StatusBadRequest, "missing_level", "level query parameter is required")
		return
	}
	if level != "sessions" && level != "spans" {
		WriteError(w, http.StatusBadRequest, "invalid_level", "level must be sessions or spans")
		return
	}

	// --- filter parsing ---
	params := service.ExportParams{}

	validStatuses := map[string]bool{
		"in_progress": true, "completed": true, "failed": true,
		"abandoned": true, "completed_with_errors": true,
	}
	if status := q.Get("status"); status != "" {
		if !validStatuses[status] {
			WriteError(w, http.StatusBadRequest, "invalid_status", "status must be one of: in_progress, completed, failed, abandoned, completed_with_errors")
			return
		}
		params.Status = &status
	}

	if apiKeyIDStr := q.Get("api_key_id"); apiKeyIDStr != "" {
		id, err := uuid.Parse(apiKeyIDStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_api_key_id", "Invalid api_key_id format")
			return
		}
		params.APIKeyID = &id
	}

	if agentName := q.Get("agent_name"); agentName != "" {
		if len(agentName) > 200 {
			WriteError(w, http.StatusBadRequest, "invalid_agent_name", "agent_name must not exceed 200 characters")
			return
		}
		params.AgentName = &agentName
	}

	if providerType := q.Get("provider_type"); providerType != "" {
		params.ProviderType = &providerType
	}

	// Default time range: last 30 days.
	now := time.Now().UTC()
	params.ToTime = now
	params.FromTime = now.AddDate(0, 0, -30)

	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_from", "from must be RFC3339 format")
			return
		}
		params.FromTime = t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_to", "to must be RFC3339 format")
			return
		}
		params.ToTime = t
	}

	// --- set CSV response headers ---
	filename := fmt.Sprintf("agentorbit-%s-%s.csv", level, now.Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-AgentOrbit-Row-Limit", strconv.Itoa(h.dashboardService.ExportRowLimit()))
	w.WriteHeader(http.StatusOK)

	cw := csv.NewWriter(w)

	switch level {
	case "sessions":
		rows, truncated, err := h.dashboardService.ExportSessions(r.Context(), orgID, params)
		if err != nil {
			// Headers already sent; best we can do is flush and return.
			cw.Flush()
			return
		}

		// Write header row.
		_ = cw.Write([]string{
			"session_id", "external_id", "status", "agent_name", "api_key_name",
			"provider_types", "span_count", "total_cost_usd",
			"started_at", "last_span_at", "closed_at", "narrative",
		})

		for _, row := range rows {
			_ = cw.Write([]string{
				row.ID,
				row.ExternalID,
				row.Status,
				row.AgentName,
				row.APIKeyName,
				row.ProviderTypes,
				strconv.Itoa(int(row.SpanCount)),
				row.TotalCostUSD,
				row.StartedAt,
				row.LastSpanAt,
				row.ClosedAt,
				row.Narrative,
			})
		}

		cw.Flush()
		if truncated {
			// Append a sentinel line so clients can detect truncation.
			fmt.Fprintf(w, "# Export truncated at %d rows. Narrow your filters or date range.\n", h.dashboardService.ExportRowLimit()) //nolint:errcheck
		}

	case "spans":
		rows, truncated, err := h.dashboardService.ExportSpans(r.Context(), orgID, params)
		if err != nil {
			cw.Flush()
			return
		}

		_ = cw.Write([]string{
			"session_id", "span_id", "session_status", "agent_name", "api_key_name",
			"provider_type", "model", "input_tokens", "output_tokens", "cost_usd",
			"duration_ms", "http_status", "finish_reason",
			"started_at", "session_started_at",
		})

		for _, row := range rows {
			_ = cw.Write([]string{
				row.SessionID,
				row.SpanID,
				row.SessionStatus,
				row.AgentName,
				row.APIKeyName,
				row.ProviderType,
				row.Model,
				row.InputTokens,
				row.OutputTokens,
				row.CostUSD,
				strconv.Itoa(int(row.DurationMs)),
				strconv.Itoa(int(row.HTTPStatus)),
				row.FinishReason,
				row.StartedAt,
				row.SessionStartedAt,
			})
		}

		cw.Flush()
		if truncated {
			fmt.Fprintf(w, "# Export truncated at %d rows. Narrow your filters or date range.\n", h.dashboardService.ExportRowLimit()) //nolint:errcheck
		}
	}
}
