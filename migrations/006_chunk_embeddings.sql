-- 006_chunk_embeddings.sql: Separate embeddings table (ADR-002) + HNSW index (ADR-003)
CREATE TABLE IF NOT EXISTS chunk_embeddings (
    chunk_id        UUID NOT NULL REFERENCES chunks(chunk_id),
    tenant_id       UUID NOT NULL REFERENCES tenants(tenant_id),
    embedding_model TEXT NOT NULL,
    embedding       vector(768),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chunk_id)
);

CREATE INDEX IF NOT EXISTS idx_chunk_embeddings_tenant
    ON chunk_embeddings(tenant_id);

-- HNSW vector index for cosine similarity search (ADR-003)
-- Using cosine distance operator class (vector_cosine_ops)
CREATE INDEX IF NOT EXISTS idx_chunk_embeddings_hnsw
    ON chunk_embeddings
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
