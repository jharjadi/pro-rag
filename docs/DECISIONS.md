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

---

## ADR-005: Embedding sidecar for Go query API

**Date:** 2026-02-14
**Status:** Accepted

**Context:** The Go query API needs to embed the user's question into a vector for cosine similarity search. The embedding model (BAAI/bge-base-en-v1.5) runs via Python's sentence-transformers library. Go has no native equivalent.

**Decision:** Add a lightweight Python/Flask HTTP sidecar (`embed-svc`) that wraps sentence-transformers and exposes a `POST /embed` endpoint. The Go API calls this sidecar to embed questions.

**Rationale:**
- **Same model, same embeddings.** Using the same sentence-transformers model for both ingestion and query ensures embedding consistency. A different embedding API (e.g., Cohere, OpenAI) would produce different vectors incompatible with the stored embeddings.
- **Simple HTTP contract.** `{"texts": [...]}` → `{"embeddings": [[...]]}`. Easy to test, easy to replace.
- **Docker Compose native.** The sidecar runs as a service with a healthcheck. core-api-go depends on it via `service_healthy`.
- **Minimal code.** ~60 lines of Flask. Model is pre-downloaded at Docker build time for fast startup.

**Trade-off acknowledged:**
- Adds one more container to the stack.
- Adds ~10ms latency per question embedding (local HTTP call).
- Accepted because embedding consistency is non-negotiable, and the latency is negligible compared to LLM calls (~500-1000ms).

**Alternatives considered:**
- Call Cohere/OpenAI embedding API: different model = incompatible vectors with stored BAAI/bge embeddings.
- Embed in Go using ONNX runtime: complex setup, fragile, not worth it for V1.
- Pre-compute question embeddings: not possible for live queries.

---

## ADR-006: Next.js Web UI with Go as single API gateway

**Date:** 2026-02-14
**Status:** Accepted

**Context:** The V1 system was API-only + CLI. Users needed `curl` to query and the Python CLI to ingest. A web UI was needed for document management, chat, and ingestion monitoring.

**Decision:**
1. **Next.js 15 (App Router) + TypeScript + Tailwind CSS** for the web UI.
2. **Go (core-api-go) as the single API gateway** — all external traffic routes through Go (:8000).
3. **ingest-api (FastAPI) is internal-only** — no external port, only reachable on the Docker network.
4. **Next.js BFF proxy** — all browser API calls go through Next.js API routes → Go. No CORS needed.

**Rationale:**
- **Single gateway:** One entry point for all external traffic means auth, rate limiting, logging, and audit all live in one place when V2 adds them. The ingest-api becomes a true internal service.
- **Next.js over SPA:** SSR, API routes for BFF proxy, file-based routing. The BFF layer eliminates CORS entirely.
- **Next.js over single HTML file:** The old `web/index.html` was too limited for document management, file uploads, routing, and multiple pages.
- **Go proxies ingestion:** The extra HTTP hop on the internal Docker network adds ~1-2ms to an operation that takes 10-60 seconds. The architectural clarity is worth far more.

**Alternatives considered:**
- Single HTML file served by Go: Too limited for document management, file uploads, routing.
- Vite + React SPA: No SSR, no API routes for BFF proxy, more manual setup.
- Next.js BFF calling both Go and Python directly: Violates single-gateway principle, complicates auth in V2.

**Known V1 limitations:**
- Chat history is client-side only (refreshing loses conversation).
- Single tenant (hardcoded UUID).
- Type sharing is manual (TypeScript types maintained separately from Go structs).
- No auth (local/internal only).

---

## ADR-007: Go-orchestrated ingestion with Python worker (spec v2.3)

**Date:** 2026-02-15
**Status:** Accepted (supersedes ingest-api proxy pattern from ADR-006)

**Context:** The original architecture had Go proxy file uploads to `ingest-api` (FastAPI), which handled everything: file storage, document creation, run tracking, and pipeline execution. This meant Python owned all write paths, and Go was a dumb proxy for ingestion. This created problems:
- No content-addressed dedup (Go couldn't inspect the file before forwarding).
- No crash recovery (if ingest-api died mid-job, runs stayed "running" forever).
- No separation between orchestration (who/what/when) and processing (extract/chunk/embed).
- Go couldn't enforce upload limits or compute file hashes without buffering the entire file.

**Decision:** Go becomes the ingestion orchestrator. Python becomes a stateless worker.

**Write ownership split:**
| Table | Go writes | Python writes |
|-------|-----------|---------------|
| `documents` | ✅ (INSERT ON CONFLICT) | — |
| `ingestion_runs` | ✅ (status=queued) | ✅ (status transitions, heartbeat, stats) |
| `document_versions` | — | ✅ (content_hash, version creation) |
| `chunks` | — | ✅ |
| `chunk_embeddings` | — | ✅ |
| `chunk_fts` | — | ✅ |

**Key design choices:**
1. **Streaming SHA-256**: Go computes the hash during upload via `io.TeeReader` — never buffers the full file in memory.
2. **Content-addressed dedup**: `source_uri = upload://sha256:<hash>` — same content always maps to same document via `INSERT ON CONFLICT`.
3. **Crash guard**: On startup, Go marks stale queued (>1h) and running (>15min no heartbeat) runs as failed.
4. **Bounded concurrency**: Worker uses `ThreadPoolExecutor(max_workers=3)` and returns 503 when busy.
5. **Internal auth**: Shared HMAC token (`INTERNAL_AUTH_TOKEN`) between Go and worker. Worker validates Bearer token.
6. **Heartbeat**: Worker updates `ingestion_runs.updated_at` after each pipeline stage for crash detection.
7. **V1 = HTTP delegation**: Go POSTs job payload to worker. V2 can swap to SQS without changing the worker contract.

**Schema changes (migrations 009–011):**
- `users` table added (for future auth).
- `content_hash` moved from `documents` to `document_versions`.
- `documents` gets unique index on `(tenant_id, source_uri)` for INSERT ON CONFLICT.
- `ingestion_runs` gets `queued` status, `doc_id`, `created_at`, `updated_at`, `run_type`, `source_id`, `initiated_by`.
- `started_at` becomes nullable (set by worker, not Go).

**Rationale:**
- **Dedup before processing**: Go can skip the entire pipeline if the content hash matches the active version.
- **Crash recovery**: Heartbeat + crash guard means no run stays stuck forever.
- **Clean separation**: Orchestration logic (dedup, routing, status) in Go; content processing (extract, chunk, embed) in Python.
- **Future-proof**: The worker contract (`POST /internal/process` with JSON payload) works identically for HTTP and SQS delivery.

**Trade-off acknowledged:**
- Go now writes to `documents` and `ingestion_runs`, breaking the pure "Python writes, Go reads" model from ADR-004.
- Accepted because the orchestration tables are structurally different from content tables, and the split is well-defined.
- The CLI ingestion path (`ingest` service) still writes directly to all tables for batch operations.

**Replaces:** `ingest-api` (FastAPI) service. The `ingest-worker` (Flask) service takes its place with a different contract.
