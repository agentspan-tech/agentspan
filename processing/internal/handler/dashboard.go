package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// DashboardHandler handles HTTP requests for the dashboard REST API.
// All endpoints are org-scoped via RequireOrg middleware applied upstream (DAPI-01, SEC-05).
type DashboardHandler struct {
	dashboardService *service.DashboardService
}

// NewDashboardHandler creates a new DashboardHandler.
func NewDashboardHandler(dashboardService *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{dashboardService: dashboardService}
}

// Routes returns a chi.Router with all dashboard endpoints.
// Must be mounted under /api/orgs/{orgID}/ with RequireOrg applied upstream.
func (h *DashboardHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/sessions", h.ListSessions)
	r.Get("/sessions/export", h.ExportSessions)
	r.Get("/sessions/{sessionID}", h.GetSession)
	r.Get("/stats", h.GetStats)
	r.Get("/stats/agents", h.GetAgentStats)
	r.Get("/stats/daily", h.GetDailyStats)
	r.Get("/stats/finish-reasons", h.GetFinishReasonDistribution)
	r.Get("/system-prompts", h.ListSystemPrompts)
	r.Get("/system-prompts/{promptID}", h.GetSystemPrompt)
	r.Get("/failure-clusters", h.ListFailureClusters)
	r.Get("/failure-clusters/{clusterID}/sessions", h.ListClusterSessions)
	r.Get("/usage", h.GetUsage)
	return r
}

// ListSessions handles GET /api/orgs/{orgID}/sessions.
// Query params: cursor, status, api_key_id, agent_name, from, to, provider_type, sort_by, sort_order, limit.
func (h *DashboardHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	q := r.URL.Query()

	params := service.ListSessionsParams{}

	// Parse sort params (needed before cursor parsing to know cursor format).
	if sortBy := q.Get("sort_by"); sortBy != "" {
		if _, ok := service.ValidSortColumns[sortBy]; !ok {
			WriteError(w, http.StatusBadRequest, "invalid_sort_by", "sort_by must be one of: started_at, total_cost_usd, span_count, last_span_at")
			return
		}
		params.SortBy = sortBy
	}
	if sortOrder := q.Get("sort_order"); sortOrder != "" {
		if !service.ValidSortOrders[sortOrder] {
			WriteError(w, http.StatusBadRequest, "invalid_sort_order", "sort_order must be one of: asc, desc")
			return
		}
		params.SortOrder = sortOrder
	}

	// Parse cursor — format depends on whether custom sort is active.
	isCustomSort := params.SortBy != "" && !(params.SortBy == "started_at" && (params.SortOrder == "" || params.SortOrder == "desc"))
	if cursor := q.Get("cursor"); cursor != "" {
		if isCustomSort {
			val, id, err := service.DecodeSortCursor(cursor)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
				return
			}
			params.CursorSortVal = &val
			params.CursorID = id
		} else {
			ts, id, err := service.DecodeCursor(cursor)
			if err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_cursor", "Invalid pagination cursor")
				return
			}
			params.CursorStartedAt = ts
			params.CursorID = id
		}
	}

	// Optional filters.
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
	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_from", "from must be RFC3339 format")
			return
		}
		params.FromTime = &t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_to", "to must be RFC3339 format")
			return
		}
		params.ToTime = &t
	}
	if providerType := q.Get("provider_type"); providerType != "" {
		params.ProviderType = &providerType
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			WriteError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		if limit > 100 {
			limit = 100
		}
		params.Limit = int32(limit) //nolint:gosec // limit is bounded to [1, 100]
	}

	result, err := h.dashboardService.ListSessions(r.Context(), orgID, params)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// GetSession handles GET /api/orgs/{orgID}/sessions/{sessionID}.
func (h *DashboardHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	sessionID, err := uuid.Parse(chi.URLParam(r, "sessionID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_session_id", "Invalid session ID format")
		return
	}

	session, err := h.dashboardService.GetSession(r.Context(), orgID, sessionID)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get session")
		return
	}

	WriteJSON(w, http.StatusOK, session)
}

// GetStats handles GET /api/orgs/{orgID}/stats.
// Query params: from, to (default: last 30 days).
func (h *DashboardHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	q := r.URL.Query()

	// Default: last 30 days.
	now := time.Now()
	from := now.AddDate(0, 0, -30)
	to := now

	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_from", "from must be RFC3339 format")
			return
		}
		from = t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_to", "to must be RFC3339 format")
			return
		}
		to = t
	}

	stats, err := h.dashboardService.GetStats(r.Context(), orgID, from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get stats")
		return
	}

	WriteJSON(w, http.StatusOK, stats)
}

// GetAgentStats handles GET /api/orgs/{orgID}/stats/agents.
// Query params: from, to (default: last 30 days).
func (h *DashboardHandler) GetAgentStats(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	q := r.URL.Query()
	now := time.Now()
	from := now.AddDate(0, 0, -30)
	to := now

	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_from", "from must be RFC3339 format")
			return
		}
		from = t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_to", "to must be RFC3339 format")
			return
		}
		to = t
	}

	result, err := h.dashboardService.GetAgentStats(r.Context(), orgID, from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get agent stats")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// GetDailyStats handles GET /api/orgs/{orgID}/stats/daily.
// Query params: days (default 30, max 365).
func (h *DashboardHandler) GetDailyStats(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	days := int32(30)
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		d, err := strconv.Atoi(daysStr)
		if err != nil || d < 1 {
			WriteError(w, http.StatusBadRequest, "invalid_days", "days must be a positive integer")
			return
		}
		if d > 365 {
			d = 365
		}
		days = int32(d) //nolint:gosec // d is bounded to [1, 365]
	}

	result, err := h.dashboardService.GetDailyStats(r.Context(), orgID, days)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get daily stats")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// GetFinishReasonDistribution handles GET /api/orgs/{orgID}/stats/finish-reasons.
// Query params: from, to (default: last 30 days).
func (h *DashboardHandler) GetFinishReasonDistribution(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	q := r.URL.Query()
	now := time.Now()
	from := now.AddDate(0, 0, -30)
	to := now

	if fromStr := q.Get("from"); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_from", "from must be RFC3339 format")
			return
		}
		from = t
	}
	if toStr := q.Get("to"); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_to", "to must be RFC3339 format")
			return
		}
		to = t
	}

	result, err := h.dashboardService.GetFinishReasonDistribution(r.Context(), orgID, from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get finish reason distribution")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// ListSystemPrompts handles GET /api/orgs/{orgID}/system-prompts.
func (h *DashboardHandler) ListSystemPrompts(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	result, err := h.dashboardService.ListSystemPrompts(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list system prompts")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// GetSystemPrompt handles GET /api/orgs/{orgID}/system-prompts/{promptID}.
func (h *DashboardHandler) GetSystemPrompt(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	promptID, err := uuid.Parse(chi.URLParam(r, "promptID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_prompt_id", "Invalid system prompt ID format")
		return
	}

	prompt, err := h.dashboardService.GetSystemPrompt(r.Context(), orgID, promptID)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get system prompt")
		return
	}

	WriteJSON(w, http.StatusOK, prompt)
}

// ListFailureClusters handles GET /api/orgs/{orgID}/failure-clusters.
func (h *DashboardHandler) ListFailureClusters(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	result, err := h.dashboardService.ListFailureClusters(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list failure clusters")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// ListClusterSessions handles GET /api/orgs/{orgID}/failure-clusters/{clusterID}/sessions.
func (h *DashboardHandler) ListClusterSessions(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	clusterID, err := uuid.Parse(chi.URLParam(r, "clusterID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_cluster_id", "Invalid cluster ID format")
		return
	}

	result, err := h.dashboardService.ListSessionsByCluster(r.Context(), orgID, clusterID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list cluster sessions")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// GetUsage handles GET /api/orgs/{orgID}/usage.
func (h *DashboardHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	usage, err := h.dashboardService.GetUsage(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get usage")
		return
	}

	WriteJSON(w, http.StatusOK, usage)
}

