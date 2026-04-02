package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewHealthHandler returns an http.HandlerFunc that checks database connectivity.
// Returns 200 with {"status":"ok"} if healthy, 503 with {"status":"unhealthy"} if DB is unreachable.
func NewHealthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`)) //nolint:errcheck
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`)) //nolint:errcheck
	}
}
