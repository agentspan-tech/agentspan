package handler

import (
	"encoding/json"
	"net/http"

	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
)

// UserHandler handles HTTP requests for user profile management.
type UserHandler struct {
	authService *service.AuthService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(authService *service.AuthService) *UserHandler {
	return &UserHandler{authService: authService}
}

// GetMe handles GET /api/user/me.
// Returns the authenticated user's profile.
func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}

	user, err := h.authService.GetMe(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"id":                user.ID,
		"email":             user.Email,
		"name":              user.Name,
		"email_verified_at": user.EmailVerifiedAt,
		"created_at":        user.CreatedAt,
	})
}

// UpdateProfile handles PUT /api/user/profile.
// Updates the authenticated user's display name.
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.authService.UpdateProfile(r.Context(), userID, req.Name); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ChangePassword handles PUT /api/user/password.
// Changes the authenticated user's password (requires current password).
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "Not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.authService.ChangePassword(r.Context(), userID, req.CurrentPassword, req.NewPassword); err != nil {
		writeServiceError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
