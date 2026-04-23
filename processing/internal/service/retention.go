package service

import (
	"context"
	"log/slog"

	"github.com/agentorbit-tech/agentorbit/processing/internal/db"
)

const retentionBatchSize = 1000

// RunRetentionPurge deletes spans, sessions, and alert events older than retentionDays.
// Processes in batches to avoid long-running transactions and excessive lock contention.
// Returns total rows deleted across all tables.
func RunRetentionPurge(ctx context.Context, queries *db.Queries, retentionDays int) (int64, error) {
	var total int64

	// Purge spans first (child rows), then sessions (parent rows).
	for {
		n, err := queries.PurgeOldSpans(ctx, db.PurgeOldSpansParams{
			RetentionDays: int32(retentionDays),
			BatchSize:     retentionBatchSize,
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < retentionBatchSize {
			break
		}
	}

	for {
		n, err := queries.PurgeOldSessions(ctx, db.PurgeOldSessionsParams{
			RetentionDays: int32(retentionDays),
			BatchSize:     retentionBatchSize,
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < retentionBatchSize {
			break
		}
	}

	for {
		n, err := queries.PurgeOldAlertEvents(ctx, db.PurgeOldAlertEventsParams{
			RetentionDays: int32(retentionDays),
			BatchSize:     retentionBatchSize,
		})
		if err != nil {
			return total, err
		}
		total += n
		if n < retentionBatchSize {
			break
		}
	}

	if total > 0 {
		slog.Info("retention purge completed", "deleted_rows", total, "retention_days", retentionDays)
	}
	return total, nil
}
