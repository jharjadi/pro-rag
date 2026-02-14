"""PDF extractor — produces structured blocks from .pdf files.

Uses pdfplumber for table extraction and pymupdf (fitz) for text.
Tables are preserved as markdown (never shredded).

Blocks: {type: heading|paragraph|table, text: str, meta: dict}
"""

from __future__ import annotations

import logging
import re
from pathlib import Path

import fitz  # pymupdf
import pdfplumber

from ingest.extract import Block

logger = logging.getLogger(__name__)

# Heuristic: text larger than this (in points) is treated as a heading
_HEADING_FONT_SIZE_THRESHOLD = 14.0
# Minimum font size for level-1 heading
_H1_FONT_SIZE_THRESHOLD = 18.0


def _table_to_markdown(table_data: list[list[str | None]]) -> str:
    """Convert pdfplumber table data to a markdown table string.

    Args:
        table_data: List of rows, each row is a list of cell strings (or None).

    Returns:
        Markdown-formatted table string.
    """
    if not table_data or not table_data[0]:
        return ""

    # Clean cells: replace None with empty string, strip whitespace, collapse newlines
    rows: list[list[str]] = []
    for row in table_data:
        cleaned = [
            (cell or "").strip().replace("\n", " ")
            for cell in row
        ]
        rows.append(cleaned)

    if not rows:
        return ""

    num_cols = len(rows[0])
    lines: list[str] = []

    # Header row
    lines.append("| " + " | ".join(rows[0]) + " |")
    # Separator
    lines.append("| " + " | ".join("---" for _ in range(num_cols)) + " |")
    # Data rows
    for row in rows[1:]:
        # Pad row to match header length if needed
        while len(row) < num_cols:
            row.append("")
        lines.append("| " + " | ".join(row[:num_cols]) + " |")

    return "\n".join(lines)


def _get_page_tables_bboxes(page: pdfplumber.page.Page) -> list[tuple]:
    """Get bounding boxes of all tables on a pdfplumber page.

    Returns:
        List of (x0, top, x1, bottom) bounding boxes.
    """
    tables = page.find_tables()
    return [t.bbox for t in tables]


def _is_inside_table(char_top: float, char_bottom: float, table_bboxes: list[tuple]) -> bool:
    """Check if a text span overlaps with any table bounding box."""
    for (x0, top, x1, bottom) in table_bboxes:
        if char_top < bottom and char_bottom > top:
            return True
    return False


def _extract_text_blocks_from_page(
    fitz_page: fitz.Page,
    table_bboxes_plumber: list[tuple],
    page_height: float,
) -> list[Block]:
    """Extract text blocks from a pymupdf page, excluding table regions.

    Uses pymupdf's text extraction with font info to detect headings.
    Converts pdfplumber bbox coordinates to pymupdf coordinates (same coordinate system).

    Args:
        fitz_page: pymupdf Page object.
        table_bboxes_plumber: Table bounding boxes from pdfplumber (x0, top, x1, bottom).
        page_height: Page height for coordinate conversion if needed.

    Returns:
        List of Block objects.
    """
    blocks: list[Block] = []

    # Extract text as dict with font info
    text_dict = fitz_page.get_text("dict", flags=fitz.TEXT_PRESERVE_WHITESPACE)

    for block in text_dict.get("blocks", []):
        if block.get("type") != 0:  # 0 = text block
            continue

        block_bbox = block.get("bbox", (0, 0, 0, 0))
        block_top = block_bbox[1]
        block_bottom = block_bbox[3]

        # Skip text inside table regions
        if _is_inside_table(block_top, block_bottom, table_bboxes_plumber):
            continue

        # Collect spans from all lines in this block
        full_text_parts: list[str] = []
        max_font_size = 0.0
        is_bold = False

        for line in block.get("lines", []):
            line_text_parts: list[str] = []
            for span in line.get("spans", []):
                text = span.get("text", "")
                if text.strip():
                    line_text_parts.append(text)
                    font_size = span.get("size", 0.0)
                    if font_size > max_font_size:
                        max_font_size = font_size
                    font_name = span.get("font", "").lower()
                    if "bold" in font_name:
                        is_bold = True

            if line_text_parts:
                full_text_parts.append("".join(line_text_parts))

        full_text = "\n".join(full_text_parts).strip()
        if not full_text:
            continue

        # Classify block type based on font characteristics
        if max_font_size >= _H1_FONT_SIZE_THRESHOLD:
            blocks.append(Block(
                type="heading",
                text=full_text,
                meta={"level": 1, "font_size": round(max_font_size, 1)},
            ))
        elif max_font_size >= _HEADING_FONT_SIZE_THRESHOLD or (is_bold and len(full_text) < 200):
            level = 2 if max_font_size >= 16.0 else 3
            blocks.append(Block(
                type="heading",
                text=full_text,
                meta={"level": level, "font_size": round(max_font_size, 1)},
            ))
        else:
            blocks.append(Block(
                type="paragraph",
                text=full_text,
                meta={},
            ))

    return blocks


def extract_pdf(file_path: str | Path) -> list[Block]:
    """Extract structured blocks from a PDF file.

    Strategy:
    - Use pdfplumber to detect and extract tables (preserves structure).
    - Use pymupdf (fitz) to extract text with font info (for heading detection).
    - Exclude text that falls within table bounding boxes to avoid duplication.

    Args:
        file_path: Path to the .pdf file.

    Returns:
        List of Block objects in document order.
    """
    file_path = Path(file_path)
    if not file_path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")
    if file_path.suffix.lower() != ".pdf":
        raise ValueError(f"Expected .pdf file, got: {file_path.suffix}")

    blocks: list[Block] = []

    # Open with both libraries
    fitz_doc = fitz.open(str(file_path))
    plumber_doc = pdfplumber.open(str(file_path))

    try:
        num_pages = len(fitz_doc)
        if num_pages != len(plumber_doc.pages):
            logger.warning(
                "Page count mismatch: pymupdf=%d, pdfplumber=%d. Using pymupdf count.",
                num_pages,
                len(plumber_doc.pages),
            )
            num_pages = min(num_pages, len(plumber_doc.pages))

        for page_idx in range(num_pages):
            fitz_page = fitz_doc[page_idx]
            plumber_page = plumber_doc.pages[page_idx]
            page_height = float(plumber_page.height)

            # Step 1: Find tables with pdfplumber
            table_bboxes = _get_page_tables_bboxes(plumber_page)
            tables = plumber_page.extract_tables()

            # Step 2: Extract text blocks (excluding table regions)
            text_blocks = _extract_text_blocks_from_page(
                fitz_page, table_bboxes, page_height
            )
            blocks.extend(text_blocks)

            # Step 3: Add table blocks
            for i, table_data in enumerate(tables):
                if not table_data:
                    continue
                md = _table_to_markdown(table_data)
                if md.strip():
                    num_rows = len(table_data)
                    num_cols = len(table_data[0]) if table_data[0] else 0
                    blocks.append(Block(
                        type="table",
                        text=md,
                        meta={
                            "format": "markdown",
                            "num_rows": num_rows,
                            "num_cols": num_cols,
                            "page": page_idx + 1,
                        },
                    ))

    finally:
        fitz_doc.close()
        plumber_doc.close()

    logger.info(
        "Extracted %d blocks from %s (headings=%d, paragraphs=%d, tables=%d)",
        len(blocks),
        file_path.name,
        sum(1 for b in blocks if b.type == "heading"),
        sum(1 for b in blocks if b.type == "paragraph"),
        sum(1 for b in blocks if b.type == "table"),
    )

    return blocks
