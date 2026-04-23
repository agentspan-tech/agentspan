package middleware

import "net/http"

// RequireXHR rejects mutating requests (POST, PUT, DELETE, PATCH) that lack the
// X-Requested-With header. Browsers never send custom headers on cross-origin
// requests without a CORS preflight, so this prevents CSRF attacks when auth
// is stored in cookies.
//
// API key authentication (Authorization: Bearer ao-...) is exempt because API keys
// are never stored in cookies and thus are not vulnerable to CSRF.
func RequireXHR(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			// Skip CSRF check for API key auth — not cookie-based, not vulnerable.
			if _, ok := GetAPIKeyID(r.Context()); ok {
				break
			}
			if r.Header.Get("X-Requested-With") == "" {
				writeError(w, http.StatusForbidden, "csrf_check_failed", "X-Requested-With header is required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
