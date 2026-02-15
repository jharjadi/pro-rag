-- 010_documents_orchestration.sql: Move content_hash to document_versions,
-- add unique constraint on documents(tenant_id, source_uri) for INSERT ON CONFLICT dedup.
-- Spec v2.3 §9.1, §9.2

-- Step 1: Add content_hash to document_versions (nullable initially for migration)
ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS content_hash TEXT;

-- Step 2: Migrate existing content_hash from documents to their active document_versions
UPDATE document_versions dv
SET content_hash = d.content_hash
FROM documents d
WHERE dv.doc_id = d.doc_id
  AND dv.tenant_id = d.tenant_id
  AND dv.content_hash IS NULL
  AND d.content_hash IS NOT NULL
  AND d.content_hash != '';

-- Step 3: Set a default for any remaining NULL content_hash values
UPDATE document_versions
SET content_hash = 'sha256:unknown'
WHERE content_hash IS NULL;

-- Step 4: Make content_hash NOT NULL now that all rows have values
ALTER TABLE document_versions ALTER COLUMN content_hash SET NOT NULL;

-- Step 5: Drop content_hash from documents table
ALTER TABLE documents DROP COLUMN IF EXISTS content_hash;

-- Step 6: Add unique index on (tenant_id, source_uri) for INSERT ON CONFLICT dedup
-- Drop the old non-unique index first if it exists
DROP INDEX IF EXISTS idx_documents_tenant_source;
CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_source_uri
    ON documents(tenant_id, source_uri);

-- Step 7: Add dedup index on document_versions for active version hash lookup
CREATE INDEX IF NOT EXISTS idx_doc_versions_dedup
    ON document_versions(tenant_id, doc_id, content_hash)
    WHERE is_active = true;
