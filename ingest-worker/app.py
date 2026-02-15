"""pro-rag Ingest Worker — Go-orchestrated ingestion worker (spec v2.3 §7).

Accepts job payloads from Go via POST /internal/process.
Runs the ingestion pipeline (extract → chunk → embed → FTS → write).
Updates ingestion_runs status with heartbeats.

This is an internal-only service — not publicly exposed.
"""

from __future__ import annotations

import json
import logging
import os
import time
import traceback
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timezone
from pathlib import Path
from threading import Lock
from typing import Any

import psycopg2
import psycopg2.extras
from flask import Flask, jsonify, request

# ── Configuration ─────────────────────────────────────────

DATABASE_URL = os.environ.get(
    "DATABASE_URL",
    "postgres://prorag:prorag_dev@localhost:5433/prorag?sslmode=disable",
)
INTERNAL_AUTH_TOKEN = os.environ.get("INTERNAL_AUTH_TOKEN", "")
MAX_CONCURRENT_JOBS = int(os.environ.get("WORKER_MAX_CONCURRENT_JOBS", "3"))
ARTIFACT_BASE_PATH = os.environ.get("ARTIFACT_BASE_PATH", "/data/artifacts")
STALE_RUNNING_MINUTES = 15

# ── Logging ───────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s — %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S",
)
logger = logging.getLogger("ingest-worker")

# Register UUID adapter for psycopg2
psycopg2.extras.register_uuid()

# ── Flask App ─────────────────────────────────────────────

app = Flask(__name__)

# Bounded concurrency pool
_executor = ThreadPoolExecutor(max_workers=MAX_CONCURRENT_JOBS)
_active_jobs: set[str] = set()
_jobs_lock = Lock()


def _get_db_connection():
    """Get a psycopg2 connection."""
    return psycopg2.connect(DATABASE_URL)


# ── Health Check ──────────────────────────────────────────

@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint."""
    with _jobs_lock:
        active = len(_active_jobs)
    return jsonify({
        "status": "ok",
        "active_jobs": active,
        "max_concurrent": MAX_CONCURRENT_JOBS,
    })


# ── Internal Process Endpoint ─────────────────────────────

@app.route("/internal/process", methods=["POST"])
def process():
    """Accept a job payload from Go and process it in the background.

    Spec v2.3 §7.2:
    - 202 Accepted: job queued internally
    - 401 Unauthorized: invalid auth token
    - 503 Service Unavailable: all slots occupied
    """
    # Validate internal auth token
    if INTERNAL_AUTH_TOKEN:
        auth_header = request.headers.get("Authorization", "")
        if not auth_header.startswith("Bearer "):
            return jsonify({"error": "missing authorization"}), 401
        token = auth_header[len("Bearer "):]
        if token != INTERNAL_AUTH_TOKEN:
            return jsonify({"error": "invalid authorization"}), 401

    # Parse job payload
    payload = request.get_json(silent=True)
    if not payload:
        return jsonify({"error": "invalid JSON payload"}), 400

    run_id = payload.get("run_id")
    if not run_id:
        return jsonify({"error": "run_id is required"}), 400

    # Check concurrency
    with _jobs_lock:
        if len(_active_jobs) >= MAX_CONCURRENT_JOBS:
            logger.warning("Worker busy — all %d slots occupied", MAX_CONCURRENT_JOBS)
            return jsonify({"error": "worker busy"}), 503
        _active_jobs.add(run_id)

    # Submit to thread pool
    _executor.submit(_process_job, payload)

    logger.info("Job accepted: run_id=%s (active=%d/%d)",
                run_id, len(_active_jobs), MAX_CONCURRENT_JOBS)

    return jsonify({"status": "accepted", "run_id": run_id}), 202


# ── Job Processing ────────────────────────────────────────

def _heartbeat(conn, run_id: str) -> None:
    """Update ingestion_runs.updated_at as a heartbeat (spec v2.3 §7.3)."""
    try:
        with conn.cursor() as cur:
            cur.execute(
                "UPDATE ingestion_runs SET updated_at = now() WHERE run_id = %s",
                (run_id,),
            )
            conn.commit()
    except Exception as e:
        logger.warning("Heartbeat failed for run_id=%s: %s", run_id, e)


def _transition_to_running(conn, run_id: str) -> bool:
    """Transition run from queued/failed to running (spec v2.3 §7.2).

    Returns True if transition succeeded, False if should skip.
    """
    with conn.cursor() as cur:
        # Try to transition queued → running
        cur.execute(
            """
            UPDATE ingestion_runs
            SET status = 'running',
                started_at = COALESCE(started_at, now()),
                updated_at = now()
            WHERE run_id = %s AND status IN ('queued', 'failed')
            """,
            (run_id,),
        )
        conn.commit()

        if cur.rowcount > 0:
            return True

        # Check current status
        cur.execute(
            "SELECT status, updated_at FROM ingestion_runs WHERE run_id = %s",
            (run_id,),
        )
        row = cur.fetchone()
        if row is None:
            logger.error("Run not found: run_id=%s", run_id)
            return False

        status, updated_at = row

        if status == "succeeded":
            logger.info("Run already succeeded, skipping: run_id=%s", run_id)
            return False

        if status == "running":
            # Check if stale (another worker crashed)
            if updated_at and (datetime.now(timezone.utc) - updated_at.replace(tzinfo=timezone.utc)).total_seconds() > STALE_RUNNING_MINUTES * 60:
                logger.warning("Stale running run detected, re-processing: run_id=%s", run_id)
                cur.execute(
                    """
                    UPDATE ingestion_runs
                    SET updated_at = now()
                    WHERE run_id = %s AND status = 'running'
                    """,
                    (run_id,),
                )
                conn.commit()
                return True
            else:
                logger.info("Run is actively being processed, skipping: run_id=%s", run_id)
                return False

    return False


def _process_job(payload: dict[str, Any]) -> None:
    """Process a single ingestion job (spec v2.3 §7.2).

    Runs in a background thread from the ThreadPoolExecutor.
    """
    run_id = payload["run_id"]
    doc_id = payload["doc_id"]
    tenant_id = payload["tenant_id"]
    upload_uri = payload["upload_uri"]
    title = payload["title"]
    source_type = payload.get("source_type", "unknown")
    source_uri = payload.get("source_uri", "")
    content_hash = payload.get("content_hash", "")

    start_time = time.monotonic()
    conn = None

    try:
        conn = _get_db_connection()

        # Step 1: Transition to running
        if not _transition_to_running(conn, run_id):
            logger.info("Skipping job: run_id=%s", run_id)
            return

        logger.info("Processing job: run_id=%s, doc_id=%s, file=%s", run_id, doc_id, upload_uri)

        # Step 2: Read raw file from upload_uri
        file_path = _resolve_upload_uri(upload_uri)
        if not file_path.exists():
            raise FileNotFoundError(f"Upload file not found: {file_path}")

        # Step 3: Run ingestion pipeline with heartbeats
        # Import here to avoid loading heavy ML models at import time
        from ingest.chunk.chunker import chunk_blocks
        from ingest.chunk.metadata import generate_chunk_metadata
        from ingest.config import IngestConfig
        from ingest.embed.embedder import embed_chunks
        from ingest.extract.docx import extract_docx
        from ingest.extract.html import extract_html
        from ingest.extract.pdf import extract_pdf
        from ingest.fts.generator import INSERT_CHUNK_FTS_SQL, get_fts_insert_params

        config = IngestConfig.from_env()

        # Detect extractor
        ext = file_path.suffix.lower()
        extractors = {
            ".docx": extract_docx,
            ".pdf": extract_pdf,
            ".html": extract_html,
            ".htm": extract_html,
        }
        extractor = extractors.get(ext)
        if not extractor:
            raise ValueError(f"Unsupported file format: {ext}")

        # Stage 1: Extract
        logger.info("[%s] Stage 1: Extracting blocks", run_id)
        blocks = extractor(file_path)
        if not blocks:
            raise ValueError(f"No blocks extracted from {file_path}")
        logger.info("[%s] Extracted %d blocks", run_id, len(blocks))
        _heartbeat(conn, run_id)

        # Stage 2: Chunk
        logger.info("[%s] Stage 2: Chunking %d blocks", run_id, len(blocks))
        chunks = chunk_blocks(
            blocks,
            target_tokens=config.chunk_target_tokens,
            min_tokens=config.chunk_min_tokens,
            max_tokens=config.chunk_max_tokens,
            hard_cap=config.chunk_hard_cap_tokens,
        )
        if not chunks:
            raise ValueError(f"No chunks created from {file_path}")
        logger.info("[%s] Created %d chunks", run_id, len(chunks))
        _heartbeat(conn, run_id)

        # Stage 3: Generate metadata
        logger.info("[%s] Stage 3: Generating metadata", run_id)
        for chunk in chunks:
            chunk.meta = generate_chunk_metadata(
                chunk.text,
                chunk.chunk_type,
                extra=chunk.meta if chunk.chunk_type == "table" else None,
            )
        _heartbeat(conn, run_id)

        # Stage 4: Embed
        logger.info("[%s] Stage 4: Embedding %d chunks", run_id, len(chunks))
        embeddings = embed_chunks(
            chunks,
            model_name=config.embedding_model,
            batch_size=config.embedding_batch_size,
        )
        logger.info("[%s] Generated %d embeddings (dim=%d)", run_id, len(embeddings), len(embeddings[0]))
        _heartbeat(conn, run_id)

        # Stage 5: Write content tables atomically (spec v2.3 §7.2 step 4)
        logger.info("[%s] Stage 5: Writing to database", run_id)

        import uuid as uuid_mod

        version_label = f"v{datetime.now(timezone.utc).strftime('%Y%m%d%H%M%S')}"
        doc_version_id = str(uuid_mod.uuid4())

        with conn.cursor() as cur:
            # Deactivate any existing active version for this doc
            cur.execute(
                """
                UPDATE document_versions
                SET is_active = false
                WHERE doc_id = %s AND tenant_id = %s AND is_active = true
                """,
                (doc_id, tenant_id),
            )

            # Create new document_versions row with content_hash
            cur.execute(
                """
                INSERT INTO document_versions
                    (doc_version_id, tenant_id, doc_id, version_label, is_active, content_hash)
                VALUES (%s, %s, %s, %s, true, %s)
                """,
                (doc_version_id, tenant_id, doc_id, version_label, content_hash),
            )

            # Insert chunks, embeddings, and FTS
            for chunk, embedding in zip(chunks, embeddings):
                chunk_id = str(uuid_mod.uuid4())

                metadata = chunk.meta.copy() if chunk.meta else {}
                cur.execute(
                    """
                    INSERT INTO chunks
                        (chunk_id, tenant_id, doc_version_id, ordinal, heading_path,
                         chunk_type, text, token_count, metadata)
                    VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
                    """,
                    (
                        chunk_id, tenant_id, doc_version_id,
                        chunk.ordinal, json.dumps(chunk.heading_path),
                        chunk.chunk_type, chunk.text, chunk.token_count,
                        json.dumps(metadata),
                    ),
                )

                # Insert embedding
                embedding_str = "[" + ",".join(str(v) for v in embedding) + "]"
                cur.execute(
                    """
                    INSERT INTO chunk_embeddings (chunk_id, tenant_id, embedding_model, embedding)
                    VALUES (%s, %s, %s, %s)
                    """,
                    (chunk_id, tenant_id, config.embedding_model, embedding_str),
                )

                # Insert FTS
                fts_params = get_fts_insert_params(chunk_id, tenant_id, chunk.text)
                cur.execute(INSERT_CHUNK_FTS_SQL, fts_params)

            conn.commit()

        logger.info("[%s] Wrote %d chunks for version %s", run_id, len(chunks), version_label)

        # Save artifact (best-effort)
        _save_artifact(blocks, tenant_id, doc_id, version_label, doc_version_id, conn)

        # Update run to succeeded
        total_tokens = sum(c.token_count for c in chunks)
        duration_ms = int((time.monotonic() - start_time) * 1000)
        stats = {
            "docs_processed": 1,
            "chunks_created": len(chunks),
            "tokens_total": total_tokens,
            "embedding_model": config.embedding_model,
            "duration_ms": duration_ms,
        }

        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE ingestion_runs
                SET status = 'succeeded', finished_at = now(), updated_at = now(), stats = %s
                WHERE run_id = %s
                """,
                (json.dumps(stats), run_id),
            )
            conn.commit()

        # Cleanup raw upload on success
        try:
            file_path.unlink(missing_ok=True)
            # Try to remove the parent directory if empty
            file_path.parent.rmdir()
        except OSError:
            pass

        logger.info(
            json.dumps({
                "event": "ingest_job_complete",
                "run_id": run_id,
                "tenant_id": tenant_id,
                "doc_id": doc_id,
                "status": "succeeded",
                "chunks_created": len(chunks),
                "tokens_total": total_tokens,
                "duration_ms": duration_ms,
            })
        )

    except Exception as e:
        duration_ms = int((time.monotonic() - start_time) * 1000)
        error_msg = str(e)
        logger.error("[%s] Job failed: %s\n%s", run_id, error_msg, traceback.format_exc())

        # Update run to failed
        if conn:
            try:
                with conn.cursor() as cur:
                    cur.execute(
                        """
                        UPDATE ingestion_runs
                        SET status = 'failed', finished_at = now(), updated_at = now(), error = %s
                        WHERE run_id = %s
                        """,
                        (error_msg, run_id),
                    )
                    conn.commit()
            except Exception as db_err:
                logger.error("[%s] Failed to update run status: %s", run_id, db_err)

        logger.info(
            json.dumps({
                "event": "ingest_job_complete",
                "run_id": run_id,
                "tenant_id": tenant_id,
                "doc_id": doc_id,
                "status": "failed",
                "error": error_msg,
                "duration_ms": duration_ms,
            })
        )

    finally:
        # Release concurrency slot
        with _jobs_lock:
            _active_jobs.discard(run_id)

        if conn:
            try:
                conn.close()
            except Exception:
                pass


def _resolve_upload_uri(upload_uri: str) -> Path:
    """Convert upload_uri to a local filesystem path.

    V1: file:///data/uploads/... → /data/uploads/...
    """
    if upload_uri.startswith("file://"):
        return Path(upload_uri[len("file://"):])
    return Path(upload_uri)


def _save_artifact(
    blocks: list,
    tenant_id: str,
    doc_id: str,
    version_label: str,
    doc_version_id: str,
    conn,
) -> None:
    """Save extracted blocks as a JSON artifact (best-effort)."""
    try:
        artifact_dir = Path(ARTIFACT_BASE_PATH) / tenant_id / doc_id
        artifact_dir.mkdir(parents=True, exist_ok=True)
        artifact_path = artifact_dir / f"{version_label}.json"

        blocks_data = [
            {"type": b.type, "text": b.text, "meta": b.meta}
            for b in blocks
        ]
        with open(artifact_path, "w") as f:
            json.dump(blocks_data, f, indent=2)

        artifact_uri = f"file://{artifact_path}"

        with conn.cursor() as cur:
            cur.execute(
                "UPDATE document_versions SET extracted_artifact_uri = %s WHERE doc_version_id = %s",
                (artifact_uri, doc_version_id),
            )
            conn.commit()

        logger.info("Saved artifact: %s", artifact_uri)
    except Exception as e:
        logger.warning("Failed to save artifact: %s", e)


# ── Main ──────────────────────────────────────────────────

if __name__ == "__main__":
    port = int(os.environ.get("WORKER_PORT", "8002"))
    logger.info(
        "Ingest worker starting (max_concurrent=%d, port=%d)",
        MAX_CONCURRENT_JOBS, port,
    )
    app.run(host="0.0.0.0", port=port, threaded=True)
