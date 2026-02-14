"""Configuration for pro-rag ingestion pipeline.

Loads settings from environment variables with sensible defaults.
"""

from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass
class IngestConfig:
    """Ingestion pipeline configuration."""

    # Database
    database_url: str = ""

    # Embedding
    embedding_model: str = "BAAI/bge-base-en-v1.5"
    embedding_dim: int = 768
    embedding_batch_size: int = 256

    # Chunking
    chunk_target_tokens: int = 450
    chunk_min_tokens: int = 350
    chunk_max_tokens: int = 500
    chunk_hard_cap_tokens: int = 800
    chunk_overlap: int = 0

    # Artifacts
    artifact_base_path: str = "/data/artifacts"

    @classmethod
    def from_env(cls) -> IngestConfig:
        """Load configuration from environment variables."""
        return cls(
            database_url=os.environ.get(
                "DATABASE_URL",
                "postgres://prorag:prorag_dev@localhost:5433/prorag?sslmode=disable",
            ),
            embedding_model=os.environ.get("EMBEDDING_MODEL", "BAAI/bge-base-en-v1.5"),
            embedding_dim=int(os.environ.get("EMBEDDING_DIM", "768")),
            embedding_batch_size=int(os.environ.get("EMBEDDING_BATCH_SIZE", "256")),
            chunk_target_tokens=int(os.environ.get("CHUNK_TARGET_TOKENS", "450")),
            chunk_min_tokens=int(os.environ.get("CHUNK_MIN_TOKENS", "350")),
            chunk_max_tokens=int(os.environ.get("CHUNK_MAX_TOKENS", "500")),
            chunk_hard_cap_tokens=int(os.environ.get("CHUNK_HARD_CAP_TOKENS", "800")),
            chunk_overlap=int(os.environ.get("CHUNK_OVERLAP", "0")),
            artifact_base_path=os.environ.get("ARTIFACT_BASE_PATH", "/data/artifacts"),
        )
