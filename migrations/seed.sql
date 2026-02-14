-- seed.sql: Test tenant for development
INSERT INTO tenants (tenant_id, name, embedding_model, embedding_dim)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Test Tenant',
    'BAAI/bge-base-en-v1.5',
    768
)
ON CONFLICT (tenant_id) DO NOTHING;
