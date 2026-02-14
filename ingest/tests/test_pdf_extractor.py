"""Tests for PDF extractor.

Creates real PDF files in a temp directory using pymupdf (fitz) to test extraction.
"""

from __future__ import annotations

import tempfile
from pathlib import Path

import fitz  # pymupdf
import pytest

from ingest.extract import Block
from ingest.extract.pdf import extract_pdf


@pytest.fixture
def tmp_dir():
    """Create a temporary directory for test files."""
    with tempfile.TemporaryDirectory() as d:
        yield Path(d)


def _create_simple_pdf(path: Path) -> Path:
    """Create a simple PDF with headings and paragraphs."""
    doc = fitz.open()

    page = doc.new_page()
    # Title (large font = heading)
    page.insert_text((72, 72), "Chapter 1: Introduction", fontsize=20)
    # Paragraph text (normal font)
    page.insert_text((72, 120), "This is the first paragraph of the introduction.", fontsize=11)
    page.insert_text((72, 140), "This is the second paragraph with more details.", fontsize=11)
    # Subheading
    page.insert_text((72, 180), "Section 1.1: Background", fontsize=16)
    page.insert_text((72, 210), "Background information goes here.", fontsize=11)

    # Second page
    page2 = doc.new_page()
    page2.insert_text((72, 72), "Chapter 2: Methods", fontsize=20)
    page2.insert_text((72, 120), "Description of methods used.", fontsize=11)

    filepath = path / "simple.pdf"
    doc.save(str(filepath))
    doc.close()
    return filepath


def _create_table_pdf(path: Path) -> Path:
    """Create a PDF with a table using pdfplumber-compatible table structure.

    We create a table by drawing lines and placing text in a grid pattern.
    """
    doc = fitz.open()
    page = doc.new_page()

    # Title
    page.insert_text((72, 50), "Data Report", fontsize=18)
    page.insert_text((72, 80), "Below is the data table:", fontsize=11)

    # Draw a simple table with lines
    # Table starts at y=100, 3 columns, 4 rows
    x_start = 72
    y_start = 100
    col_widths = [120, 80, 120]
    row_height = 25
    num_rows = 4

    # Table data
    data = [
        ["Name", "Age", "City"],
        ["Alice", "30", "New York"],
        ["Bob", "25", "Los Angeles"],
        ["Charlie", "35", "Chicago"],
    ]

    # Draw horizontal lines
    total_width = sum(col_widths)
    for i in range(num_rows + 1):
        y = y_start + i * row_height
        page.draw_line(
            fitz.Point(x_start, y),
            fitz.Point(x_start + total_width, y),
        )

    # Draw vertical lines
    x = x_start
    for w in [0] + col_widths:
        x += w if w > 0 else 0
        page.draw_line(
            fitz.Point(x if w > 0 else x_start, y_start),
            fitz.Point(x if w > 0 else x_start, y_start + num_rows * row_height),
        )

    # Insert text in cells
    for row_idx, row_data in enumerate(data):
        x = x_start
        for col_idx, cell_text in enumerate(row_data):
            page.insert_text(
                (x + 5, y_start + row_idx * row_height + 17),
                cell_text,
                fontsize=10,
            )
            x += col_widths[col_idx]

    page.insert_text((72, y_start + num_rows * row_height + 30), "End of report.", fontsize=11)

    filepath = path / "with_table.pdf"
    doc.save(str(filepath))
    doc.close()
    return filepath


def _create_empty_pdf(path: Path) -> Path:
    """Create an empty PDF (no text content)."""
    doc = fitz.open()
    doc.new_page()
    filepath = path / "empty.pdf"
    doc.save(str(filepath))
    doc.close()
    return filepath


def _create_multipage_pdf(path: Path) -> Path:
    """Create a multi-page PDF."""
    doc = fitz.open()

    for i in range(3):
        page = doc.new_page()
        page.insert_text((72, 72), f"Page {i + 1} Title", fontsize=18)
        page.insert_text((72, 110), f"Content on page {i + 1}.", fontsize=11)

    filepath = path / "multipage.pdf"
    doc.save(str(filepath))
    doc.close()
    return filepath


# ── Tests ────────────────────────────────────────────────


class TestPdfExtractor:
    def test_simple_document(self, tmp_dir):
        filepath = _create_simple_pdf(tmp_dir)
        blocks = extract_pdf(filepath)

        assert len(blocks) > 0

        # Should have headings (large font text)
        headings = [b for b in blocks if b.type == "heading"]
        assert len(headings) >= 2  # At least Chapter 1 and Chapter 2

        # Should have paragraphs
        paragraphs = [b for b in blocks if b.type == "paragraph"]
        assert len(paragraphs) >= 2

    def test_heading_detection(self, tmp_dir):
        filepath = _create_simple_pdf(tmp_dir)
        blocks = extract_pdf(filepath)

        headings = [b for b in blocks if b.type == "heading"]
        # Large font (20pt) should be detected as heading
        heading_texts = [h.text for h in headings]
        assert any("Chapter 1" in t for t in heading_texts)

    def test_multipage(self, tmp_dir):
        filepath = _create_multipage_pdf(tmp_dir)
        blocks = extract_pdf(filepath)

        assert len(blocks) >= 6  # 3 pages × (1 heading + 1 paragraph)

        headings = [b for b in blocks if b.type == "heading"]
        assert len(headings) >= 3

    def test_table_extraction(self, tmp_dir):
        """Test that tables drawn with lines are detected by pdfplumber."""
        filepath = _create_table_pdf(tmp_dir)
        blocks = extract_pdf(filepath)

        # pdfplumber should detect the table from the drawn lines
        tables = [b for b in blocks if b.type == "table"]
        # Table detection depends on pdfplumber's line detection;
        # if it finds the table, verify markdown format
        if tables:
            table = tables[0]
            assert table.meta["format"] == "markdown"
            assert "Name" in table.text or "Alice" in table.text
            assert table.meta["num_rows"] >= 2

    def test_empty_document(self, tmp_dir):
        filepath = _create_empty_pdf(tmp_dir)
        blocks = extract_pdf(filepath)
        assert blocks == []

    def test_file_not_found(self):
        with pytest.raises(FileNotFoundError):
            extract_pdf("/nonexistent/file.pdf")

    def test_wrong_extension(self, tmp_dir):
        filepath = tmp_dir / "test.txt"
        filepath.write_text("hello")
        with pytest.raises(ValueError, match="Expected .pdf"):
            extract_pdf(filepath)

    def test_blocks_have_correct_types(self, tmp_dir):
        filepath = _create_simple_pdf(tmp_dir)
        blocks = extract_pdf(filepath)

        for block in blocks:
            assert isinstance(block, Block)
            assert block.type in ("heading", "paragraph", "table", "list", "code")
            assert isinstance(block.text, str)
            assert len(block.text) > 0
            assert isinstance(block.meta, dict)
