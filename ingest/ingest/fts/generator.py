"""FTS (Full-Text Search) generation for pro-rag ingestion.

FTS tsvectors are generated server-side by Postgres using:
    to_tsvector('english', text)

This module provides the SQL and helpers for the DB writer to insert
chunk_fts rows. The actual tsvector computation happens in Postgres,
not in Python — this ensures consistency with the query runtime's
websearch_to_tsquery('english', ...).
"""

from __future__ import annotations

import logging

logger = logging.getLogger(__name__)

# SQL for inserting a chunk_fts row with server-side tsvector generation
INSERT_CHUNK_FTS_SQL = """
INSERT INTO chunk_fts (chunk_id, tenant_id, tsv)
VALUES (%s, %s, to_tsvector('english', %s))
ON CONFLICT (chunk_id) DO UPDATE SET tsv = EXCLUDED.tsv
"""


def get_fts_insert_params(
    chunk_id: str,
    tenant_id: str,
    text: str,
) -> tuple[str, str, str]:
    """Return parameters for the FTS insert SQL.

    The tsvector is computed server-side by Postgres.

    Args:
        chunk_id: UUID of the chunk.
        tenant_id: UUID of the tenant.
        text: Chunk text to index.

    Returns:
        Tuple of (chunk_id, tenant_id, text) for SQL parameterization.
    """
    return (chunk_id, tenant_id, text)
