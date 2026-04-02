package metrics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StartDBPoolCollector launches a goroutine that periodically collects DB pool stats.
// Stops when ctx is cancelled.
func StartDBPoolCollector(ctx context.Context, pool *pgxpool.Pool, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	DBPoolMaxConns.Set(float64(pool.Config().MaxConns))

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stat := pool.Stat()
				DBPoolActiveConns.Set(float64(stat.AcquiredConns()))
				DBPoolIdleConns.Set(float64(stat.IdleConns()))
				DBPoolTotalConns.Set(float64(stat.TotalConns()))
			}
		}
	}()
}
