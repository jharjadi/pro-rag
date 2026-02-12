# DECISIONS.md — pro-rag

Architectural Decision Records (ADRs) for the pro-rag project.

---

## ADR-001: All-Python for V1 (instead of Go + Python)

**Date:** 2026-02-12  
**Status:** Superseded by ADR-004

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

**Why superseded:** See ADR-004. The contract boundary erosion risk was deemed too high — with both services in Python, the temptation to share code across the Option A boundary undermines the clean separation that makes future evolution possible. Two languages enforce the contract physically.

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

---

## ADR-004: Go query runtime + Python ingestion (as spec'd)

**Date:** 2026-02-12  
**Status:** Accepted (supersedes ADR-001)

**Context:** ADR-001 proposed all-Python for V1. During plan review, we identified that the contract boundary erosion risk was the critical issue: with both services in Python, nothing prevents importing ingestion modules into the query service, silently coupling them and killing the clean swap path.

**Decision:** Use Go (Chi router) for the query runtime and Python for the ingestion pipeline, as the original spec prescribes.

**Rationale:**
- **Forced contract separation.** Two languages make it physically impossible to `from ingest.chunk import ...` in the query service. The Option A boundary is enforced by the type system, not by discipline.
- **The query service is simple.** ~5 endpoints, DB reads, HTTP calls to Cohere/LLM, JSON responses. Go with Chi handles this well. The complexity is in the logic (RRF, abstain, context budgeting), not the framework.
- **Go's strengths align with query needs.** Low latency, excellent concurrency for parallel vector+FTS queries, small Docker images, fast startup.
- **Python's strengths align with ingestion needs.** PDF extraction, sentence-transformers, tiktoken, NLP libraries — Go has nothing comparable.
- **The iteration loop that matters is ingestion → eval.** That's all Python. The query service changes less frequently once retrieval logic is right.

**Trade-off acknowledged:**
- Two Dockerfiles, two test runners (`go test` + `pytest`), two dependency ecosystems.
- Slightly more infrastructure complexity.
- Accepted because the contract enforcement benefit outweighs the infrastructure cost.
