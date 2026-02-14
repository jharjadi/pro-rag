"""HTML extractor — produces structured blocks from .html/.htm files.

Uses BeautifulSoup4 to parse HTML and extract structured blocks.
Tables are preserved as markdown (never shredded).

Blocks: {type: heading|paragraph|table|list|code, text: str, meta: dict}
"""

from __future__ import annotations

import logging
import re
from pathlib import Path

from bs4 import BeautifulSoup, NavigableString, Tag

from ingest.extract import Block

logger = logging.getLogger(__name__)

# HTML heading tags → level mapping
_HEADING_TAGS = {"h1": 1, "h2": 2, "h3": 3, "h4": 4, "h5": 5, "h6": 6}

# Tags that contain block-level content we want to extract
_BLOCK_TAGS = {"p", "div", "blockquote", "article", "section"}

# Tags to skip entirely (navigation, scripts, styles, etc.)
_SKIP_TAGS = {"script", "style", "nav", "footer", "header", "aside", "noscript", "meta", "link"}


def _table_to_markdown(table: Tag) -> tuple[str, int, int]:
    """Convert an HTML <table> to a markdown table string.

    Args:
        table: BeautifulSoup Tag for a <table> element.

    Returns:
        Tuple of (markdown_string, num_rows, num_cols).
    """
    rows: list[list[str]] = []

    # Extract header rows from <thead>
    thead = table.find("thead")
    if thead:
        for tr in thead.find_all("tr"):
            cells = [
                (td.get_text(separator=" ", strip=True) or "").replace("\n", " ")
                for td in tr.find_all(["th", "td"])
            ]
            if cells:
                rows.append(cells)

    # Extract body rows from <tbody> or direct <tr>
    tbody = table.find("tbody")
    row_source = tbody if tbody else table
    for tr in row_source.find_all("tr", recursive=(tbody is None)):
        # Skip rows already captured from thead
        if thead and tr.parent == thead:
            continue
        cells = [
            (td.get_text(separator=" ", strip=True) or "").replace("\n", " ")
            for td in tr.find_all(["th", "td"])
        ]
        if cells:
            rows.append(cells)

    if not rows:
        return "", 0, 0

    num_rows = len(rows)
    num_cols = max(len(r) for r in rows) if rows else 0

    lines: list[str] = []
    # Header row
    header = rows[0]
    while len(header) < num_cols:
        header.append("")
    lines.append("| " + " | ".join(header[:num_cols]) + " |")
    # Separator
    lines.append("| " + " | ".join("---" for _ in range(num_cols)) + " |")
    # Data rows
    for row in rows[1:]:
        while len(row) < num_cols:
            row.append("")
        lines.append("| " + " | ".join(row[:num_cols]) + " |")

    return "\n".join(lines), num_rows, num_cols


def _extract_list_items(list_tag: Tag) -> list[str]:
    """Extract text from list items in a <ul> or <ol>.

    Args:
        list_tag: BeautifulSoup Tag for a <ul> or <ol> element.

    Returns:
        List of text strings, one per <li>.
    """
    items: list[str] = []
    for li in list_tag.find_all("li", recursive=False):
        text = li.get_text(separator=" ", strip=True)
        if text:
            items.append(text)
    return items


def _get_text_content(element: Tag) -> str:
    """Get clean text content from an element, collapsing whitespace."""
    text = element.get_text(separator=" ", strip=True)
    # Collapse multiple whitespace
    text = re.sub(r"\s+", " ", text).strip()
    return text


def extract_html(file_path: str | Path) -> list[Block]:
    """Extract structured blocks from an HTML file.

    Walks the HTML DOM tree and extracts:
    - Headings (h1-h6)
    - Paragraphs (p, div with text)
    - Tables (converted to markdown)
    - Lists (ul, ol)
    - Code blocks (pre, code)

    Args:
        file_path: Path to the .html or .htm file.

    Returns:
        List of Block objects in document order.
    """
    file_path = Path(file_path)
    if not file_path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")
    if file_path.suffix.lower() not in (".html", ".htm"):
        raise ValueError(f"Expected .html or .htm file, got: {file_path.suffix}")

    with open(file_path, "r", encoding="utf-8", errors="replace") as f:
        content = f.read()

    soup = BeautifulSoup(content, "html.parser")

    # Try to find the main content area; fall back to body or whole document
    main = soup.find("main") or soup.find("article") or soup.find("body") or soup

    blocks: list[Block] = []
    _seen_texts: set[str] = set()  # Deduplicate blocks

    def _walk(element: Tag) -> None:
        """Recursively walk the DOM and extract blocks."""
        for child in element.children:
            if isinstance(child, NavigableString):
                continue
            if not isinstance(child, Tag):
                continue

            tag_name = child.name.lower() if child.name else ""

            # Skip non-content tags
            if tag_name in _SKIP_TAGS:
                continue

            # Headings
            if tag_name in _HEADING_TAGS:
                text = _get_text_content(child)
                if text and text not in _seen_texts:
                    _seen_texts.add(text)
                    blocks.append(Block(
                        type="heading",
                        text=text,
                        meta={"level": _HEADING_TAGS[tag_name]},
                    ))
                continue

            # Tables
            if tag_name == "table":
                md, num_rows, num_cols = _table_to_markdown(child)
                if md.strip() and md not in _seen_texts:
                    _seen_texts.add(md)
                    blocks.append(Block(
                        type="table",
                        text=md,
                        meta={
                            "format": "markdown",
                            "num_rows": num_rows,
                            "num_cols": num_cols,
                        },
                    ))
                continue

            # Lists
            if tag_name in ("ul", "ol"):
                items = _extract_list_items(child)
                for item_text in items:
                    if item_text and item_text not in _seen_texts:
                        _seen_texts.add(item_text)
                        blocks.append(Block(
                            type="list",
                            text=item_text,
                            meta={},
                        ))
                continue

            # Code blocks
            if tag_name == "pre":
                code_tag = child.find("code")
                text = code_tag.get_text() if code_tag else child.get_text()
                text = text.strip()
                if text and text not in _seen_texts:
                    _seen_texts.add(text)
                    # Try to detect language from class
                    lang = ""
                    if code_tag and code_tag.get("class"):
                        for cls in code_tag["class"]:
                            if cls.startswith("language-") or cls.startswith("lang-"):
                                lang = cls.split("-", 1)[1]
                                break
                    blocks.append(Block(
                        type="code",
                        text=text,
                        meta={"language": lang} if lang else {},
                    ))
                continue

            # Paragraphs
            if tag_name == "p":
                text = _get_text_content(child)
                if text and text not in _seen_texts:
                    _seen_texts.add(text)
                    blocks.append(Block(
                        type="paragraph",
                        text=text,
                        meta={},
                    ))
                continue

            # For container tags (div, section, article, etc.), recurse
            _walk(child)

    _walk(main)

    logger.info(
        "Extracted %d blocks from %s (headings=%d, paragraphs=%d, tables=%d, lists=%d, code=%d)",
        len(blocks),
        file_path.name,
        sum(1 for b in blocks if b.type == "heading"),
        sum(1 for b in blocks if b.type == "paragraph"),
        sum(1 for b in blocks if b.type == "table"),
        sum(1 for b in blocks if b.type == "list"),
        sum(1 for b in blocks if b.type == "code"),
    )

    return blocks
