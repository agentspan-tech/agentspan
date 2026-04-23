package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func setupCSRFRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(mw.RequireXHR)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Put("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Delete("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func TestCSRF_GETAllowed(t *testing.T) {
	r := setupCSRFRouter()

	// GET without X-Requested-With should pass (safe method)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET without X-Requested-With, got %d", rr.Code)
	}
}

func TestCSRF_POSTWithHeader(t *testing.T) {
	r := setupCSRFRouter()

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for POST with X-Requested-With, got %d", rr.Code)
	}
}

func TestCSRF_POSTWithoutHeader(t *testing.T) {
	r := setupCSRFRouter()

	req := httptest.NewRequest("POST", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for POST without X-Requested-With, got %d", rr.Code)
	}
}

func TestCSRF_PUTWithoutHeader(t *testing.T) {
	r := setupCSRFRouter()

	req := httptest.NewRequest("PUT", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for PUT without X-Requested-With, got %d", rr.Code)
	}
}

func TestCSRF_DELETEWithoutHeader(t *testing.T) {
	r := setupCSRFRouter()

	req := httptest.NewRequest("DELETE", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for DELETE without X-Requested-With, got %d", rr.Code)
	}
}

func TestCSRF_APIKeyBypass(t *testing.T) {
	// When API key is in context, CSRF check is skipped
	r := chi.NewRouter()
	// Inject API key ID into context before CSRF check
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), mw.APIKeyIDKey, uuid.New())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	r.Use(mw.RequireXHR)
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	// No X-Requested-With header
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for API key bypass, got %d", rr.Code)
	}
}
