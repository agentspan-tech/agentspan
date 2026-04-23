package web

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"strings"
)

// dist is the compiled Vite output, embedded at build time.
// The dist/ directory must exist at processing/web/dist/ when go build runs.
// In Docker: the Dockerfile copies web/dist/ here before building.
// For local dev: run `npm run build` in web/, then `cp -r web/dist processing/web/dist`.
//
//go:embed dist
var embeddedWebFS embed.FS

// NewSPAHandler returns an http.Handler that serves the embedded Vite SPA.
// Unknown paths fall back to index.html for client-side routing.
func NewSPAHandler() (http.Handler, error) {
	sub, err := fs.Sub(embeddedWebFS, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; font-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		if _, err := fs.Stat(sub, path); errors.Is(err, fs.ErrNotExist) {
			// SPA fallback: serve index.html for all unmatched paths
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	}), nil
}
