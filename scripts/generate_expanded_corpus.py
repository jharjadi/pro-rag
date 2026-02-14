#!/usr/bin/env python3
"""Generate 10 additional synthetic documents for Phase 3b.5 expanded corpus.

Adds to the 5 DOCX documents from Phase 4a to reach 15 total documents.
Includes multiple formats (DOCX, HTML, PDF) and table-heavy content.

New documents:
 6. Annual Leave & Benefits Summary (DOCX, table-heavy)
 7. Software Development Lifecycle (DOCX)
 8. Data Retention Policy (DOCX)
 9. Vendor Management Policy (HTML)
10. Acceptable Use Policy (HTML)
11. Business Continuity Plan (HTML, table-heavy)
12. Employee Compensation Bands (PDF, table-heavy)
13. IT Asset Inventory Standards (PDF, table-heavy)
14. Code of Conduct (DOCX)
15. Travel Safety Guidelines (DOCX)

Usage: cd ingest && .venv/bin/python ../scripts/generate_expanded_corpus.py
"""

from __future__ import annotations

import sys
from pathlib import Path

# Add scripts dir to path so we can import sibling modules
sys.path.insert(0, str(Path(__file__).parent))

OUTPUT_DIR = Path(__file__).parent.parent / "data" / "test-corpus"


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    print(f"Generating expanded corpus in {OUTPUT_DIR}/")

    # DOCX documents (6, 7, 8, 14, 15)
    from gen_corpus_docx import generate_all as gen_docx
    gen_docx()

    # HTML documents (9, 10, 11)
    from gen_corpus_html import generate_all as gen_html
    gen_html()

    # PDF documents (12, 13) — requires fpdf2
    from gen_corpus_pdf import generate_all as gen_pdf
    gen_pdf()

    print("\nDone! Generated 10 additional documents (15 total with Phase 4a).")


if __name__ == "__main__":
    main()
