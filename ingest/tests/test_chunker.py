"""Tests for structure-aware chunker.

Covers: chunk boundaries, table handling, heading path tracking,
token limits (target, max, hard cap).
"""

from __future__ import annotations

import pytest

from ingest.extract import Block
from ingest.chunk.chunker import (
    Chunk,
    chunk_blocks,
    count_tokens,
    _split_sentences,
    DEFAULT_TARGET_TOKENS,
    DEFAULT_MAX_TOKENS,
    DEFAULT_HARD_CAP_TOKENS,
)


# ── Helpers ──────────────────────────────────────────────


def _make_paragraph(text: str) -> Block:
    return Block(type="paragraph", text=text)


def _make_heading(text: str, level: int = 1) -> Block:
    return Block(type="heading", text=text, meta={"level": level})


def _make_table(rows: list[list[str]]) -> Block:
    """Build a markdown table Block from row data."""
    if not rows:
        return Block(type="table", text="", meta={"format": "markdown"})
    lines = []
    lines.append("| " + " | ".join(rows[0]) + " |")
    lines.append("| " + " | ".join("---" for _ in rows[0]) + " |")
    for row in rows[1:]:
        lines.append("| " + " | ".join(row) + " |")
    return Block(type="table", text="\n".join(lines), meta={"format": "markdown"})


def _make_list_item(text: str) -> Block:
    return Block(type="list", text=text)


def _long_text(approx_tokens: int) -> str:
    """Generate text with approximately the given number of tokens."""
    # Each "word " is roughly 1 token
    words = ["word"] * approx_tokens
    return " ".join(words)


# ── Token counting ───────────────────────────────────────


class TestCountTokens:
    def test_empty_string(self):
        assert count_tokens("") == 0

    def test_simple_text(self):
        tc = count_tokens("Hello world")
        assert tc >= 2  # At least 2 tokens

    def test_longer_text(self):
        text = "The quick brown fox jumps over the lazy dog."
        tc = count_tokens(text)
        assert 5 < tc < 20  # Reasonable range


# ── Sentence splitting ───────────────────────────────────


class TestSplitSentences:
    def test_basic_split(self):
        result = _split_sentences("Hello world. How are you? Fine!")
        assert len(result) == 3

    def test_no_split(self):
        result = _split_sentences("Hello world")
        assert len(result) == 1

    def test_empty(self):
        result = _split_sentences("")
        assert len(result) == 0


# ── Basic chunking ───────────────────────────────────────


class TestChunkBasic:
    def test_single_short_paragraph(self):
        blocks = [_make_paragraph("This is a short paragraph.")]
        chunks = chunk_blocks(blocks)
        assert len(chunks) == 1
        assert chunks[0].chunk_type == "text"
        assert chunks[0].text == "This is a short paragraph."
        assert chunks[0].ordinal == 0

    def test_multiple_short_paragraphs_merge(self):
        """Short paragraphs should be merged until target is reached."""
        blocks = [_make_paragraph(f"Paragraph {i}.") for i in range(5)]
        chunks = chunk_blocks(blocks, target_tokens=100, max_tokens=200, hard_cap=800)
        # All 5 short paragraphs should fit in one chunk
        assert len(chunks) == 1
        for i in range(5):
            assert f"Paragraph {i}." in chunks[0].text

    def test_ordinals_sequential(self):
        blocks = [
            _make_heading("Section 1"),
            _make_paragraph(_long_text(400)),
            _make_heading("Section 2"),
            _make_paragraph(_long_text(400)),
        ]
        chunks = chunk_blocks(blocks)
        for i, chunk in enumerate(chunks):
            assert chunk.ordinal == i


# ── Heading boundaries ───────────────────────────────────


class TestHeadingBoundaries:
    def test_heading_starts_new_chunk(self):
        blocks = [
            _make_paragraph("First paragraph."),
            _make_heading("New Section"),
            _make_paragraph("Second paragraph."),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        # Heading should force a new chunk
        assert len(chunks) == 2
        assert "First paragraph." in chunks[0].text
        assert "New Section" in chunks[1].text
        assert "Second paragraph." in chunks[1].text

    def test_heading_path_tracking(self):
        blocks = [
            _make_heading("Chapter 1", level=1),
            _make_paragraph("Intro text."),
            _make_heading("Section 1.1", level=2),
            _make_paragraph("Section text."),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        assert chunks[0].heading_path == ["Chapter 1"]
        assert chunks[1].heading_path == ["Chapter 1", "Section 1.1"]

    def test_heading_path_reset_on_same_level(self):
        blocks = [
            _make_heading("Chapter 1", level=1),
            _make_paragraph("Text 1."),
            _make_heading("Chapter 2", level=1),
            _make_paragraph("Text 2."),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        assert chunks[0].heading_path == ["Chapter 1"]
        assert chunks[1].heading_path == ["Chapter 2"]


# ── Token limits ─────────────────────────────────────────


class TestTokenLimits:
    def test_chunk_respects_max_tokens(self):
        """No text chunk should exceed hard_cap tokens."""
        blocks = [_make_paragraph(_long_text(600)) for _ in range(5)]
        chunks = chunk_blocks(blocks, target_tokens=450, max_tokens=500, hard_cap=800)
        for chunk in chunks:
            assert chunk.token_count <= 800 + 10  # Small tolerance for tokenizer variance

    def test_large_paragraph_split_at_sentences(self):
        """A paragraph exceeding max_tokens should be split at sentence boundaries."""
        # Create a paragraph with many sentences
        sentences = [f"This is sentence number {i} with some extra words." for i in range(50)]
        text = " ".join(sentences)
        blocks = [_make_paragraph(text)]
        chunks = chunk_blocks(blocks, target_tokens=100, max_tokens=150, hard_cap=800)
        assert len(chunks) > 1
        # All chunks should be within hard cap
        for chunk in chunks:
            assert chunk.token_count <= 800 + 10


# ── Table chunking ───────────────────────────────────────


class TestTableChunking:
    def test_small_table_single_chunk(self):
        rows = [
            ["Name", "Age", "City"],
            ["Alice", "30", "NYC"],
            ["Bob", "25", "LA"],
        ]
        blocks = [_make_table(rows)]
        chunks = chunk_blocks(blocks)
        assert len(chunks) == 1
        assert chunks[0].chunk_type == "table"
        assert "Alice" in chunks[0].text
        assert "Bob" in chunks[0].text

    def test_table_preserves_header(self):
        """When a table is split, each chunk should have the header row."""
        # Create a large table that must be split
        header = ["Col1", "Col2", "Col3"]
        data_rows = [[f"data_{i}_1", f"data_{i}_2 " + "x" * 100, f"data_{i}_3"] for i in range(50)]
        rows = [header] + data_rows
        blocks = [_make_table(rows)]
        chunks = chunk_blocks(blocks, hard_cap=200)
        assert len(chunks) > 1
        for chunk in chunks:
            assert chunk.chunk_type == "table"
            # Each chunk should contain the header
            assert "Col1" in chunk.text
            assert "Col2" in chunk.text

    def test_table_not_split_arbitrarily(self):
        """Table chunks should contain complete rows, not partial rows."""
        header = ["Name", "Value"]
        data_rows = [[f"item_{i}", f"value_{i}"] for i in range(20)]
        rows = [header] + data_rows
        blocks = [_make_table(rows)]
        chunks = chunk_blocks(blocks, hard_cap=100)
        for chunk in chunks:
            lines = chunk.text.strip().split("\n")
            # Each line should be a complete table row (starts and ends with |)
            for line in lines:
                assert line.startswith("|") and line.endswith("|")

    def test_table_between_text(self):
        """Tables should appear in correct order between text chunks."""
        blocks = [
            _make_paragraph("Before table."),
            _make_table([["A", "B"], ["1", "2"]]),
            _make_paragraph("After table."),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        assert len(chunks) == 3
        assert chunks[0].chunk_type == "text"
        assert chunks[1].chunk_type == "table"
        assert chunks[2].chunk_type == "text"

    def test_table_inherits_heading_path(self):
        blocks = [
            _make_heading("Data Section", level=1),
            _make_paragraph("Some intro."),
            _make_table([["X", "Y"], ["1", "2"]]),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        table_chunk = [c for c in chunks if c.chunk_type == "table"][0]
        assert table_chunk.heading_path == ["Data Section"]


# ── Edge cases ───────────────────────────────────────────


class TestEdgeCases:
    def test_empty_blocks(self):
        chunks = chunk_blocks([])
        assert chunks == []

    def test_only_headings(self):
        blocks = [
            _make_heading("H1", level=1),
            _make_heading("H2", level=2),
        ]
        chunks = chunk_blocks(blocks)
        # Headings with no content should still produce chunks
        assert len(chunks) >= 1

    def test_list_items_chunked_like_paragraphs(self):
        blocks = [
            _make_list_item("Item 1"),
            _make_list_item("Item 2"),
            _make_list_item("Item 3"),
        ]
        chunks = chunk_blocks(blocks, target_tokens=1000, max_tokens=2000, hard_cap=3000)
        assert len(chunks) == 1
        assert "Item 1" in chunks[0].text
        assert "Item 3" in chunks[0].text
