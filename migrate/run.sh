#!/bin/sh
# migrate/run.sh — Idempotent migration runner
# Connects to Postgres, creates schema_migrations tracking table,
# runs each .sql in order, skips already-applied.
set -e

MIGRATIONS_DIR="${MIGRATIONS_DIR:-/migrations}"

echo "==> Waiting for Postgres to be ready..."
until pg_isready -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" >/dev/null 2>&1; do
    echo "    Postgres not ready, retrying in 1s..."
    sleep 1
done
echo "==> Postgres is ready."

# Create tracking table if it doesn't exist
psql -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename    TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL

echo "==> Running migrations from ${MIGRATIONS_DIR}..."

# Process each .sql file in order (excluding seed.sql)
for f in $(ls "${MIGRATIONS_DIR}"/*.sql 2>/dev/null | sort); do
    filename=$(basename "$f")

    # Skip seed file — that's run separately
    if [ "$filename" = "seed.sql" ]; then
        continue
    fi

    # Check if already applied
    already_applied=$(psql -tAc "SELECT 1 FROM schema_migrations WHERE filename = '${filename}'" 2>/dev/null || echo "")

    if [ "$already_applied" = "1" ]; then
        echo "    SKIP: ${filename} (already applied)"
    else
        echo "    APPLY: ${filename}"
        psql -v ON_ERROR_STOP=1 -f "$f"
        psql -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations (filename) VALUES ('${filename}')"
        echo "    DONE: ${filename}"
    fi
done

echo "==> All migrations applied."
