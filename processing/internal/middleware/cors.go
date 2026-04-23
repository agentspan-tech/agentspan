package middleware

import (
	"log/slog"
	"net/http"
	"strings"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing.
// allowedOrigins is a comma-separated list of allowed origins ("*" to allow all).
// If empty, CORS headers are not set and cross-origin requests are rejected by browsers.
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	origins := parseOrigins(allowedOrigins)

	for _, o := range origins {
		if o == "*" {
			slog.Warn("ALLOWED_ORIGINS=* — cookie-based auth will not work cross-origin (Access-Control-Allow-Credentials cannot be true with wildcard). Use explicit origins for cross-origin deployments.")
			break
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			isWildcard, allowed := matchOrigin(origin, origins)
			if !allowed {
				// No CORS headers — browser will block the response.
				next.ServeHTTP(w, r)
				return
			}

			if isWildcard {
				// Wildcard origin: per spec, Access-Control-Allow-Credentials must not
				// be true with "*". Send wildcard without credentials.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Vary", "Origin")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
				w.Header().Set("Access-Control-Max-Age", "86400")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func parseOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

func matchOrigin(origin string, allowed []string) (isWildcard bool, matched bool) {
	for _, a := range allowed {
		if a == "*" {
			return true, true
		}
		if strings.EqualFold(a, origin) {
			return false, true
		}
	}
	return false, false
}
