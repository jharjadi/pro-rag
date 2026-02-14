"""Ingestion pipeline orchestrator.

Orchestrates: extract → chunk → metadata → embed → write (with FTS).
Tracks ingestion runs (running → succeeded/failed).

Spec §3a: End-to-end pipeline for one document format (DOCX).
"""

from __future__ import annotations

import hashlib
import json
import logging
import os
import traceback
from pathlib import Path
from typing import Any

from ingest.chunk.chunker import Chunk, chunk_blocks
from ingest.chunk.metadata import generate_chunk_metadata
from ingest.config import IngestConfig
from ingest.db.writer import (
    create_ingestion_run,
    get_connection,
    update_ingestion_run_failure,
    update_ingestion_run_success,
    write_document,
)
from ingest.embed.embedder import embed_chunks
from ingest.extract.docx import extract_docx
from ingest.extract.html import extract_html
from ingest.extract.pdf import extract_pdf

logger = logging.getLogger(__name__)

# Supported file extensions → extractor functions
_EXTRACTORS = {
    ".docx": extract_docx,
    ".pdf": extract_pdf,
    ".html": extract_html,
    ".htm": extract_html,
}


def _compute_content_hash(file_path: Path) -> str:
    """Compute SHA-256 hash of file content."""
    sha256 = hashlib.sha256()
    with open(file_path, "rb") as f:
        for block in iter(lambda: f.read(8192), b""):
            sha256.update(block)
    return sha256.hexdigest()


def _detect_source_type(file_path: Path) -> str:
    """Detect source type from file extension."""
    ext = file_path.suffix.lower()
    type_map = {
        ".docx": "docx",
        ".pdf": "pdf",
        ".html": "html",
        ".htm": "html",
    }
    return type_map.get(ext, "unknown")


def _save_artifact(
    blocks_data: list[dict],
    tenant_id: str,
    doc_id: str,
    version_label: str,
    artifact_base_path: str,
) -> str | None:
    """Save extracted blocks as a JSON artifact.

    Returns:
        Artifact URI or None if save fails.
    """
    try:
        artifact_dir = Path(artifact_base_path) / tenant_id / doc_id
        artifact_dir.mkdir(parents=True, exist_ok=True)
        artifact_path = artifact_dir / f"{version_label}.json"
        with open(artifact_path, "w") as f:
            json.dump(blocks_data, f, indent=2)
        uri = f"file://{artifact_path}"
        logger.info("Saved artifact: %s", uri)
        return uri
    except Exception as e:
        logger.warning("Failed to save artifact: %s", e)
        return None


def ingest_document(
    file_path: str | Path,
    tenant_id: str,
    title: str,
    activate: bool = True,
    config: IngestConfig | None = None,
) -> dict[str, Any]:
    """Ingest a single document end-to-end.

    Pipeline stages:
    1. Extract structured blocks from document
    2. Chunk blocks (structure-aware)
    3. Generate metadata for each chunk
    4. Batch embed chunks
    5. Write to DB (document, version, chunks, embeddings, FTS)

    Args:
        file_path: Path to the document file.
        tenant_id: Tenant UUID.
        title: Document title.
        activate: Whether to activate the version immediately.
        config: Pipeline configuration (loads from env if None).

    Returns:
        Dict with doc_id, doc_version_id, num_chunks, skipped, run_id.
    """
    if config is None:
        config = IngestConfig.from_env()

    file_path = Path(file_path)
    if not file_path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")

    ext = file_path.suffix.lower()
    if ext not in _EXTRACTORS:
        raise ValueError(
            f"Unsupported file format: {ext}. Supported: {list(_EXTRACTORS.keys())}"
        )

    # Connect to DB
    conn = get_connection(config.database_url)
    run_id = None

    try:
        # Create ingestion run
        run_config = {
            "file_path": str(file_path),
            "tenant_id": tenant_id,
            "title": title,
            "activate": activate,
            "embedding_model": config.embedding_model,
            "chunk_target_tokens": config.chunk_target_tokens,
            "chunk_max_tokens": config.chunk_max_tokens,
            "chunk_hard_cap_tokens": config.chunk_hard_cap_tokens,
        }
        run_id = create_ingestion_run(conn, tenant_id, run_config)

        # Stage 1: Extract
        logger.info("Stage 1: Extracting blocks from %s", file_path.name)
        extractor = _EXTRACTORS[ext]
        blocks = extractor(file_path)
        logger.info("Extracted %d blocks", len(blocks))

        if not blocks:
            raise ValueError(f"No blocks extracted from {file_path}")

        # Stage 2: Chunk
        logger.info("Stage 2: Chunking %d blocks", len(blocks))
        chunks = chunk_blocks(
            blocks,
            target_tokens=config.chunk_target_tokens,
            min_tokens=config.chunk_min_tokens,
            max_tokens=config.chunk_max_tokens,
            hard_cap=config.chunk_hard_cap_tokens,
        )
        logger.info("Created %d chunks", len(chunks))

        if not chunks:
            raise ValueError(f"No chunks created from {file_path}")

        # Stage 3: Generate metadata
        logger.info("Stage 3: Generating metadata for %d chunks", len(chunks))
        for chunk in chunks:
            chunk.meta = generate_chunk_metadata(
                chunk.text,
                chunk.chunk_type,
                extra=chunk.meta if chunk.chunk_type == "table" else None,
            )

        # Stage 4: Embed
        logger.info("Stage 4: Embedding %d chunks", len(chunks))
        embeddings = embed_chunks(
            chunks,
            model_name=config.embedding_model,
            batch_size=config.embedding_batch_size,
        )
        logger.info("Generated %d embeddings (dim=%d)", len(embeddings), len(embeddings[0]))

        # Compute content hash
        content_hash = _compute_content_hash(file_path)
        source_uri = str(file_path.resolve())
        source_type = _detect_source_type(file_path)

        # Stage 5: Write to DB
        logger.info("Stage 5: Writing to database")
        result = write_document(
            conn=conn,
            tenant_id=tenant_id,
            source_type=source_type,
            source_uri=source_uri,
            title=title,
            content_hash=content_hash,
            chunks=chunks,
            embeddings=embeddings,
            embedding_model=config.embedding_model,
            activate=activate,
        )

        # Save artifact (best-effort, after DB write succeeds)
        if result["doc_id"] and not result["skipped"]:
            blocks_data = [
                {"type": b.type, "text": b.text, "meta": b.meta}
                for b in blocks
            ]
            artifact_uri = _save_artifact(
                blocks_data,
                tenant_id,
                result["doc_id"],
                f"v{result['doc_version_id'][:8]}",
                config.artifact_base_path,
            )
            # Update version with artifact URI if saved
            if artifact_uri and result["doc_version_id"]:
                with conn.cursor() as cur:
                    cur.execute(
                        "UPDATE document_versions SET extracted_artifact_uri = %s WHERE doc_version_id = %s",
                        (artifact_uri, result["doc_version_id"]),
                    )
                    conn.commit()

        # Update ingestion run
        total_tokens = sum(c.token_count for c in chunks)
        stats = {
            "docs_processed": 1,
            "chunks_created": result["num_chunks"],
            "tokens_total": total_tokens,
            "skipped": result["skipped"],
            "embedding_model": config.embedding_model,
        }
        update_ingestion_run_success(conn, run_id, stats)

        result["run_id"] = run_id
        return result

    except Exception as e:
        # Determine which stage failed
        stage = "unknown"
        tb = traceback.format_exc()
        if "Stage 1" in tb or "extract" in str(e).lower():
            stage = "extract"
        elif "Stage 2" in tb or "chunk" in str(e).lower():
            stage = "chunk"
        elif "Stage 3" in tb or "metadata" in str(e).lower():
            stage = "metadata"
        elif "Stage 4" in tb or "embed" in str(e).lower():
            stage = "embed"
        elif "Stage 5" in tb or "write" in str(e).lower() or "database" in str(e).lower():
            stage = "db_write"

        logger.error("Ingestion failed at stage '%s': %s", stage, e)

        if run_id:
            try:
                update_ingestion_run_failure(conn, run_id, str(e), stage)
            except Exception as run_err:
                logger.error("Failed to update ingestion run: %s", run_err)

        raise

    finally:
        conn.close()
