package handler

import (
	"encoding/json"
	"net/http"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// InviteHandler handles HTTP requests for invite management.
type InviteHandler struct {
	inviteService *service.InviteService
	queries       *db.Queries
	mailer        email.Mailer
}

// NewInviteHandler creates a new InviteHandler.
func NewInviteHandler(inviteService *service.InviteService, queries *db.Queries, mailer email.Mailer) *InviteHandler {
	return &InviteHandler{
		inviteService: inviteService,
		queries:       queries,
		mailer:        mailer,
	}
}

// Routes returns org-scoped invite routes (mounted under /api/orgs/{orgID}/invites).
// All routes require owner or admin role.
func (h *InviteHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).Post("/", h.Create)
	r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).Get("/", h.List)
	r.With(middleware.RequireActiveOrg(), middleware.RequireRole("owner", "admin")).Delete("/{inviteID}", h.Revoke)
	return r
}

// POST /api/orgs/{orgID}/invites
func (h *InviteHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	orgPlan, _ := middleware.GetOrgPlan(r.Context())

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Email == "" {
		WriteError(w, http.StatusBadRequest, "missing_email", "email is required")
		return
	}

	// Fetch inviter's name and the org name for the email.
	inviter, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch inviter info")
		return
	}

	org, err := h.queries.GetOrganizationByID(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch organization info")
		return
	}

	result, err := h.inviteService.CreateInvite(r.Context(), orgID, userID, body.Email, body.Role, org.Name, inviter.Name, orgPlan, org.Locale)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	resp := map[string]interface{}{
		"invite_id": result.InviteID,
	}
	if h.mailer.IsSMTP() {
		resp["email_sent"] = true
	} else {
		resp["invite_url"] = result.InviteURL
	}

	WriteJSON(w, http.StatusCreated, resp)
}

// GET /api/orgs/{orgID}/invites
func (h *InviteHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	invites, err := h.inviteService.ListPendingInvites(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list invites")
		return
	}

	WriteJSON(w, http.StatusOK, invites)
}

// DELETE /api/orgs/{orgID}/invites/{inviteID}
func (h *InviteHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusBadRequest, "invalid_org_id", "Organization ID not in context")
		return
	}

	inviteIDStr := chi.URLParam(r, "inviteID")
	inviteID, err := uuid.Parse(inviteIDStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_invite_id", "Invalid invite ID")
		return
	}

	if err := h.inviteService.RevokeInvite(r.Context(), orgID, inviteID); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// AcceptInvite handles POST /auth/accept-invite (mounted by Plan 05 main.go wiring).
// The user must be authenticated. This method is NOT part of Routes().
func (h *InviteHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if body.Token == "" {
		WriteError(w, http.StatusBadRequest, "missing_token", "token is required")
		return
	}

	// Fetch the user's email to verify it matches the invite target.
	user, err := h.queries.GetUserByID(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to fetch user info")
		return
	}

	result, err := h.inviteService.AcceptInvite(r.Context(), body.Token, userID, user.Email)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"accepted":        true,
		"organization_id": result.OrganizationID,
	})
}
