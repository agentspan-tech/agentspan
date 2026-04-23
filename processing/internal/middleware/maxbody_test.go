package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mw "github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func TestMaxBodySize_UnderLimit(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.MaxBodySize(1024))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader("small body"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "small body" {
		t.Errorf("body = %q, want 'small body'", rr.Body.String())
	}
}

func TestMaxBodySize_OverLimit(t *testing.T) {
	r := chi.NewRouter()
	r.Use(mw.MaxBodySize(10))
	r.Post("/test", func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	body := strings.Repeat("x", 100)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rr.Code)
	}
}
