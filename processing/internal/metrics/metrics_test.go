package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestObserveHTTPRequest(t *testing.T) {
	// Should not panic; metrics are registered via promauto (init-time).
	ObserveHTTPRequest("GET", "/api/test", 200, 50*time.Millisecond)
	ObserveHTTPRequest("POST", "/api/test", 500, 1*time.Second)
}

func TestHandler(t *testing.T) {
	h := Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "agentspan_processing") {
		t.Error("expected agentspan_processing metrics in output")
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("statusCode = %d, want 404", rw.statusCode)
	}

	// Second call should not change the captured code.
	rw.WriteHeader(http.StatusInternalServerError)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("statusCode changed to %d after second WriteHeader", rw.statusCode)
	}
}

func TestResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: 0}

	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("wrote %d bytes, want 5", n)
	}
	// Write without prior WriteHeader should set 200.
	if rw.statusCode != http.StatusOK {
		t.Errorf("statusCode = %d, want 200", rw.statusCode)
	}
	if !rw.written {
		t.Error("expected written=true after Write")
	}
}

func TestResponseWriter_Unwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec}
	if rw.Unwrap() != rec {
		t.Error("Unwrap should return original ResponseWriter")
	}
}

func TestMiddleware(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/test/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test/42", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want %q", rec.Body.String(), "ok")
	}
}

func TestMiddleware_UnknownRoute(t *testing.T) {
	// Request to an unregistered route — routePattern should be "unknown".
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/exists", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/does-not-exist", nil)
	r.ServeHTTP(rec, req)

	// chi returns 404 for unmatched routes — middleware still records metrics.
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}
