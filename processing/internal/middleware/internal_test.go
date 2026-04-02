package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/agentspan/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func setupInternalTokenRouter(token string) *chi.Mux {
	r := chi.NewRouter()
	r.Use(mw.RequireInternalToken(token))
	r.Post("/internal/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	return r
}

func TestRequireInternalToken_ValidToken(t *testing.T) {
	r := setupInternalTokenRouter("my-secret-internal-token")

	req := httptest.NewRequest("POST", "/internal/test", nil)
	req.Header.Set("X-Internal-Token", "my-secret-internal-token")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestRequireInternalToken_MissingToken(t *testing.T) {
	r := setupInternalTokenRouter("my-secret-internal-token")

	req := httptest.NewRequest("POST", "/internal/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireInternalToken_WrongToken(t *testing.T) {
	r := setupInternalTokenRouter("my-secret-internal-token")

	req := httptest.NewRequest("POST", "/internal/test", nil)
	req.Header.Set("X-Internal-Token", "wrong-token")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireInternalIP_AllowedIP(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.RequireInternalIP([]string{"10.0.0.1"}))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequireInternalIP_DisallowedIP(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.RequireInternalIP([]string{"10.0.0.1"}))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequireInternalIP_EmptyAllowlist(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.RequireInternalIP(nil))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (no-op middleware), got %d", rr.Code)
	}
}

func TestRequireInternalIP_IPWithoutPort(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.RequireInternalIP([]string{"10.0.0.1"}))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.RemoteAddr = "10.0.0.1"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for IP without port, got %d", rr.Code)
	}
}
