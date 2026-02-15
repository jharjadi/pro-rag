-- 011_ingestion_runs_orchestration.sql: Add orchestration columns to ingestion_runs.
-- Spec v2.3 §9.3, §9.4

-- Step 1: Add 'queued' to status constraint
ALTER TABLE ingestion_runs DROP CONSTRAINT IF EXISTS chk_ingestion_runs_status;
ALTER TABLE ingestion_runs ADD CONSTRAINT chk_ingestion_runs_status
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed'));

-- Step 2: Add doc_id column (links run to document)
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS doc_id UUID REFERENCES documents(doc_id);

-- Step 3: Add updated_at for heartbeat tracking (crash guard uses this)
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT now();

-- Step 4: Add created_at column (when Go created the run, distinct from started_at)
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- Step 5: Make started_at nullable (set by worker, not Go)
-- Current schema has started_at NOT NULL DEFAULT now(). We need it nullable for queued runs.
ALTER TABLE ingestion_runs ALTER COLUMN started_at DROP NOT NULL;
ALTER TABLE ingestion_runs ALTER COLUMN started_at DROP DEFAULT;

-- Step 5: Add orchestration columns
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS run_type TEXT NOT NULL DEFAULT 'manual_upload'
    CHECK (run_type IN ('manual_upload', 's3_sync'));

-- source_id FK is nullable — only populated for s3_sync runs.
-- ingestion_sources table doesn't exist yet (V2), so no FK for now.
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS source_id UUID;

-- initiated_by references users table (created in 009)
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS initiated_by UUID REFERENCES users(user_id);

-- Step 6: Update existing rows — set updated_at, started_at for any existing running rows
UPDATE ingestion_runs SET updated_at = COALESCE(started_at, now()) WHERE updated_at IS NULL;

-- Step 7: Indexes for crash guard and listing
CREATE INDEX IF NOT EXISTS idx_ingestion_runs_tenant_status
    ON ingestion_runs(tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_stale
    ON ingestion_runs(status, updated_at)
    WHERE status IN ('queued', 'running');

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_tenant_created
    ON ingestion_runs(tenant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_doc
    ON ingestion_runs(doc_id);
