-- 005_chunks.sql: Chunks table with heading_path and metadata JSONB
CREATE TABLE IF NOT EXISTS chunks (
    chunk_id        UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id       UUID NOT NULL REFERENCES tenants(tenant_id),
    doc_version_id  UUID NOT NULL REFERENCES document_versions(doc_version_id),
    ordinal         INTEGER NOT NULL,
    heading_path    JSONB,
    chunk_type      TEXT NOT NULL,
    text            TEXT NOT NULL,
    token_count     INTEGER NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chunks_tenant
    ON chunks(tenant_id);

CREATE INDEX IF NOT EXISTS idx_chunks_doc_version
    ON chunks(doc_version_id);

CREATE INDEX IF NOT EXISTS idx_chunks_tenant_doc_version
    ON chunks(tenant_id, doc_version_id);
