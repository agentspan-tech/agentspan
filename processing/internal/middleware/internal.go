package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"
)

// RequireInternalToken returns an HTTP middleware that validates the X-Internal-Token header.
// It uses constant-time comparison to prevent timing attacks (SEC-02).
func RequireInternalToken(token string) func(next http.Handler) http.Handler {
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("X-Internal-Token")
			if provided == "" || subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid internal token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireInternalIP returns middleware that restricts access to a list of allowed IPs.
// If allowedIPs is empty, the middleware is a no-op (all IPs allowed).
func RequireInternalIP(allowedIPs []string) func(next http.Handler) http.Handler {
	if len(allowedIPs) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	allowed := make(map[string]bool, len(allowedIPs))
	for _, ip := range allowedIPs {
		allowed[ip] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r.RemoteAddr)
			if !allowed[ip] {
				writeError(w, http.StatusForbidden, "forbidden", "IP not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractIP extracts the IP from a host:port string.
func extractIP(addr string) string {
	if strings.ContainsRune(addr, ':') {
		host, _, err := net.SplitHostPort(addr)
		if err == nil {
			return host
		}
	}
	return addr
}
