//go:build integration

package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentorbit-tech/agentorbit/processing/internal/handler"
)

func TestHealthHandler_Healthy(t *testing.T) {
	h := handler.NewHealthHandler(sharedPool)

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", rr.Body.String())
	}
}
