package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mw "github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func setupRateLimitRouter(limit int, window time.Duration) (*chi.Mux, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	rl := mw.NewRateLimiter(ctx, limit, window, nil)

	r := chi.NewRouter()
	r.Use(rl.Middleware)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r, cancel
}

func TestRateLimit_UnderLimit(t *testing.T) {
	r, cancel := setupRateLimitRouter(5, time.Minute)
	defer cancel()

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}
}

func TestRateLimit_OverLimit(t *testing.T) {
	r, cancel := setupRateLimitRouter(3, time.Minute)
	defer cancel()

	// Make 3 requests (at limit)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestRateLimit_DifferentIPs(t *testing.T) {
	r, cancel := setupRateLimitRouter(2, time.Minute)
	defer cancel()

	// 2 requests from IP1
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("IP1 request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// IP1 should be blocked
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 3rd request: expected 429, got %d", rr.Code)
	}

	// IP2 should still be allowed
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("IP2 1st request: expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_WindowReset(t *testing.T) {
	// Use a very short window
	r, cancel := setupRateLimitRouter(1, 50*time.Millisecond)
	defer cancel()

	// First request — allowed
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rr.Code)
	}

	// Second request — blocked
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rr.Code)
	}

	// Wait for window to pass
	time.Sleep(100 * time.Millisecond)

	// Third request — should be allowed again
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("after window reset: expected 200, got %d", rr.Code)
	}
}

func TestRateLimit_TrustedProxy_XFF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	rl := mw.NewRateLimiter(ctx, 1, time.Minute, []string{"10.0.0.1"})

	r := chi.NewRouter()
	r.Use(rl.Middleware)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request from trusted proxy with X-Forwarded-For
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rr.Code)
	}

	// Same XFF IP should be rate limited
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("second request from same IP: expected 429, got %d", rr.Code)
	}
}

func TestRateLimit_UntrustedProxy_IgnoresXFF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// No trusted proxies configured
	rl := mw.NewRateLimiter(ctx, 1, time.Minute, nil)

	r := chi.NewRouter()
	r.Use(rl.Middleware)
	r.Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request with spoofed XFF
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Second request — should be limited by RemoteAddr, not XFF
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "5.6.7.8") // different XFF, same RemoteAddr
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 (rate limited by RemoteAddr), got %d", rr.Code)
	}
}

func TestEmailRateLimiter_WindowReset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	erl := mw.NewEmailRateLimiter(ctx, 1, 50*time.Millisecond)

	if !erl.Allow("test@example.com") {
		t.Error("first request should be allowed")
	}
	if erl.Allow("test@example.com") {
		t.Error("second request should be blocked")
	}

	time.Sleep(100 * time.Millisecond)

	if !erl.Allow("test@example.com") {
		t.Error("request after window reset should be allowed")
	}
}

func TestEmailRateLimiter_CaseInsensitive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	erl := mw.NewEmailRateLimiter(ctx, 1, time.Minute)

	if !erl.Allow("Test@Example.Com") {
		t.Error("first request should be allowed")
	}
	if erl.Allow("test@example.com") {
		t.Error("same email different case should be rate limited")
	}
}

func TestEmailRateLimiter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	erl := mw.NewEmailRateLimiter(ctx, 2, time.Minute)

	// First two should be allowed
	if !erl.Allow("test@example.com") {
		t.Error("first request should be allowed")
	}
	if !erl.Allow("test@example.com") {
		t.Error("second request should be allowed")
	}

	// Third should be blocked
	if erl.Allow("test@example.com") {
		t.Error("third request should be blocked")
	}

	// Different email should be allowed
	if !erl.Allow("other@example.com") {
		t.Error("different email should be allowed")
	}
}
