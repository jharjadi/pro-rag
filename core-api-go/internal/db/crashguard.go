package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunCrashGuard marks stale ingestion runs as failed on startup (spec v2.3 §11.1).
//
// Two separate queries:
// 1. Stale queued runs (created_at > queuedTTLHours ago) — job was never picked up
// 2. Stale running runs (updated_at > runningStaleMin ago) — worker probably crashed
func RunCrashGuard(ctx context.Context, pool *pgxpool.Pool, queuedTTLHours, runningStaleMin int) error {
	// Stale queued runs
	tag, err := pool.Exec(ctx,
		`UPDATE ingestion_runs
		 SET status = 'failed',
		     error = 'interrupted — job was never picked up (service restarted)',
		     finished_at = now(),
		     updated_at = now()
		 WHERE status = 'queued'
		   AND created_at < now() - make_interval(hours => $1)`,
		queuedTTLHours,
	)
	if err != nil {
		return fmt.Errorf("crash guard (queued): %w", err)
	}
	if tag.RowsAffected() > 0 {
		slog.Warn("crash guard: marked stale queued runs as failed",
			"count", tag.RowsAffected(),
			"ttl_hours", queuedTTLHours,
		)
	}

	// Stale running runs (no heartbeat)
	tag, err = pool.Exec(ctx,
		`UPDATE ingestion_runs
		 SET status = 'failed',
		     error = 'interrupted — worker stopped responding (no heartbeat)',
		     finished_at = now(),
		     updated_at = now()
		 WHERE status = 'running'
		   AND updated_at < now() - make_interval(mins => $1)`,
		runningStaleMin,
	)
	if err != nil {
		return fmt.Errorf("crash guard (running): %w", err)
	}
	if tag.RowsAffected() > 0 {
		slog.Warn("crash guard: marked stale running runs as failed",
			"count", tag.RowsAffected(),
			"stale_minutes", runningStaleMin,
		)
	}

	slog.Info("crash guard complete")
	return nil
}
