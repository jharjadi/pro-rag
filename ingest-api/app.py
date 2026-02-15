"""pro-rag Ingest API — HTTP wrapper around the ingestion pipeline.

Thin FastAPI service that accepts file uploads and runs ingestion asynchronously.
This is an internal-only service — all external traffic routes through Go (core-api-go).

Spec: plans/web-ui-spec.md §Phase 7b
"""

from __future__ import annotations

import logging
import os
import tempfile
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import psycopg2
from fastapi import BackgroundTasks, FastAPI, File, Form, HTTPException, UploadFile
from fastapi.responses import JSONResponse

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s — %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S",
)
logger = logging.getLogger(__name__)

app = FastAPI(title="pro-rag Ingest API", version="0.1.0")

# Config
DATABASE_URL = os.environ.get(
    "DATABASE_URL",
    "postgres://prorag:prorag_dev@localhost:5433/prorag?sslmode=disable",
)
MAX_UPLOAD_SIZE_MB = int(os.environ.get("MAX_UPLOAD_SIZE_MB", "50"))
MAX_UPLOAD_SIZE_BYTES = MAX_UPLOAD_SIZE_MB * 1024 * 1024

SUPPORTED_EXTENSIONS = {".docx", ".pdf", ".html", ".htm"}


def _get_db_connection():
    """Get a psycopg2 connection."""
    return psycopg2.connect(DATABASE_URL)


def _mark_stale_runs():
    """Crash guard: mark stale 'running' runs as 'failed' on startup.

    Finds any ingestion_runs with status='running' older than 10 minutes
    and marks them as failed. Prevents permanently-spinning indicators
    in the UI after a container restart.
    """
    try:
        conn = _get_db_connection()
        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE ingestion_runs
                SET status = 'failed',
                    error = 'interrupted — service restarted',
                    finished_at = NOW()
                WHERE status = 'running'
                  AND started_at < NOW() - INTERVAL '10 minutes'
                """
            )
            count = cur.rowcount
            conn.commit()
        conn.close()
        if count > 0:
            logger.warning("Marked %d stale ingestion runs as failed", count)
    except Exception as e:
        logger.error("Failed to mark stale runs: %s", e)


@app.on_event("startup")
async def startup_event():
    """Run crash guard on startup."""
    _mark_stale_runs()
    logger.info("Ingest API started (max upload: %dMB)", MAX_UPLOAD_SIZE_MB)


@app.get("/health")
async def health():
    """Health check endpoint."""
    return {"status": "ok"}


def _run_ingestion(
    file_path: str,
    tenant_id: str,
    title: str,
    run_id: str,
    doc_id: str,
):
    """Background task: run the ingestion pipeline.

    This imports and calls the existing pipeline.ingest_document function.
    On success/failure, the ingestion_runs table is updated by the pipeline itself.
    """
    try:
        # Import here to avoid loading heavy ML models at import time
        from ingest.config import IngestConfig
        from ingest.pipeline import ingest_document

        logger.info("Starting ingestion: run_id=%s, file=%s", run_id, file_path)

        config = IngestConfig.from_env()
        result = ingest_document(
            file_path=file_path,
            tenant_id=tenant_id,
            title=title,
            activate=True,
            config=config,
        )

        logger.info(
            "Ingestion complete: run_id=%s, doc_id=%s, chunks=%d, skipped=%s",
            run_id,
            result.get("doc_id"),
            result.get("num_chunks", 0),
            result.get("skipped", False),
        )

    except Exception as e:
        logger.error("Ingestion failed: run_id=%s, error=%s", run_id, e)
        # The pipeline's own error handling updates ingestion_runs,
        # but if it fails before creating the run, we update manually
        try:
            conn = _get_db_connection()
            with conn.cursor() as cur:
                cur.execute(
                    """
                    UPDATE ingestion_runs
                    SET status = 'failed', error = %s, finished_at = NOW()
                    WHERE run_id = %s AND status = 'running'
                    """,
                    (str(e), run_id),
                )
                conn.commit()
            conn.close()
        except Exception as db_err:
            logger.error("Failed to update run status: %s", db_err)

    finally:
        # Clean up temp file
        try:
            Path(file_path).unlink(missing_ok=True)
        except Exception:
            pass


@app.post("/ingest")
async def ingest(
    background_tasks: BackgroundTasks,
    file: UploadFile = File(...),
    tenant_id: str = Form(...),
    title: str = Form(...),
):
    """Accept file upload and trigger async ingestion.

    Returns immediately with run_id and doc_id.
    The caller polls GET /v1/ingestion-runs/:run_id for completion.
    """
    # Validate tenant_id
    if not tenant_id:
        raise HTTPException(status_code=400, detail="tenant_id is required")

    # Validate title
    if not title:
        raise HTTPException(status_code=400, detail="title is required")

    # Validate file extension
    if file.filename:
        ext = Path(file.filename).suffix.lower()
        if ext not in SUPPORTED_EXTENSIONS:
            raise HTTPException(
                status_code=400,
                detail=f"Unsupported file format: {ext}. Supported: {sorted(SUPPORTED_EXTENSIONS)}",
            )
    else:
        raise HTTPException(status_code=400, detail="filename is required")

    # Read file content (enforce size limit)
    content = await file.read()
    if len(content) > MAX_UPLOAD_SIZE_BYTES:
        raise HTTPException(
            status_code=400,
            detail=f"File too large: {len(content)} bytes. Max: {MAX_UPLOAD_SIZE_BYTES} bytes ({MAX_UPLOAD_SIZE_MB}MB)",
        )

    # Save to temp file
    ext = Path(file.filename).suffix.lower()
    tmp = tempfile.NamedTemporaryFile(delete=False, suffix=ext, dir="/tmp")
    tmp.write(content)
    tmp.close()
    temp_path = tmp.name

    # Create ingestion run in DB
    run_id = str(uuid.uuid4())
    doc_id = str(uuid.uuid4())  # Pre-generate for immediate response

    try:
        conn = _get_db_connection()
        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO ingestion_runs (run_id, tenant_id, status, started_at, config)
                VALUES (%s, %s, 'running', %s, %s)
                """,
                (
                    run_id,
                    tenant_id,
                    datetime.now(timezone.utc),
                    '{"source": "api", "title": "' + title.replace('"', '\\"') + '"}',
                ),
            )
            conn.commit()
        conn.close()
    except Exception as e:
        logger.error("Failed to create ingestion run: %s", e)
        Path(temp_path).unlink(missing_ok=True)
        raise HTTPException(status_code=500, detail="Failed to create ingestion run")

    # Spawn background task
    background_tasks.add_task(
        _run_ingestion,
        file_path=temp_path,
        tenant_id=tenant_id,
        title=title,
        run_id=run_id,
        doc_id=doc_id,
    )

    return JSONResponse(
        status_code=202,
        content={
            "status": "processing",
            "run_id": run_id,
            "doc_id": doc_id,
        },
    )


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8002)
