-- 004_document_versions.sql: Document versions with partial unique index
CREATE TABLE IF NOT EXISTS document_versions (
    doc_version_id       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id            UUID NOT NULL REFERENCES tenants(tenant_id),
    doc_id               UUID NOT NULL REFERENCES documents(doc_id),
    version_label        TEXT NOT NULL,
    effective_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    is_active            BOOLEAN NOT NULL DEFAULT true,
    extracted_artifact_uri TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Partial unique index: at most one active version per (tenant_id, doc_id)
CREATE UNIQUE INDEX IF NOT EXISTS idx_document_versions_one_active
    ON document_versions(tenant_id, doc_id)
    WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_document_versions_tenant_active
    ON document_versions(tenant_id, is_active);
