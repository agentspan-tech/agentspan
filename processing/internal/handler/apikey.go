package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agentspan/processing/internal/middleware"
	"github.com/agentspan/processing/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// APIKeyHandler handles HTTP requests for API key management.
type APIKeyHandler struct {
	apiKeyService    *service.APIKeyService
	requireActiveOrg func(http.Handler) http.Handler
	requireRole      func(roles ...string) func(http.Handler) http.Handler
}

// NewAPIKeyHandler creates a new APIKeyHandler.
// requireActiveOrg and requireRole are middleware factories injected to avoid an import cycle
// (middleware imports handler, so handler must not import middleware).
func NewAPIKeyHandler(
	apiKeyService *service.APIKeyService,
	requireActiveOrg func(http.Handler) http.Handler,
	requireRole func(roles ...string) func(http.Handler) http.Handler,
) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeyService:    apiKeyService,
		requireActiveOrg: requireActiveOrg,
		requireRole:      requireRole,
	}
}

// Routes returns a chi.Router with all API key management endpoints.
// Must be mounted under /api/orgs/{orgID}/keys with RequireOrg applied upstream.
func (h *APIKeyHandler) Routes() chi.Router {
	r := chi.NewRouter()

	// All routes require an active organization (not pending_deletion).
	r.Use(h.requireActiveOrg)

	// Create — owner, admin, member can create keys; viewer cannot.
	r.With(h.requireRole("owner", "admin", "member")).Post("/", h.Create)

	// List — all roles can view.
	r.Get("/", h.List)

	// Get single — all roles can view.
	r.Get("/{keyID}", h.Get)

	// Deactivate — owner, admin, member only; irreversible.
	r.With(h.requireRole("owner", "admin", "member")).Delete("/{keyID}", h.Deactivate)

	return r
}

// createRequest is the JSON body for POST /api/orgs/{orgID}/keys.
type createRequest struct {
	Name         string  `json:"name"`
	ProviderType string  `json:"provider_type"`
	ProviderKey  string  `json:"provider_key"`
	BaseURL      *string `json:"base_url"`
}

// Create handles POST /api/orgs/{orgID}/keys.
// Returns 201 with APIKeyCreateResult, including raw_key shown this one time only.
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_json", "Request body must be valid JSON")
		return
	}

	result, err := h.apiKeyService.CreateAPIKey(r.Context(), orgID, req.Name, req.ProviderType, req.ProviderKey, req.BaseURL)
	if err != nil {
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			WriteError(w, http.StatusUnprocessableEntity, ve.Code, ve.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create API key")
		return
	}

	WriteJSON(w, http.StatusCreated, result)
}

// List handles GET /api/orgs/{orgID}/keys.
// Returns 200 with array of APIKeyListItem (masked display, no raw key, no encrypted provider key).
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	items, err := h.apiKeyService.ListAPIKeys(r.Context(), orgID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list API keys")
		return
	}

	WriteJSON(w, http.StatusOK, items)
}

// Get handles GET /api/orgs/{orgID}/keys/{keyID}.
// Returns 200 with a single APIKeyListItem.
func (h *APIKeyHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	keyID, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_key_id", "Invalid API key ID")
		return
	}

	item, err := h.apiKeyService.GetAPIKey(r.Context(), orgID, keyID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not_found", "API key not found")
		return
	}

	WriteJSON(w, http.StatusOK, item)
}

// Deactivate handles DELETE /api/orgs/{orgID}/keys/{keyID}.
// Deactivation is irreversible — there is no reactivation endpoint (AKEY-03).
func (h *APIKeyHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	orgID, ok := middleware.GetOrgID(r.Context())
	if !ok {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Organization context missing")
		return
	}

	keyID, err := uuid.Parse(chi.URLParam(r, "keyID"))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_key_id", "Invalid API key ID")
		return
	}

	if err := h.apiKeyService.DeactivateAPIKey(r.Context(), orgID, keyID); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to deactivate API key")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"deactivated": true})
}

