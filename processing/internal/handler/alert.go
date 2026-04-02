package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/agentspan/processing/internal/middleware"
	"github.com/agentspan/processing/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// AlertHandler handles HTTP requests for alert rule management.
// All endpoints are org-scoped via RequireOrg middleware applied upstream.
type AlertHandler struct {
	alertService *service.AlertService
}

// NewAlertHandler creates a new AlertHandler.
func NewAlertHandler(alertService *service.AlertService) *AlertHandler {
	return &AlertHandler{alertService: alertService}
}

// Routes returns a chi.Router with all alert endpoints.
// Mounted at /api/orgs/{orgID}/alerts with RequireOrg applied upstream.
func (h *AlertHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/events", h.ListEvents)
	r.Get("/{alertID}", h.Get)
	r.Put("/{alertID}", h.Update)
	r.Delete("/{alertID}", h.Delete)
	return r
}

// Create handles POST /api/orgs/{orgID}/alerts.
func (h *AlertHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	var req service.CreateAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	rule, err := h.alertService.Create(r.Context(), orgID, plan, req)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create alert rule")
		return
	}

	WriteJSON(w, http.StatusCreated, rule)
}

// List handles GET /api/orgs/{orgID}/alerts.
func (h *AlertHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	rules, err := h.alertService.List(r.Context(), orgID, plan)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list alert rules")
		return
	}

	WriteJSON(w, http.StatusOK, rules)
}

// Get handles GET /api/orgs/{orgID}/alerts/{alertID}.
func (h *AlertHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	alertID, err := uuid.Parse(chi.URLParam(r, "alertID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_alert_id", "Invalid alert ID format")
		return
	}

	rule, err := h.alertService.Get(r.Context(), orgID, plan, alertID)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get alert rule")
		return
	}

	WriteJSON(w, http.StatusOK, rule)
}

// Update handles PUT /api/orgs/{orgID}/alerts/{alertID}.
func (h *AlertHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	alertID, err := uuid.Parse(chi.URLParam(r, "alertID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_alert_id", "Invalid alert ID format")
		return
	}

	var req service.UpdateAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	rule, err := h.alertService.Update(r.Context(), orgID, plan, alertID, req)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update alert rule")
		return
	}

	WriteJSON(w, http.StatusOK, rule)
}

// Delete handles DELETE /api/orgs/{orgID}/alerts/{alertID}.
func (h *AlertHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	alertID, err := uuid.Parse(chi.URLParam(r, "alertID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_alert_id", "Invalid alert ID format")
		return
	}

	err = h.alertService.Delete(r.Context(), orgID, plan, alertID)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete alert rule")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListEvents handles GET /api/orgs/{orgID}/alerts/events.
func (h *AlertHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}
	plan, _ := middleware.GetOrgPlan(r.Context())

	// Parse optional limit (default 50, max 200).
	limit := int32(50)
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 {
			WriteError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		if l > 200 {
			l = 200
		}
		limit = int32(l) //nolint:gosec // l is bounded to [1, 200]
	}

	events, err := h.alertService.ListEvents(r.Context(), orgID, plan, limit)
	if err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			WriteError(w, se.Status, se.Code, se.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list alert events")
		return
	}

	WriteJSON(w, http.StatusOK, events)
}
