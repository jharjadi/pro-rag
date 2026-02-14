"""Batch embedding using sentence-transformers.

Spec §5: Embedding must be done in batches, not one-by-one.
Default model: BAAI/bge-base-en-v1.5 (768-dim).
BATCH_SIZE <= 256.
"""

from __future__ import annotations

import logging
from typing import TYPE_CHECKING

import numpy as np

if TYPE_CHECKING:
    from sentence_transformers import SentenceTransformer

    from ingest.chunk.chunker import Chunk

logger = logging.getLogger(__name__)

# Module-level model cache to avoid reloading
_model_cache: dict[str, SentenceTransformer] = {}


def _get_model(model_name: str) -> SentenceTransformer:
    """Load or retrieve cached sentence-transformers model."""
    if model_name not in _model_cache:
        from sentence_transformers import SentenceTransformer

        logger.info("Loading embedding model: %s", model_name)
        _model_cache[model_name] = SentenceTransformer(model_name)
        logger.info("Model loaded: %s", model_name)
    return _model_cache[model_name]


def embed_chunks(
    chunks: list[Chunk],
    model_name: str = "BAAI/bge-base-en-v1.5",
    batch_size: int = 256,
) -> list[list[float]]:
    """Batch-embed a list of chunks.

    Args:
        chunks: List of Chunk objects to embed.
        model_name: Sentence-transformers model name.
        batch_size: Max batch size for encoding (<=256).

    Returns:
        List of embedding vectors (list of floats), one per chunk.
        Order matches input chunks.
    """
    if not chunks:
        return []

    model = _get_model(model_name)
    texts = [chunk.text for chunk in chunks]

    logger.info(
        "Embedding %d chunks with model=%s, batch_size=%d",
        len(texts), model_name, batch_size,
    )

    # sentence-transformers handles batching internally, but we respect batch_size
    embeddings = model.encode(
        texts,
        batch_size=min(batch_size, 256),
        show_progress_bar=False,
        normalize_embeddings=True,  # Cosine similarity works better with normalized vectors
    )

    # Convert numpy arrays to lists of floats
    result = [emb.tolist() for emb in embeddings]

    dim = len(result[0]) if result else 0
    logger.info("Embedded %d chunks, dimension=%d", len(result), dim)

    return result
