"""Structure-aware chunker for pro-rag ingestion.

Rules (from spec v7 §5):
- Non-table: target 350-500 tokens (default 450), hard cap 800
- Split boundaries: heading → paragraph → sentence
- Tables: never split arbitrarily; split by row groups ≤800 tokens, repeat header row
- Single row >800 tokens: keep as one chunk + log warning
- Overlap: 0 in V1
- Token counting via tiktoken
"""

from __future__ import annotations

import logging
import re
from dataclasses import dataclass, field
from typing import Literal

import tiktoken

from ingest.extract import Block

logger = logging.getLogger(__name__)

# Default chunking parameters (overridable via config)
DEFAULT_TARGET_TOKENS = 450
DEFAULT_MIN_TOKENS = 350
DEFAULT_MAX_TOKENS = 500
DEFAULT_HARD_CAP_TOKENS = 800


@dataclass
class Chunk:
    """A chunk ready for embedding and storage."""

    text: str
    chunk_type: Literal["text", "table"]
    token_count: int
    heading_path: list[str] = field(default_factory=list)
    ordinal: int = 0  # Set by caller
    meta: dict = field(default_factory=dict)


def _get_encoder() -> tiktoken.Encoding:
    """Get tiktoken encoder (cl100k_base, used by most modern models)."""
    return tiktoken.get_encoding("cl100k_base")


def count_tokens(text: str, encoder: tiktoken.Encoding | None = None) -> int:
    """Count tokens in text using tiktoken."""
    if encoder is None:
        encoder = _get_encoder()
    return len(encoder.encode(text))


def _split_sentences(text: str) -> list[str]:
    """Split text into sentences. Simple regex-based splitter."""
    # Split on sentence-ending punctuation followed by whitespace
    parts = re.split(r"(?<=[.!?])\s+", text)
    return [p.strip() for p in parts if p.strip()]


def _chunk_text_blocks(
    blocks: list[Block],
    target_tokens: int,
    max_tokens: int,
    hard_cap: int,
    encoder: tiktoken.Encoding,
) -> list[Chunk]:
    """Chunk non-table blocks respecting heading boundaries.

    Strategy:
    1. Headings always start a new chunk (heading boundary).
    2. Accumulate paragraphs/lists until target reached.
    3. If a single paragraph exceeds max_tokens, split at sentence boundaries.
    4. Hard cap: if a single sentence exceeds hard_cap, keep it (log warning).
    """
    chunks: list[Chunk] = []
    current_text_parts: list[str] = []
    current_tokens = 0
    heading_path: list[str] = []

    def _flush():
        nonlocal current_text_parts, current_tokens
        if current_text_parts:
            text = "\n\n".join(current_text_parts)
            tc = count_tokens(text, encoder)
            chunks.append(Chunk(
                text=text,
                chunk_type="text",
                token_count=tc,
                heading_path=list(heading_path),
            ))
            current_text_parts = []
            current_tokens = 0

    for block in blocks:
        if block.type == "table":
            # Tables handled separately
            continue

        if block.type == "heading":
            # Flush current accumulation
            _flush()
            # Update heading path
            level = block.meta.get("level", 1)
            # Trim heading_path to parent level, then append
            heading_path = heading_path[: level - 1]
            heading_path.append(block.text)
            # Include heading text in next chunk
            current_text_parts.append(block.text)
            current_tokens += count_tokens(block.text, encoder)
            continue

        # paragraph or list block
        block_tokens = count_tokens(block.text, encoder)

        # If adding this block would exceed max_tokens, flush first
        if current_tokens + block_tokens > max_tokens and current_text_parts:
            _flush()

        # If a single block exceeds max_tokens, split at sentence level
        if block_tokens > max_tokens:
            _flush()  # Ensure clean state
            sentences = _split_sentences(block.text)
            sent_parts: list[str] = []
            sent_tokens = 0

            for sent in sentences:
                st = count_tokens(sent, encoder)
                if sent_tokens + st > max_tokens and sent_parts:
                    text = " ".join(sent_parts)
                    tc = count_tokens(text, encoder)
                    chunks.append(Chunk(
                        text=text,
                        chunk_type="text",
                        token_count=tc,
                        heading_path=list(heading_path),
                    ))
                    sent_parts = []
                    sent_tokens = 0

                if st > hard_cap:
                    logger.warning(
                        "Single sentence exceeds hard cap (%d tokens > %d): %.80s...",
                        st, hard_cap, sent,
                    )
                sent_parts.append(sent)
                sent_tokens += st

            if sent_parts:
                text = " ".join(sent_parts)
                tc = count_tokens(text, encoder)
                chunks.append(Chunk(
                    text=text,
                    chunk_type="text",
                    token_count=tc,
                    heading_path=list(heading_path),
                ))
            continue

        current_text_parts.append(block.text)
        current_tokens += block_tokens

        # If we've reached target, flush
        if current_tokens >= target_tokens:
            _flush()

    # Flush remaining
    _flush()

    return chunks


def _chunk_table(
    block: Block,
    hard_cap: int,
    encoder: tiktoken.Encoding,
    heading_path: list[str],
) -> list[Chunk]:
    """Chunk a table block.

    Rules:
    - Never split arbitrarily
    - Split by row groups ≤ hard_cap tokens, repeat header row
    - Single row > hard_cap: keep as one chunk + log warning
    """
    lines = block.text.split("\n")
    if len(lines) < 3:
        # Too small to split — just one chunk
        tc = count_tokens(block.text, encoder)
        return [Chunk(
            text=block.text,
            chunk_type="table",
            token_count=tc,
            heading_path=list(heading_path),
            meta=dict(block.meta),
        )]

    # Parse: header (line 0), separator (line 1), data rows (line 2+)
    header_line = lines[0]
    separator_line = lines[1]
    data_rows = lines[2:]

    header_text = header_line + "\n" + separator_line
    header_tokens = count_tokens(header_text, encoder)

    # Check if entire table fits in hard_cap
    total_tokens = count_tokens(block.text, encoder)
    if total_tokens <= hard_cap:
        return [Chunk(
            text=block.text,
            chunk_type="table",
            token_count=total_tokens,
            heading_path=list(heading_path),
            meta=dict(block.meta),
        )]

    # Split by row groups
    chunks: list[Chunk] = []
    current_rows: list[str] = []
    current_tokens = header_tokens  # Always include header

    for row in data_rows:
        row_tokens = count_tokens(row, encoder)

        # Single row exceeds hard_cap (including header)
        if header_tokens + row_tokens > hard_cap:
            # Flush current group first
            if current_rows:
                text = header_text + "\n" + "\n".join(current_rows)
                tc = count_tokens(text, encoder)
                chunks.append(Chunk(
                    text=text,
                    chunk_type="table",
                    token_count=tc,
                    heading_path=list(heading_path),
                    meta=dict(block.meta),
                ))
                current_rows = []
                current_tokens = header_tokens

            # Keep oversized row as its own chunk
            logger.warning(
                "Single table row exceeds hard cap (%d + %d = %d tokens > %d)",
                header_tokens, row_tokens, header_tokens + row_tokens, hard_cap,
            )
            text = header_text + "\n" + row
            tc = count_tokens(text, encoder)
            chunks.append(Chunk(
                text=text,
                chunk_type="table",
                token_count=tc,
                heading_path=list(heading_path),
                meta=dict(block.meta),
            ))
            continue

        # Would adding this row exceed hard_cap?
        if current_tokens + row_tokens > hard_cap and current_rows:
            text = header_text + "\n" + "\n".join(current_rows)
            tc = count_tokens(text, encoder)
            chunks.append(Chunk(
                text=text,
                chunk_type="table",
                token_count=tc,
                heading_path=list(heading_path),
                meta=dict(block.meta),
            ))
            current_rows = []
            current_tokens = header_tokens

        current_rows.append(row)
        current_tokens += row_tokens

    # Flush remaining rows
    if current_rows:
        text = header_text + "\n" + "\n".join(current_rows)
        tc = count_tokens(text, encoder)
        chunks.append(Chunk(
            text=text,
            chunk_type="table",
            token_count=tc,
            heading_path=list(heading_path),
            meta=dict(block.meta),
        ))

    return chunks


def chunk_blocks(
    blocks: list[Block],
    target_tokens: int = DEFAULT_TARGET_TOKENS,
    min_tokens: int = DEFAULT_MIN_TOKENS,
    max_tokens: int = DEFAULT_MAX_TOKENS,
    hard_cap: int = DEFAULT_HARD_CAP_TOKENS,
) -> list[Chunk]:
    """Chunk a list of extracted blocks into chunks for embedding.

    Args:
        blocks: Extracted document blocks.
        target_tokens: Target chunk size in tokens (default 450).
        min_tokens: Minimum chunk size (default 350).
        max_tokens: Soft max chunk size (default 500).
        hard_cap: Hard maximum chunk size (default 800).

    Returns:
        List of Chunk objects with ordinals set.
    """
    encoder = _get_encoder()

    # Separate text blocks and table blocks, preserving order
    # We need to interleave text chunks and table chunks in document order
    chunks: list[Chunk] = []

    # Track heading path for tables
    heading_path: list[str] = []

    # Group consecutive non-table blocks, process tables inline
    text_group: list[Block] = []

    def _flush_text_group():
        nonlocal text_group
        if text_group:
            text_chunks = _chunk_text_blocks(
                text_group, target_tokens, max_tokens, hard_cap, encoder
            )
            chunks.extend(text_chunks)
            text_group = []

    for block in blocks:
        if block.type == "heading":
            level = block.meta.get("level", 1)
            heading_path = heading_path[: level - 1]
            heading_path.append(block.text)
            text_group.append(block)

        elif block.type == "table":
            # Flush any accumulated text blocks first
            _flush_text_group()
            # Chunk the table
            table_chunks = _chunk_table(block, hard_cap, encoder, heading_path)
            chunks.extend(table_chunks)

        else:
            text_group.append(block)

    # Flush remaining text blocks
    _flush_text_group()

    # Set ordinals
    for i, chunk in enumerate(chunks):
        chunk.ordinal = i

    logger.info(
        "Chunked %d blocks into %d chunks (text=%d, table=%d)",
        len(blocks),
        len(chunks),
        sum(1 for c in chunks if c.chunk_type == "text"),
        sum(1 for c in chunks if c.chunk_type == "table"),
    )

    return chunks
