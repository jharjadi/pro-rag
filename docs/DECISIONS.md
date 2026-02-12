# DECISIONS.md — pro-rag

Architectural Decision Records (ADRs) for the pro-rag project.

---

## ADR-001: All-Python for V1 (instead of Go + Python)

**Date:** 2026-02-12  
**Status:** Accepted

**Context:** The original spec (v7) prescribed Go for the query runtime and Python for ingestion. For a V1 POC, we evaluated whether the two-language approach was justified.

**Decision:** Use Python for both query runtime (FastAPI) and ingestion pipeline in V1.

**Rationale:**
- The query runtime is I/O-bound (DB queries + API calls to Cohere/LLM), not CPU-bound. Python with async (FastAPI + asyncpg) handles this fine at POC scale.
- Single language means faster iteration on the critical feedback loop: change chunking → re-ingest → run eval → adjust retrieval.
- Simpler infrastructure: one Dockerfile pattern, one test runner (pytest), one dependency manager.
- The Option A contract (Python writes, runtime reads) means we can swap in a Go query service in V2 without touching ingestion.

**Alternatives considered:**
- Go + Python (as spec'd): better production performance, but slower to iterate on a POC.
- All-Go: Go's ML/NLP ecosystem is too weak for ingestion (PDF extraction, sentence-transformers, etc.).

**Upgrade path:** If V2 needs sub-10ms p99 on the query path, rewrite query-api in Go. The DB contract makes this a clean swap.

---

## ADR-002: Separate chunk_embeddings table

**Date:** 2026-02-12  
**Status:** Accepted (from spec v7 §3.1)

**Context:** Embeddings could live in the `chunks` table or a separate `chunk_embeddings` table.

**Decision:** Keep `chunk_embeddings` separate.

**Rationale:** Enables future re-embed migrations without rewriting chunks. Supports partial backfills and A/B comparisons. The multi-table join is acceptable at V1 scale (1k–10k chunks).

**V1.5 optimization:** If latency becomes an issue at 50k+ chunks, create `MV_ACTIVE_CHUNKS` materialized view.

---

## ADR-003: HNSW vector index (not IVF)

**Date:** 2026-02-12  
**Status:** Accepted (from spec v7 §3.3)

**Decision:** Use HNSW for vector index in V1.

**Rationale:** Works well at small/medium scale without IVF tuning. IVF deferred to V2.
