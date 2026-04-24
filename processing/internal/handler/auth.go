package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
	"github.com/agentorbit-tech/agentorbit/processing/internal/email"
	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/agentorbit-tech/agentorbit/processing/internal/service"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles HTTP requests for /auth/* routes.
type AuthHandler struct {
	authService      *service.AuthService
	mailer           email.Mailer
	cookieSecure     bool
	jwtTTL           time.Duration
	emailRateLimiter *middleware.EmailRateLimiter
	queries          *db.Queries
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authService *service.AuthService, mailer email.Mailer, appBaseURL string, jwtTTL time.Duration, emailRL *middleware.EmailRateLimiter, queries *db.Queries) *AuthHandler {
	return &AuthHandler{
		authService:      authService,
		mailer:           mailer,
		cookieSecure:     strings.HasPrefix(appBaseURL, "https://"),
		jwtTTL:           jwtTTL,
		emailRateLimiter: emailRL,
		queries:          queries,
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
	r.Post("/resend-verification", h.ResendVerification)
	r.Post("/reset-password", h.ResetPassword)
	r.Post("/logout", h.Logout)
	r.Post("/pro-request", h.ProRequest)
	return r
}

// SetupStatus handles GET /auth/setup-status.
// Returns whether the initial setup (first user creation) is complete.
func (h *AuthHandler) SetupStatus(w http.ResponseWriter, r *http.Request) {
	result, err := h.authService.SetupStatus(r.Context())
	if err != nil {
		slog.Error("setup-status failed", "error", err)
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// Register handles POST /auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email            string `json:"email"`
		Name             string `json:"name"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
		AcceptedTerms    *bool  `json:"accepted_terms"`
		AcceptedPrivacy  *bool  `json:"accepted_privacy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.AcceptedTerms == nil || !*req.AcceptedTerms {
		WriteError(w, http.StatusUnprocessableEntity, "terms_not_accepted", "You must accept the Terms of Service")
		return
	}
	if req.AcceptedPrivacy == nil || !*req.AcceptedPrivacy {
		WriteError(w, http.StatusUnprocessableEntity, "privacy_not_accepted", "You must accept the Privacy Policy")
		return
	}

	result, err := h.authService.Register(r.Context(), req.Email, req.Name, req.Password, LocaleFromRequest(r), req.OrganizationName)
	if err != nil {
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		slog.Error("register failed", "error", err, "email", req.Email)
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	// Self-host first user: auto-verified, frontend can proceed to login
	if result.AutoLogin {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":         result.UserID,
			"email":           result.Email,
			"organization_id": result.OrganizationID,
			"auto_login":      true,
		})
		return
	}

	if h.mailer.IsSMTP() {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":         result.UserID,
			"email":           result.Email,
			"organization_id": result.OrganizationID,
			"email_sent":      true,
		})
	} else {
		WriteJSON(w, http.StatusCreated, map[string]interface{}{
			"user_id":          result.UserID,
			"email":            result.Email,
			"organization_id":  result.OrganizationID,
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
		slog.Error("login failed", "error", err, "email", req.Email)
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "agentorbit_token",
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
		slog.Error("verify-email failed", "error", err)
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

	result, err := h.authService.RequestPasswordReset(r.Context(), req.Email, LocaleFromRequest(r))
	if err != nil {
		slog.Error("request-password-reset failed", "error", err, "email", req.Email)
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

// ResendVerification handles POST /auth/resend-verification.
// Always returns 202 accepted regardless of outcome to avoid email enumeration.
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// Per-email rate limit (shares the RequestPasswordReset limiter: 3 per hour).
	if !h.emailRateLimiter.Allow(req.Email) {
		WriteJSON(w, http.StatusAccepted, map[string]interface{}{"accepted": true})
		return
	}

	if err := h.authService.ResendVerification(r.Context(), req.Email, LocaleFromRequest(r)); err != nil {
		slog.Error("resend-verification failed", "error", err, "email", req.Email)
	}
	WriteJSON(w, http.StatusAccepted, map[string]interface{}{"accepted": true})
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
		slog.Error("reset-password failed", "error", err)
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
		Name:     "agentorbit_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// ProRequest handles POST /auth/pro-request.
// Public endpoint — no authentication required. Collects Pro tier interest.
func (h *AuthHandler) ProRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email   string `json:"email"`
		Company string `json:"company"`
		Message string `json:"message"`
		Source  string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		WriteError(w, http.StatusUnprocessableEntity, "invalid_email", "Email is required")
		return
	}

	if len(req.Message) > 2000 {
		req.Message = req.Message[:2000]
	}
	if len(req.Company) > 200 {
		req.Company = req.Company[:200]
	}

	_, err := h.queries.CreateProRequest(r.Context(), db.CreateProRequestParams{
		Email:   req.Email,
		Company: strings.TrimSpace(req.Company),
		Message: strings.TrimSpace(req.Message),
		Source:  strings.TrimSpace(req.Source),
	})
	if err != nil {
		slog.Error("pro-request failed", "error", err, "email", req.Email)
		WriteError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"submitted": true,
	})
}
