-- 002_tenants.sql: Tenants table
CREATE TABLE IF NOT EXISTS tenants (
    tenant_id   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        TEXT NOT NULL,
    embedding_model TEXT NOT NULL,
    embedding_dim   INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
