-- 007_chunk_fts.sql: Full-text search with tsvector + GIN index
CREATE TABLE IF NOT EXISTS chunk_fts (
    chunk_id    UUID NOT NULL REFERENCES chunks(chunk_id),
    tenant_id   UUID NOT NULL REFERENCES tenants(tenant_id),
    tsv         tsvector NOT NULL,
    PRIMARY KEY (chunk_id)
);

CREATE INDEX IF NOT EXISTS idx_chunk_fts_tenant
    ON chunk_fts(tenant_id);

-- GIN index for full-text search
CREATE INDEX IF NOT EXISTS idx_chunk_fts_gin
    ON chunk_fts USING gin(tsv);
