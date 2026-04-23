//go:build pprof

package handler

import (
	"net/http"
	_ "net/http/pprof"

	"github.com/go-chi/chi/v5"
)

// registerPprof mounts pprof handlers on the router when built with -tags pprof.
// net/http/pprof registers on DefaultServeMux in init(). chi's route params
// interfere with pprof.Index path parsing, so delegate to DefaultServeMux directly.
func registerPprof(r chi.Router) {
	pprofHandler := http.StripPrefix("/internal", http.DefaultServeMux)
	r.Get("/debug/pprof/*", pprofHandler.ServeHTTP)
	r.Get("/debug/pprof", pprofHandler.ServeHTTP)
}
