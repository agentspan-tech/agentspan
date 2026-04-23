//go:build !pprof

package handler

import "github.com/go-chi/chi/v5"

// registerPprof is a no-op when built without -tags pprof.
func registerPprof(_ chi.Router) {}
