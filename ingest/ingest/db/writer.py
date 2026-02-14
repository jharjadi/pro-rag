"""Database writer for pro-rag ingestion pipeline.

Handles:
- Document + version creation with activation logic
- content_hash deduplication (skip if same hash already active)
- Chunk + embedding + FTS writes
- All writes within a single transaction
- Ingestion run tracking

Spec references:
- Version activation: §3a.6
- content_hash dedup: §3a.6
- Tenant isolation: every write includes tenant_id
"""

from __future__ import annotations

import json
import logging
import uuid
from datetime import datetime, timezone
from typing import Any

import psycopg2
import psycopg2.extras

from ingest.chunk.chunker import Chunk
from ingest.fts.generator import INSERT_CHUNK_FTS_SQL, get_fts_insert_params

logger = logging.getLogger(__name__)

# Register UUID adapter for psycopg2
psycopg2.extras.register_uuid()


def get_connection(database_url: str):
    """Create a psycopg2 connection from a DATABASE_URL."""
    return psycopg2.connect(database_url)


def check_existing_document(
    conn,
    tenant_id: str,
    source_uri: str,
    content_hash: str,
) -> dict[str, Any] | None:
    """Check if a document with the same source_uri exists for this tenant.

    Returns:
        Dict with doc_id, content_hash, has_active_version info, or None if not found.
    """
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT d.doc_id, d.content_hash,
                   EXISTS(
                       SELECT 1 FROM document_versions dv
                       WHERE dv.doc_id = d.doc_id
                         AND dv.tenant_id = d.tenant_id
                         AND dv.is_active = true
                   ) as has_active_version
            FROM documents d
            WHERE d.tenant_id = %s AND d.source_uri = %s
            ORDER BY d.created_at DESC
            LIMIT 1
            """,
            (tenant_id, source_uri),
        )
        row = cur.fetchone()
        if row is None:
            return None
        return {
            "doc_id": str(row[0]),
            "content_hash": row[1],
            "has_active_version": row[2],
        }


def write_document(
    conn,
    tenant_id: str,
    source_type: str,
    source_uri: str,
    title: str,
    content_hash: str,
    chunks: list[Chunk],
    embeddings: list[list[float]],
    embedding_model: str,
    activate: bool = True,
    version_label: str | None = None,
    artifact_uri: str | None = None,
) -> dict[str, Any]:
    """Write a complete document with all related data in a single transaction.

    Handles:
    1. content_hash dedup: skip if same (tenant, source_uri, content_hash) already active
    2. New document: create doc + version + chunks + embeddings + FTS
    3. New version: deactivate old version, create new version + chunks + embeddings + FTS

    Args:
        conn: psycopg2 connection.
        tenant_id: Tenant UUID.
        source_type: Document type (e.g., "docx").
        source_uri: Source file path/URI.
        title: Document title.
        content_hash: SHA-256 of raw file content.
        chunks: List of Chunk objects.
        embeddings: List of embedding vectors (parallel to chunks).
        embedding_model: Name of the embedding model used.
        activate: Whether to activate the version immediately.
        version_label: Version label (auto-generated if None).
        artifact_uri: Path to extracted artifact JSON.

    Returns:
        Dict with doc_id, doc_version_id, num_chunks, skipped (bool).
    """
    if len(chunks) != len(embeddings):
        raise ValueError(
            f"chunks ({len(chunks)}) and embeddings ({len(embeddings)}) must have same length"
        )

    # Check for existing document with same source_uri
    existing = check_existing_document(conn, tenant_id, source_uri, content_hash)

    if existing and existing["content_hash"] == content_hash and existing["has_active_version"]:
        logger.info(
            "Document already ingested with same content_hash, skipping: %s (doc_id=%s)",
            source_uri, existing["doc_id"],
        )
        return {
            "doc_id": existing["doc_id"],
            "doc_version_id": None,
            "num_chunks": 0,
            "skipped": True,
        }

    # Generate version label if not provided
    if version_label is None:
        version_label = f"v{datetime.now(timezone.utc).strftime('%Y%m%d%H%M%S')}"

    try:
        with conn.cursor() as cur:
            # Start transaction (psycopg2 auto-begins)

            if existing:
                # Existing document — new version
                doc_id = existing["doc_id"]

                # Update content_hash on the document
                cur.execute(
                    "UPDATE documents SET content_hash = %s WHERE doc_id = %s AND tenant_id = %s",
                    (content_hash, doc_id, tenant_id),
                )

                if activate:
                    # Deactivate old version(s)
                    cur.execute(
                        """
                        UPDATE document_versions
                        SET is_active = false
                        WHERE doc_id = %s AND tenant_id = %s AND is_active = true
                        """,
                        (doc_id, tenant_id),
                    )
                    logger.info("Deactivated old version(s) for doc_id=%s", doc_id)
            else:
                # New document
                doc_id = str(uuid.uuid4())
                cur.execute(
                    """
                    INSERT INTO documents (doc_id, tenant_id, source_type, source_uri, title, content_hash)
                    VALUES (%s, %s, %s, %s, %s, %s)
                    """,
                    (doc_id, tenant_id, source_type, source_uri, title, content_hash),
                )

            # Create document version
            doc_version_id = str(uuid.uuid4())
            cur.execute(
                """
                INSERT INTO document_versions
                    (doc_version_id, tenant_id, doc_id, version_label, is_active, extracted_artifact_uri)
                VALUES (%s, %s, %s, %s, %s, %s)
                """,
                (doc_version_id, tenant_id, doc_id, version_label, activate, artifact_uri),
            )

            # Insert chunks, embeddings, and FTS
            for i, (chunk, embedding) in enumerate(zip(chunks, embeddings)):
                chunk_id = str(uuid.uuid4())

                # Insert chunk
                metadata = chunk.meta.copy() if chunk.meta else {}
                cur.execute(
                    """
                    INSERT INTO chunks
                        (chunk_id, tenant_id, doc_version_id, ordinal, heading_path,
                         chunk_type, text, token_count, metadata)
                    VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
                    """,
                    (
                        chunk_id,
                        tenant_id,
                        doc_version_id,
                        chunk.ordinal,
                        json.dumps(chunk.heading_path),
                        chunk.chunk_type,
                        chunk.text,
                        chunk.token_count,
                        json.dumps(metadata),
                    ),
                )

                # Insert embedding
                # pgvector expects the vector as a string like '[0.1, 0.2, ...]'
                embedding_str = "[" + ",".join(str(v) for v in embedding) + "]"
                cur.execute(
                    """
                    INSERT INTO chunk_embeddings (chunk_id, tenant_id, embedding_model, embedding)
                    VALUES (%s, %s, %s, %s)
                    """,
                    (chunk_id, tenant_id, embedding_model, embedding_str),
                )

                # Insert FTS (tsvector computed server-side)
                fts_params = get_fts_insert_params(chunk_id, tenant_id, chunk.text)
                cur.execute(INSERT_CHUNK_FTS_SQL, fts_params)

            conn.commit()

            logger.info(
                "Wrote document: doc_id=%s, version=%s, chunks=%d, activate=%s",
                doc_id, version_label, len(chunks), activate,
            )

            return {
                "doc_id": doc_id,
                "doc_version_id": doc_version_id,
                "num_chunks": len(chunks),
                "skipped": False,
            }

    except Exception:
        conn.rollback()
        raise


# ── Ingestion Run Tracking ───────────────────────────────


def create_ingestion_run(
    conn,
    tenant_id: str,
    config: dict[str, Any] | None = None,
) -> str:
    """Create an ingestion_runs row with status='running'.

    Returns:
        run_id (UUID string).
    """
    run_id = str(uuid.uuid4())
    with conn.cursor() as cur:
        cur.execute(
            """
            INSERT INTO ingestion_runs (run_id, tenant_id, status, config)
            VALUES (%s, %s, 'running', %s)
            """,
            (run_id, tenant_id, json.dumps(config or {})),
        )
        conn.commit()
    logger.info("Created ingestion run: run_id=%s", run_id)
    return run_id


def update_ingestion_run_success(
    conn,
    run_id: str,
    stats: dict[str, Any],
) -> None:
    """Mark an ingestion run as succeeded."""
    with conn.cursor() as cur:
        cur.execute(
            """
            UPDATE ingestion_runs
            SET status = 'succeeded',
                finished_at = now(),
                stats = %s
            WHERE run_id = %s
            """,
            (json.dumps(stats), run_id),
        )
        conn.commit()
    logger.info("Ingestion run succeeded: run_id=%s, stats=%s", run_id, stats)


def update_ingestion_run_failure(
    conn,
    run_id: str,
    error_msg: str,
    stage: str = "unknown",
) -> None:
    """Mark an ingestion run as failed."""
    error_text = f"[{stage}] {error_msg}"
    with conn.cursor() as cur:
        cur.execute(
            """
            UPDATE ingestion_runs
            SET status = 'failed',
                finished_at = now(),
                error = %s
            WHERE run_id = %s
            """,
            (error_text, run_id),
        )
        conn.commit()
    logger.info("Ingestion run failed: run_id=%s, stage=%s", run_id, stage)
