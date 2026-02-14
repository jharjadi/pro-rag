-- 003_documents.sql: Documents table with tenant FK
CREATE TABLE IF NOT EXISTS documents (
    doc_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id    UUID NOT NULL REFERENCES tenants(tenant_id),
    source_type  TEXT NOT NULL,
    source_uri   TEXT NOT NULL,
    title        TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_documents_tenant
    ON documents(tenant_id);

CREATE INDEX IF NOT EXISTS idx_documents_tenant_source
    ON documents(tenant_id, source_uri);
