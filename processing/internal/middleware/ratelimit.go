package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxVisitors = 100000

// RateLimiter is a simple per-IP sliding window rate limiter.
type RateLimiter struct {
	mu             sync.Mutex
	visitors       map[string]*visitor
	limit          int
	window         time.Duration
	trustedProxies map[string]struct{}
}

type visitor struct {
	timestamps []time.Time
}

// NewRateLimiter creates a rate limiter allowing limit requests per window per IP.
// trustedProxies is a list of IP addresses whose X-Forwarded-For header is trusted.
// If empty, X-Forwarded-For is never used and RemoteAddr is always used.
// The cleanup goroutine stops when ctx is cancelled.
func NewRateLimiter(ctx context.Context, limit int, window time.Duration, trustedProxies []string) *RateLimiter {
	tp := make(map[string]struct{}, len(trustedProxies))
	for _, p := range trustedProxies {
		tp[p] = struct{}{}
	}
	rl := &RateLimiter{
		visitors:       make(map[string]*visitor),
		limit:          limit,
		window:         window,
		trustedProxies: tp,
	}
	go rl.cleanup(ctx)
	return rl
}

// Middleware returns a chi-compatible middleware that enforces rate limits.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := rl.clientIP(r)

		if !rl.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate_limit_exceeded","message":"Too many requests. Please try again later."}`)) //nolint:errcheck
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]
	if !exists {
		// At capacity: evict the oldest entry instead of rejecting legitimate users.
		if len(rl.visitors) >= maxVisitors {
			slog.Error("rate limiter at max visitors capacity, evicting oldest entry — possible attack", "max_visitors", maxVisitors)
			var oldestIP string
			var oldestTime time.Time
			first := true
			for k, vis := range rl.visitors {
				latest := time.Time{}
				if len(vis.timestamps) > 0 {
					latest = vis.timestamps[len(vis.timestamps)-1]
				}
				if first || latest.Before(oldestTime) {
					oldestIP = k
					oldestTime = latest
					first = false
				}
			}
			if oldestIP != "" {
				delete(rl.visitors, oldestIP)
			}
		}
		v = &visitor{}
		rl.visitors[ip] = v
	}

	// Remove timestamps outside the window
	cutoff := now.Add(-rl.window)
	valid := v.timestamps[:0]
	for _, t := range v.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	v.timestamps = valid

	if len(v.timestamps) >= rl.limit {
		return false
	}

	v.timestamps = append(v.timestamps, now)
	return true
}

// cleanup periodically removes stale visitor entries.
// Stops when ctx is cancelled.
// Evicts entries where all timestamps are older than 2x window (TTL-based).
func (rl *RateLimiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			ttlCutoff := now.Add(-2 * rl.window)
			for ip, v := range rl.visitors {
				// Remove all expired timestamps
				cutoff := now.Add(-rl.window)
				valid := v.timestamps[:0]
				for _, t := range v.timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				// Evict entry if no valid timestamps or all are older than TTL
				if len(valid) == 0 {
					delete(rl.visitors, ip)
				} else {
					// Check if latest timestamp is beyond TTL cutoff
					latest := valid[len(valid)-1]
					if latest.Before(ttlCutoff) {
						delete(rl.visitors, ip)
					} else {
						v.timestamps = valid
					}
				}
			}
			rl.mu.Unlock()
		}
	}
}

// clientIP extracts the client IP from the request.
// X-Forwarded-For is only trusted when RemoteAddr is a configured trusted proxy.
func (rl *RateLimiter) clientIP(r *http.Request) string {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}

	if _, trusted := rl.trustedProxies[remoteIP]; trusted {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// Use the first (leftmost) IP — the original client
			if idx := strings.IndexByte(fwd, ','); idx != -1 {
				return strings.TrimSpace(fwd[:idx])
			}
			return strings.TrimSpace(fwd)
		}
	}

	return remoteIP
}

// EmailRateLimiter limits requests per email address (e.g., password reset, email verification).
type EmailRateLimiter struct {
	mu       sync.Mutex
	entries  map[string][]time.Time
	limit    int
	window   time.Duration
}

// NewEmailRateLimiter creates an email-based rate limiter.
// limit is the max requests per email per window.
// The cleanup goroutine stops when ctx is cancelled.
func NewEmailRateLimiter(ctx context.Context, limit int, window time.Duration) *EmailRateLimiter {
	erl := &EmailRateLimiter{
		entries: make(map[string][]time.Time),
		limit:   limit,
		window:  window,
	}
	go erl.cleanup(ctx)
	return erl
}

// Allow returns true if the email has not exceeded its rate limit.
func (erl *EmailRateLimiter) Allow(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	erl.mu.Lock()
	defer erl.mu.Unlock()

	now := time.Now()
	timestamps := erl.entries[email]

	// Remove timestamps outside the window
	cutoff := now.Add(-erl.window)
	valid := timestamps[:0]
	for _, t := range timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= erl.limit {
		erl.entries[email] = valid
		return false
	}

	erl.entries[email] = append(valid, now)
	return true
}

func (erl *EmailRateLimiter) cleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			erl.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-erl.window)
			for email, timestamps := range erl.entries {
				valid := timestamps[:0]
				for _, t := range timestamps {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				if len(valid) == 0 {
					delete(erl.entries, email)
				} else {
					erl.entries[email] = valid
				}
			}
			erl.mu.Unlock()
		}
	}
}
