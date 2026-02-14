"""Metadata generation for chunks.

Spec §3a.8:
- chunks.metadata JSONB: {"summary": "", "keywords": []}
- V1: keywords extracted via simple TF-IDF or keyword extraction from chunk text
- hypothetical_questions: empty list (placeholder for V2)
"""

from __future__ import annotations

import logging
import re
from collections import Counter

logger = logging.getLogger(__name__)

# Common English stop words for keyword extraction
_STOP_WORDS = frozenset({
    "a", "an", "the", "and", "or", "but", "in", "on", "at", "to", "for",
    "of", "with", "by", "from", "is", "are", "was", "were", "be", "been",
    "being", "have", "has", "had", "do", "does", "did", "will", "would",
    "could", "should", "may", "might", "shall", "can", "need", "must",
    "it", "its", "this", "that", "these", "those", "i", "you", "he", "she",
    "we", "they", "me", "him", "her", "us", "them", "my", "your", "his",
    "our", "their", "what", "which", "who", "whom", "when", "where", "why",
    "how", "all", "each", "every", "both", "few", "more", "most", "other",
    "some", "such", "no", "not", "only", "own", "same", "so", "than",
    "too", "very", "just", "because", "as", "until", "while", "about",
    "between", "through", "during", "before", "after", "above", "below",
    "up", "down", "out", "off", "over", "under", "again", "further",
    "then", "once", "here", "there", "also", "if", "into",
})

# Max keywords per chunk
_MAX_KEYWORDS = 8


def _extract_words(text: str) -> list[str]:
    """Extract lowercase alphabetic words from text."""
    return [w.lower() for w in re.findall(r"[a-zA-Z]{3,}", text)]


def extract_keywords(text: str, max_keywords: int = _MAX_KEYWORDS) -> list[str]:
    """Extract keywords from text using simple frequency-based approach.

    Filters stop words, returns top-N most frequent meaningful words.
    """
    words = _extract_words(text)
    filtered = [w for w in words if w not in _STOP_WORDS]

    if not filtered:
        return []

    counts = Counter(filtered)
    # Return top keywords by frequency
    return [word for word, _ in counts.most_common(max_keywords)]


def generate_chunk_metadata(
    text: str,
    chunk_type: str,
    extra: dict | None = None,
) -> dict:
    """Generate metadata dict for a chunk.

    Args:
        text: Chunk text.
        chunk_type: "text" or "table".
        extra: Additional metadata to merge (e.g., table format info).

    Returns:
        Metadata dict with summary, keywords, hypothetical_questions.
    """
    keywords = extract_keywords(text)

    metadata: dict = {
        "summary": "",  # V1: empty, V2: LLM-generated
        "keywords": keywords,
        "hypothetical_questions": [],  # V2 placeholder
    }

    # Add table-specific metadata
    if chunk_type == "table" and extra:
        if "format" in extra:
            metadata["table"] = {"format": extra["format"]}

    return metadata
