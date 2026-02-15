-- seed.sql: Test tenant + admin user for development
INSERT INTO tenants (tenant_id, name, embedding_model, embedding_dim)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Test Tenant',
    'BAAI/bge-base-en-v1.5',
    768
)
ON CONFLICT (tenant_id) DO NOTHING;

-- Default admin user for dev (password: admin123)
-- bcrypt hash generated with cost=10
INSERT INTO users (user_id, tenant_id, email, password_hash, role)
VALUES (
    '00000000-0000-0000-0000-000000000099',
    '00000000-0000-0000-0000-000000000001',
    'admin@test.local',
    '$2a$10$iHkfn8tShQJ3dNdUPjDPI.neCcnldmKPJI.p2DpK6pYUqFMC1kYEu',
    'admin'
)
ON CONFLICT DO NOTHING;
