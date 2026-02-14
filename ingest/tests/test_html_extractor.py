"""Tests for HTML extractor.

Creates real HTML files in a temp directory to test extraction.
"""

from __future__ import annotations

import tempfile
from pathlib import Path

import pytest

from ingest.extract import Block
from ingest.extract.html import extract_html


@pytest.fixture
def tmp_dir():
    """Create a temporary directory for test files."""
    with tempfile.TemporaryDirectory() as d:
        yield Path(d)


def _write_html(path: Path, name: str, content: str) -> Path:
    """Write HTML content to a file."""
    filepath = path / name
    filepath.write_text(content, encoding="utf-8")
    return filepath


# ── Test HTML documents ──────────────────────────────────

SIMPLE_HTML = """\
<!DOCTYPE html>
<html>
<head><title>Test Document</title></head>
<body>
    <h1>Chapter 1</h1>
    <p>This is the first paragraph of chapter 1.</p>
    <p>This is the second paragraph.</p>
    <h2>Section 1.1</h2>
    <p>Content under section 1.1.</p>
    <h1>Chapter 2</h1>
    <p>Content of chapter 2.</p>
</body>
</html>
"""

TABLE_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <h1>Data Report</h1>
    <p>Below is the data table:</p>
    <table>
        <thead>
            <tr><th>Name</th><th>Age</th><th>City</th></tr>
        </thead>
        <tbody>
            <tr><td>Alice</td><td>30</td><td>New York</td></tr>
            <tr><td>Bob</td><td>25</td><td>Los Angeles</td></tr>
            <tr><td>Charlie</td><td>35</td><td>Chicago</td></tr>
        </tbody>
    </table>
    <p>End of report.</p>
</body>
</html>
"""

LIST_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <h1>Shopping List</h1>
    <ul>
        <li>Apples</li>
        <li>Bananas</li>
        <li>Oranges</li>
    </ul>
    <p>That's all.</p>
</body>
</html>
"""

CODE_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <h1>Code Example</h1>
    <p>Here is some code:</p>
    <pre><code class="language-python">def hello():
    print("Hello, world!")</code></pre>
    <p>End of example.</p>
</body>
</html>
"""

NESTED_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <div class="content">
        <section>
            <h2>Nested Section</h2>
            <div>
                <p>Deeply nested paragraph.</p>
            </div>
        </section>
    </div>
</body>
</html>
"""

SKIP_TAGS_HTML = """\
<!DOCTYPE html>
<html>
<head>
    <title>Test</title>
    <style>body { color: red; }</style>
    <script>console.log("skip me");</script>
</head>
<body>
    <nav><a href="/">Home</a></nav>
    <h1>Main Content</h1>
    <p>This should be extracted.</p>
    <footer>Footer content</footer>
    <aside>Sidebar content</aside>
</body>
</html>
"""

TABLE_NO_THEAD_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <table>
        <tr><td>Name</td><td>Value</td></tr>
        <tr><td>Alpha</td><td>100</td></tr>
        <tr><td>Beta</td><td>200</td></tr>
    </table>
</body>
</html>
"""

EMPTY_HTML = """\
<!DOCTYPE html>
<html>
<head><title>Empty</title></head>
<body></body>
</html>
"""

ORDERED_LIST_HTML = """\
<!DOCTYPE html>
<html>
<body>
    <h1>Steps</h1>
    <ol>
        <li>First step</li>
        <li>Second step</li>
        <li>Third step</li>
    </ol>
</body>
</html>
"""


# ── Tests ────────────────────────────────────────────────


class TestHtmlExtractor:
    def test_simple_document(self, tmp_dir):
        filepath = _write_html(tmp_dir, "simple.html", SIMPLE_HTML)
        blocks = extract_html(filepath)

        assert len(blocks) > 0

        # Check headings
        headings = [b for b in blocks if b.type == "heading"]
        assert len(headings) == 3  # Chapter 1, Section 1.1, Chapter 2

        assert headings[0].text == "Chapter 1"
        assert headings[0].meta["level"] == 1
        assert headings[1].text == "Section 1.1"
        assert headings[1].meta["level"] == 2
        assert headings[2].text == "Chapter 2"
        assert headings[2].meta["level"] == 1

        # Check paragraphs
        paragraphs = [b for b in blocks if b.type == "paragraph"]
        assert len(paragraphs) >= 3

    def test_table_extraction(self, tmp_dir):
        filepath = _write_html(tmp_dir, "table.html", TABLE_HTML)
        blocks = extract_html(filepath)

        tables = [b for b in blocks if b.type == "table"]
        assert len(tables) == 1

        table = tables[0]
        assert "Name" in table.text
        assert "Alice" in table.text
        assert "Bob" in table.text
        assert table.meta["format"] == "markdown"
        assert table.meta["num_rows"] == 4  # 1 header + 3 data
        assert table.meta["num_cols"] == 3

    def test_table_is_markdown_format(self, tmp_dir):
        filepath = _write_html(tmp_dir, "table.html", TABLE_HTML)
        blocks = extract_html(filepath)
        table = [b for b in blocks if b.type == "table"][0]

        lines = table.text.split("\n")
        assert len(lines) >= 4  # header + separator + 3 data rows
        assert lines[0].startswith("|")
        assert "---" in lines[1]

    def test_table_without_thead(self, tmp_dir):
        filepath = _write_html(tmp_dir, "table_no_thead.html", TABLE_NO_THEAD_HTML)
        blocks = extract_html(filepath)

        tables = [b for b in blocks if b.type == "table"]
        assert len(tables) == 1
        assert "Name" in tables[0].text
        assert "Alpha" in tables[0].text

    def test_list_extraction(self, tmp_dir):
        filepath = _write_html(tmp_dir, "list.html", LIST_HTML)
        blocks = extract_html(filepath)

        lists = [b for b in blocks if b.type == "list"]
        assert len(lists) == 3
        assert lists[0].text == "Apples"
        assert lists[1].text == "Bananas"
        assert lists[2].text == "Oranges"

    def test_ordered_list(self, tmp_dir):
        filepath = _write_html(tmp_dir, "ordered.html", ORDERED_LIST_HTML)
        blocks = extract_html(filepath)

        lists = [b for b in blocks if b.type == "list"]
        assert len(lists) == 3
        assert lists[0].text == "First step"

    def test_code_block(self, tmp_dir):
        filepath = _write_html(tmp_dir, "code.html", CODE_HTML)
        blocks = extract_html(filepath)

        code_blocks = [b for b in blocks if b.type == "code"]
        assert len(code_blocks) == 1
        assert "def hello():" in code_blocks[0].text
        assert code_blocks[0].meta.get("language") == "python"

    def test_nested_content(self, tmp_dir):
        filepath = _write_html(tmp_dir, "nested.html", NESTED_HTML)
        blocks = extract_html(filepath)

        headings = [b for b in blocks if b.type == "heading"]
        assert len(headings) == 1
        assert headings[0].text == "Nested Section"

        paragraphs = [b for b in blocks if b.type == "paragraph"]
        assert len(paragraphs) == 1
        assert paragraphs[0].text == "Deeply nested paragraph."

    def test_skip_non_content_tags(self, tmp_dir):
        filepath = _write_html(tmp_dir, "skip.html", SKIP_TAGS_HTML)
        blocks = extract_html(filepath)

        all_text = " ".join(b.text for b in blocks)
        # Script, style, nav, footer, aside content should not appear
        assert "console.log" not in all_text
        assert "color: red" not in all_text
        assert "Footer content" not in all_text
        assert "Sidebar content" not in all_text

        # Main content should be present
        assert any("Main Content" in b.text for b in blocks)
        assert any("This should be extracted" in b.text for b in blocks)

    def test_empty_document(self, tmp_dir):
        filepath = _write_html(tmp_dir, "empty.html", EMPTY_HTML)
        blocks = extract_html(filepath)
        assert blocks == []

    def test_file_not_found(self):
        with pytest.raises(FileNotFoundError):
            extract_html("/nonexistent/file.html")

    def test_wrong_extension(self, tmp_dir):
        filepath = tmp_dir / "test.txt"
        filepath.write_text("hello")
        with pytest.raises(ValueError, match="Expected .html"):
            extract_html(filepath)

    def test_htm_extension(self, tmp_dir):
        filepath = _write_html(tmp_dir, "test.htm", SIMPLE_HTML)
        blocks = extract_html(filepath)
        assert len(blocks) > 0

    def test_document_order_preserved(self, tmp_dir):
        filepath = _write_html(tmp_dir, "simple.html", SIMPLE_HTML)
        blocks = extract_html(filepath)

        types = [b.type for b in blocks]
        # First block should be heading
        assert types[0] == "heading"

    def test_blocks_have_correct_types(self, tmp_dir):
        filepath = _write_html(tmp_dir, "simple.html", SIMPLE_HTML)
        blocks = extract_html(filepath)

        for block in blocks:
            assert isinstance(block, Block)
            assert block.type in ("heading", "paragraph", "table", "list", "code")
            assert isinstance(block.text, str)
            assert len(block.text) > 0
            assert isinstance(block.meta, dict)
