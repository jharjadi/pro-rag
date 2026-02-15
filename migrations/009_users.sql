-- 009_users.sql: Users table for auth + RBAC (spec v2.3 §10)
-- Required before ingestion_runs.initiated_by FK

CREATE TABLE IF NOT EXISTS users (
    user_id     UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID NOT NULL REFERENCES tenants(tenant_id),
    email       TEXT NOT NULL,
    password_hash TEXT NOT NULL DEFAULT '',
    role        TEXT NOT NULL DEFAULT 'user'
                CHECK (role IN ('admin', 'user')),
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_tenant_email
    ON users(tenant_id, email);

CREATE INDEX IF NOT EXISTS idx_users_tenant
    ON users(tenant_id);

-- Note: Default admin user is seeded via seed.sql (requires tenant to exist first)
