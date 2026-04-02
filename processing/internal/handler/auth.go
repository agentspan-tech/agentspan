package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/agentspan/processing/internal/email"
	"github.com/agentspan/processing/internal/middleware"
	"github.com/agentspan/processing/internal/service"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles HTTP requests for /auth/* routes.
type AuthHandler struct {
	authService      *service.AuthService
	mailer           email.Mailer
	cookieSecure     bool
	jwtTTL           time.Duration
	emailRateLimiter *middleware.EmailRateLimiter
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService, mailer email.Mailer, appBaseURL string, jwtTTL time.Duration, emailRL *middleware.EmailRateLimiter) *AuthHandler {
	return &AuthHandler{
		authService:      authService,
		mailer:           mailer,
		cookieSecure:     strings.HasPrefix(appBaseURL, "https://"),
		jwtTTL:           jwtTTL,
		emailRateLimiter: emailRL,
	}
}

// Routes returns a mountable chi.Router for /auth/ routes.
func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/setup-status", h.SetupStatus)
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/verify-email", h.VerifyEmail)
	r.Post("/request-password-reset", h.RequestPasswordReset)
	r.Post("/reset-password", h.ResetPassword)
	r.Post("/logout", h.Logout)
	return r
}

// SetupStatus handles GET /auth/setup-status.
// Returns whether the initial setup (first user creation) is complete.
func (h *AuthHandler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	result, err := h.authService.SetupStatus(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	result, err := h.authService.Register(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	// Self-host first user: auto-verified, frontend can proceed to login
	if result.AutoLogin {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":    result.UserID,
			"email":      result.Email,
			"auto_login": true,
		})
		return
	}

	if h.mailer.IsSMTP() {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":    result.UserID,
			"email":      result.Email,
			"email_sent": true,
		})
	} else {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":          result.UserID,
			"email":            result.Email,
			"verification_url": result.VerificationURL,
		})
	}
}

// Login handles POST /auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	result, err := h.authService.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "agentspan_token",
		Value:    result.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.jwtTTL.Seconds()),
	})

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"expires_at": result.ExpiresAt,
	})
}

// VerifyEmail handles POST /auth/verify-email.
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.authService.VerifyEmail(r.Context(), req.Token); err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"verified": true,
	})
}

// RequestPasswordReset handles POST /auth/request-password-reset.
func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// Per-email rate limit: 3 requests per hour per email address
	if !h.emailRateLimiter.Allow(req.Email) {
		// Return 202 regardless to avoid email enumeration
		WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"accepted": true,
		})
		return
	}

	result, err := h.authService.RequestPasswordReset(r.Context(), req.Email)
	if err != nil {
		// Internal errors still return 202 to avoid email enumeration
		WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"accepted": true,
		})
		return
	}

	// Always 202 — never reveal whether email exists (AUTH-04)
	if result == nil || h.mailer.IsSMTP() {
		WriteJSON(w, http.StatusAccepted, map[string]interface{}{
			"accepted": true,
		})
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"accepted":  true,
		"reset_url": result.ResetURL,
	})
}

// ResetPassword handles POST /auth/reset-password.
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if err := h.authService.ResetPassword(r.Context(), req.Token, req.Password); err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"reset": true,
	})
}

// Logout handles POST /auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "agentspan_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}
