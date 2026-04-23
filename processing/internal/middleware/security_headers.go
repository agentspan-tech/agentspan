package middleware

import "net/http"

// SecurityHeaders sets standard security headers on all responses.
//
// KNOWN LIMITATION: style-src uses 'unsafe-inline' because React (and libraries like
// Radix UI / shadcn) inject inline styles at runtime. All inline styles are hardcoded or
// computed from layout — none use user-controlled data. This is safe because React's JSX
// auto-escaping prevents HTML injection. A nonce-based approach would require build pipeline
// changes (Vite plugin to inject nonces) and server-side nonce generation per request.
// Acceptable for v1 and documented in docs/SECURITY.md.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "0") // disabled in favor of CSP
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' wss: ws:; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

