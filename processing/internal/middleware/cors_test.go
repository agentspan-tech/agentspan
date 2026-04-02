package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/agentspan/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func setupCORSRouter(allowedOrigins string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(mw.CORS(allowedOrigins))
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func TestCORS_AllowedOrigin(t *testing.T) {
	r := setupCORSRouter("http://localhost:3000,http://app.example.com")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("expected ACAO=http://localhost:3000, got %s", rr.Header().Get("Access-Control-Allow-Origin"))
	}
	if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("expected Access-Control-Allow-Credentials=true for specific origin")
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	r := setupCORSRouter("http://localhost:3000")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Response should succeed but without CORS headers
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no ACAO header for disallowed origin, got %s", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORS_PreflightOptions(t *testing.T) {
	r := setupCORSRouter("http://localhost:3000")

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected Access-Control-Allow-Methods header in preflight response")
	}
	if rr.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected Access-Control-Allow-Headers header in preflight response")
	}
}

func TestCORS_WildcardOrigin(t *testing.T) {
	r := setupCORSRouter("*")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://any-origin.example.com")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected ACAO=*, got %s", rr.Header().Get("Access-Control-Allow-Origin"))
	}
	// With wildcard, credentials should NOT be true
	if rr.Header().Get("Access-Control-Allow-Credentials") == "true" {
		t.Error("Access-Control-Allow-Credentials should not be 'true' with wildcard origin")
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	r := setupCORSRouter("http://localhost:3000")

	// Same-origin request (no Origin header)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers when no Origin header")
	}
}

func TestCORS_EmptyConfig(t *testing.T) {
	r := setupCORSRouter("")

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://some-origin.example.com")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers when allowed origins is empty")
	}
}
