package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// OrgHandler handles HTTP requests for organization management.
type OrgHandler struct {
	orgService *service.OrgService
}

// NewOrgHandler creates a new OrgHandler.
func NewOrgHandler(orgService *service.OrgService) *OrgHandler {
	return &OrgHandler{orgService: orgService}
}

// POST /api/orgs/
func (h *OrgHandler) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	org, err := h.orgService.CreateOrganization(r.Context(), userID, body.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusCreated, org)
}

// GET /api/orgs/
func (h *OrgHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	orgs, err := h.orgService.ListUserOrganizations(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list organizations")
		return
	}

	WriteJSON(w, http.StatusOK, orgs)
}

// GET /api/orgs/{orgID}/
func (h *OrgHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	org, err := h.orgService.GetOrganization(r.Context(), orgID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, org)
}

// PUT /api/orgs/{orgID}/settings
func (h *OrgHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	var body struct {
		Locale                string `json:"locale"`
		SessionTimeoutSeconds int32  `json:"session_timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.orgService.UpdateSettings(r.Context(), orgID, body.Locale, body.SessionTimeoutSeconds); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// DELETE /api/orgs/{orgID}/
func (h *OrgHandler) InitiateDeletion(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	// Guard: check org is not already pending deletion.
	status, _ := middleware.GetOrgStatus(r.Context())
	if status == "pending_deletion" {
		WriteError(w, http.StatusConflict, "already_pending_deletion", "Organization is already scheduled for deletion")
		return
	}

	scheduledAt, err := h.orgService.InitiateDeletion(r.Context(), orgID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	if actorID, ok := middleware.GetUserID(r.Context()); ok {
		slog.Info("audit", "action", "org.initiate_deletion", "actor", actorID, "org", orgID)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":               "pending_deletion",
		"deletion_scheduled_at": scheduledAt,
	})
}

// POST /api/orgs/{orgID}/restore
func (h *OrgHandler) CancelDeletion(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	// Guard: check org IS pending deletion.
	status, _ := middleware.GetOrgStatus(r.Context())
	if status != "pending_deletion" {
		WriteError(w, http.StatusConflict, "not_pending_deletion", "Organization is not scheduled for deletion")
		return
	}

	if err := h.orgService.CancelDeletion(r.Context(), orgID); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

// POST /api/orgs/{orgID}/transfer
func (h *OrgHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	newOwnerID, err := uuid.Parse(body.UserID)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_user_id", "Invalid user_id format")
		return
	}

	if err := h.orgService.TransferOwnership(r.Context(), orgID, newOwnerID); err != nil {
		writeServiceError(w, err)
		return
	}

	if actorID, ok := middleware.GetUserID(r.Context()); ok {
		slog.Info("audit", "action", "org.transfer_ownership", "actor", actorID, "org", orgID, "target", newOwnerID)
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"transferred": true})
}

// POST /api/orgs/{orgID}/leave
func (h *OrgHandler) Leave(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	if err := h.orgService.LeaveOrganization(r.Context(), orgID, userID); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"left": true})
}

// GET /api/orgs/{orgID}/members
func (h *OrgHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	members, err := h.orgService.ListMembers(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list members")
		return
	}

	WriteJSON(w, http.StatusOK, members)
}

// PUT /api/orgs/{orgID}/members/{memberID}/role
func (h *OrgHandler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	memberIDStr := chi.URLParam(r, "memberID")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_member_id", "Invalid member ID")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.orgService.UpdateMemberRole(r.Context(), orgID, memberID, body.Role); err != nil {
		writeServiceError(w, err)
		return
	}

	if actorID, ok := middleware.GetUserID(r.Context()); ok {
		slog.Info("audit", "action", "member.role_change", "actor", actorID, "org", orgID, "target", memberID, "role", body.Role)
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"updated": true})
}

// DELETE /api/orgs/{orgID}/members/{memberID}
func (h *OrgHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	memberIDStr := chi.URLParam(r, "memberID")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_member_id", "Invalid member ID")
		return
	}

	if err := h.orgService.RemoveMember(r.Context(), orgID, memberID); err != nil {
		writeServiceError(w, err)
		return
	}

	if actorID, ok := middleware.GetUserID(r.Context()); ok {
		slog.Info("audit", "action", "member.remove", "actor", actorID, "org", orgID, "target", memberID)
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"removed": true})
}

// GET /api/orgs/{orgID}/privacy-settings
func (h *OrgHandler) GetPrivacySettings(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	settings, err := h.orgService.GetPrivacySettings(r.Context(), orgID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, settings)
}

// PUT /api/orgs/{orgID}/privacy-settings
func (h *OrgHandler) UpdatePrivacySettings(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	var body struct {
		StoreSpanContent *bool           `json:"store_span_content"`
		MaskingConfig    json.RawMessage `json:"masking_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.StoreSpanContent == nil {
		WriteError(w, http.StatusBadRequest, "missing_field", "store_span_content is required")
		return
	}

	if err := h.orgService.UpdatePrivacySettings(r.Context(), orgID, *body.StoreSpanContent, body.MaskingConfig); err != nil {
		writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/orgs/{orgID}/spans/{spanID}/masking-maps
func (h *OrgHandler) GetSpanMaskingMaps(w http.ResponseWriter, r *http.Request) {
	spanIDStr := chi.URLParam(r, "spanID")
	spanID, err := uuid.Parse(spanIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_span_id", "Invalid span ID")
		return
	}

	maps, err := h.orgService.GetSpanMaskingMaps(r.Context(), spanID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch masking maps")
		return
	}

	WriteJSON(w, http.StatusOK, maps)
}

// writeServiceError maps a ServiceError to an HTTP response.
// Other errors become 500 internal errors.
func writeServiceError(w http.ResponseWriter, err error) {
	var svcErr *service.ServiceError
	if errors.As(err, &svcErr) {
		WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
		return
	}
	WriteError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
}
