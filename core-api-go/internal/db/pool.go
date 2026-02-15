// Package db provides database connection pooling and startup checks.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxRetries    = 10
	retryBaseWait = 1 * time.Second
	retryMaxWait  = 10 * time.Second
)

// requiredExtensions that must be installed in the database.
var requiredExtensions = []string{"uuid-ossp", "vector"}

// requiredTables that must exist for the query API to function.
var requiredTables = []string{
	"tenants",
	"users",
	"documents",
	"document_versions",
	"chunks",
	"chunk_embeddings",
	"chunk_fts",
	"ingestion_runs",
}

// Connect creates a pgx connection pool with retry logic.
// It retries up to maxRetries times with exponential backoff.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	// Pool settings suitable for a query API
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute

	var pool *pgxpool.Pool
	wait := retryBaseWait

	for attempt := 1; attempt <= maxRetries; attempt++ {
		pool, err = pgxpool.NewWithConfig(ctx, config)
		if err == nil {
			// Test the connection
			if pingErr := pool.Ping(ctx); pingErr == nil {
				slog.Info("database connected", "attempt", attempt)
				return pool, nil
			} else {
				err = pingErr
				pool.Close()
			}
		}

		if attempt == maxRetries {
			return nil, fmt.Errorf("database connection failed after %d attempts: %w", maxRetries, err)
		}

		slog.Warn("database connection failed, retrying",
			"attempt", attempt,
			"max_retries", maxRetries,
			"wait", wait.String(),
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during DB connect: %w", ctx.Err())
		case <-time.After(wait):
		}

		// Exponential backoff with cap
		wait = wait * 2
		if wait > retryMaxWait {
			wait = retryMaxWait
		}
	}

	return nil, fmt.Errorf("database connection failed: %w", err)
}

// CheckExtensions verifies that all required Postgres extensions are installed.
func CheckExtensions(ctx context.Context, pool *pgxpool.Pool) error {
	for _, ext := range requiredExtensions {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = $1)", ext,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check extension %q: %w", ext, err)
		}
		if !exists {
			return fmt.Errorf("required extension %q is not installed", ext)
		}
		slog.Debug("extension check passed", "extension", ext)
	}
	return nil
}

// CheckTables verifies that all required tables exist in the database.
func CheckTables(ctx context.Context, pool *pgxpool.Pool) error {
	for _, table := range requiredTables {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check table %q: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("required table %q does not exist — run migrations first", table)
		}
		slog.Debug("table check passed", "table", table)
	}
	return nil
}

// StartupChecks runs all pre-flight checks (extensions + tables).
func StartupChecks(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("running startup checks...")

	if err := CheckExtensions(ctx, pool); err != nil {
		return fmt.Errorf("extension check failed: %w", err)
	}
	slog.Info("all required extensions present")

	if err := CheckTables(ctx, pool); err != nil {
		return fmt.Errorf("table check failed: %w", err)
	}
	slog.Info("all required tables present")

	return nil
}
