package web

import (
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/agentorbit-tech/agentorbit/processing/internal/middleware"
)

// dist is the compiled Vite output, embedded at build time.
// The dist/ directory must exist at processing/web/dist/ when go build runs.
// In Docker: the Dockerfile copies web/dist/ here before building.
// For local dev: run `npm run build` in web/, then `cp -r web/dist processing/web/dist`.
//
// A `.gitkeep` placeholder is committed under dist/ so `//go:embed dist`
// always succeeds even on a clean checkout where the SPA has not been built.
// At runtime, NewSPAHandler detects the placeholder-only state and returns a
// 503 stub explaining the build step instead of serving an empty directory.
//
//go:embed dist
var embeddedWebFS embed.FS

const fallbackBody = `Frontend assets not built.

Run 'make web' from the repo root (or 'cd web && npm install && npm run build')
before 'go build ./processing/cmd/processing'.
`

// NewSPAHandler returns an http.Handler that serves the embedded Vite SPA.
// Unknown paths fall back to index.html for client-side routing.
// billingURL is embedded into the CSP connect-src so the SPA can call a
// separately-hosted billing service in cloud deployments.
//
// If the embedded dist/ directory contains only the .gitkeep placeholder
// (i.e. the SPA was never built), NewSPAHandler returns a 503 stub handler
// explaining the build step instead. The error return is reserved for
// genuine fs.Sub failures.
func NewSPAHandler(billingURL string) (http.Handler, error) {
	sub, err := fs.Sub(embeddedWebFS, "dist")
	if err != nil {
		return nil, err
	}
	return newSPAHandlerFromFS(sub, billingURL), nil
}

// newSPAHandlerFromFS contains the post-fs.Sub logic of NewSPAHandler so tests
// can inject a synthetic FS and exercise the fallback + happy paths through
// the same code path that production uses.
func newSPAHandlerFromFS(sub fs.FS, billingURL string) http.Handler {
	if onlyGitkeep(sub) {
		return fallbackHandler()
	}
	csp := middleware.BuildCSP(billingURL)
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
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
	})
}

// onlyGitkeep reports whether the given filesystem contains nothing but the
// .gitkeep placeholder (or is unreadable). When true, no real build artifacts
// are present and NewSPAHandler should fall back to the 503 stub.
func onlyGitkeep(sub fs.FS) bool {
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		slog.Warn("dist readdir failed, falling back to 503", "err", err)
		return true
	}
	if len(entries) == 0 {
		return true
	}
	for _, e := range entries {
		if e.Name() != ".gitkeep" {
			return false
		}
	}
	return true
}

// fallbackHandler returns a handler that responds with HTTP 503 and a plaintext
// message instructing the operator to build the SPA. Used when the embedded
// dist/ contains only the .gitkeep placeholder.
func fallbackHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(fallbackBody))
	})
}
