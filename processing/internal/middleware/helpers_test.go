package middleware_test

import "github.com/agentspan/processing/internal/db"

// newMockQueries creates a *db.Queries backed by a mock DBTX.
func newMockQueries(mock *mockDBTX) *db.Queries {
	return db.New(mock)
}
