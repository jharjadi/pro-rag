-- 008_ingestion_runs.sql: Pipeline run tracking
CREATE TABLE IF NOT EXISTS ingestion_runs (
    run_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID NOT NULL REFERENCES tenants(tenant_id),
    status      TEXT NOT NULL DEFAULT 'running',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    config      JSONB NOT NULL DEFAULT '{}',
    stats       JSONB NOT NULL DEFAULT '{}',
    error       TEXT,
    CONSTRAINT chk_ingestion_runs_status CHECK (status IN ('running', 'succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_tenant
    ON ingestion_runs(tenant_id);

CREATE INDEX IF NOT EXISTS idx_ingestion_runs_status
    ON ingestion_runs(tenant_id, status);
