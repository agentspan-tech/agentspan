package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/agentspan/processing/internal/service"
	"github.com/go-chi/chi/v5"
)

// InternalHandler handles requests from the Proxy on the Internal API.
type InternalHandler struct {
	internalService *service.InternalService
}

// NewInternalHandler creates a new InternalHandler.
func NewInternalHandler(internalService *service.InternalService) *InternalHandler {
	return &InternalHandler{internalService: internalService}
}

// Routes returns a chi.Router with the internal API endpoints.
// X-Internal-Token auth is applied at the mount point in main.go.
func (h *InternalHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/auth/verify", h.Verify)
	r.Post("/spans/ingest", h.Ingest)

	// Debug endpoints — only available when built with -tags pprof (D-09).
	registerPprof(r)

	return r
}

// verifyRequest is the body for POST /internal/auth/verify.
type verifyRequest struct {
	KeyDigest string `json:"key_digest"`
}

// Verify handles POST /internal/auth/verify.
// Returns 200 with AuthVerifyResult regardless of key validity — the Proxy
// interprets the valid field. Errors in DB/decryption still return 5xx.
func (h *InternalHandler) Verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if req.KeyDigest == "" {
		WriteJSON(w, http.StatusOK, &service.AuthVerifyResult{Valid: false, Reason: "invalid_key"})
		return
	}

	result, err := h.internalService.VerifyAPIKey(r.Context(), req.KeyDigest)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify API key")
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// Ingest handles POST /internal/spans/ingest.
// Returns 202 Accepted when the span is accepted for storage (stub in Phase 2).
// Returns 429 when the free-plan 3000 spans/month limit is exceeded (ORG-12).
func (h *InternalHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	var req service.SpanIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	if err := h.internalService.IngestSpan(r.Context(), &req); err != nil {
		var quotaErr *service.SpanQuotaExceededError
		if errors.As(err, &quotaErr) {
			WriteError(w, http.StatusTooManyRequests, "span_quota_exceeded", "Free plan limit of 3000 spans/month reached")
			return
		}
		var svcErr *service.ServiceError
		if errors.As(err, &svcErr) {
			WriteError(w, svcErr.Status, svcErr.Code, svcErr.Message)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to ingest span")
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}
