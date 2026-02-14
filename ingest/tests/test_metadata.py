"""Tests for metadata generation and config module."""

from __future__ import annotations

import os

import pytest

from ingest.chunk.metadata import extract_keywords, generate_chunk_metadata
from ingest.config import IngestConfig


# ── Keyword extraction ───────────────────────────────────


class TestExtractKeywords:
    def test_basic_extraction(self):
        text = "Python programming language is great for data science and machine learning"
        keywords = extract_keywords(text)
        assert len(keywords) > 0
        assert all(isinstance(k, str) for k in keywords)
        # Stop words should be filtered
        assert "is" not in keywords
        assert "for" not in keywords
        assert "and" not in keywords

    def test_repeated_words_ranked_higher(self):
        text = "database database database query query index"
        keywords = extract_keywords(text)
        assert keywords[0] == "database"
        assert "query" in keywords

    def test_empty_text(self):
        keywords = extract_keywords("")
        assert keywords == []

    def test_only_stop_words(self):
        keywords = extract_keywords("the and or but in on at to for of")
        assert keywords == []

    def test_max_keywords_limit(self):
        text = " ".join(f"word{i}" * (10 - i) for i in range(20))
        keywords = extract_keywords(text, max_keywords=5)
        assert len(keywords) <= 5

    def test_short_words_filtered(self):
        """Words shorter than 3 chars should be filtered."""
        keywords = extract_keywords("I am a go to do it")
        # All these are either stop words or < 3 chars
        assert keywords == []


# ── Metadata generation ──────────────────────────────────


class TestGenerateChunkMetadata:
    def test_text_chunk_metadata(self):
        meta = generate_chunk_metadata(
            "Python is a programming language used for data science.",
            chunk_type="text",
        )
        assert "summary" in meta
        assert "keywords" in meta
        assert "hypothetical_questions" in meta
        assert isinstance(meta["keywords"], list)
        assert meta["hypothetical_questions"] == []  # V2 placeholder

    def test_table_chunk_metadata(self):
        meta = generate_chunk_metadata(
            "| Name | Age |\n| --- | --- |\n| Alice | 30 |",
            chunk_type="table",
            extra={"format": "markdown"},
        )
        assert "table" in meta
        assert meta["table"]["format"] == "markdown"

    def test_table_without_extra(self):
        meta = generate_chunk_metadata(
            "| Name | Age |",
            chunk_type="table",
        )
        assert "table" not in meta  # No extra provided

    def test_summary_empty_in_v1(self):
        meta = generate_chunk_metadata("Some text here.", chunk_type="text")
        assert meta["summary"] == ""


# ── Config ───────────────────────────────────────────────


class TestIngestConfig:
    def test_defaults(self):
        config = IngestConfig()
        assert config.embedding_model == "BAAI/bge-base-en-v1.5"
        assert config.embedding_dim == 768
        assert config.embedding_batch_size == 256
        assert config.chunk_target_tokens == 450
        assert config.chunk_min_tokens == 350
        assert config.chunk_max_tokens == 500
        assert config.chunk_hard_cap_tokens == 800
        assert config.chunk_overlap == 0

    def test_from_env(self, monkeypatch):
        monkeypatch.setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
        monkeypatch.setenv("EMBEDDING_MODEL", "test-model")
        monkeypatch.setenv("EMBEDDING_DIM", "384")
        monkeypatch.setenv("CHUNK_TARGET_TOKENS", "300")

        config = IngestConfig.from_env()
        assert config.database_url == "postgres://test:test@localhost:5432/test"
        assert config.embedding_model == "test-model"
        assert config.embedding_dim == 384
        assert config.chunk_target_tokens == 300

    def test_from_env_defaults(self, monkeypatch):
        # Clear relevant env vars to test defaults
        for key in [
            "DATABASE_URL", "EMBEDDING_MODEL", "EMBEDDING_DIM",
            "EMBEDDING_BATCH_SIZE", "CHUNK_TARGET_TOKENS",
        ]:
            monkeypatch.delenv(key, raising=False)

        config = IngestConfig.from_env()
        assert config.embedding_model == "BAAI/bge-base-en-v1.5"
        assert config.embedding_dim == 768
