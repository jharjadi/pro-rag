# Production-Ready RAG POC (Go Core + Python Ingestion) — Final Spec v2 (Option A DB Contract)

> This version incorporates the latest review items: explicit trade-offs, HNSW default, calibrated abstain thresholds, chunk sizing rules, FTS construction, a concrete prompt template, an explicit V1 auth story, ingestion_runs behavior, and build-order changes to force early eval.

---

## 0) What we are building (context for Roo / Claude 4.6)

We are building a **production-shaped Retrieval-Augmented Generation (RAG) system** for an **English-only internal company knowledge base**, with a planned upgrade path to **multi-tenant + external users**.

This is not a toy “chunk + embed + answer” demo. It must handle:
- messy documents (PDF/Word/HTML/images/spreadsheets),
- **table-aware extraction** (tables preserved, not shredded),
- **latest-only** answers (no stale versions),
- tenant isolation (data-model enforced in V1, hardened in V2),
- hybrid retrieval + reranking (with fail-open fallbacks),
- **evaluation + stress tests** built early.

Core principle: **bad retrieval is worse than no retrieval**—wrong context produces confident hallucinations. The runtime must **abstain** if evidence is weak and must always produce **citations** for factual claims.

---

## 1) Scope

### V1 (POC, dev-tool)
**Deliverables**
- Python ingestion: extract → restructure → chunk (structure-aware) → metadata → embed → FTS → Postgres (+ ingestion run tracking)
- Go query runtime: hybrid retrieval (vector + FTS) → **RRF merge** → rerank (API) **fail-open** → context budgeting → answer w/ citations → abstain
- Latest-only versioning (active versions only)
- Multi-tenant data model + strict query filters by tenant_id
- Eval harness (**Hit@K + MRR + abstain rate + latency**) + minimal red-team prompts
- Docker compose stack with reliable startup (migrate service + DB retries)

### V2 (hardening)
- Postgres RLS + per-tenant auth (API key/JWT) + per-tenant encryption/keys
- Better PDF table extraction + OCR pipeline
- Query expansion (HyDE / multi-query) once eval baseline exists
- Monitoring dashboards + automated red teaming + CI eval gates
- Ingestion orchestration (queue, retries, backpressure)

---

## 2) Architecture Overview

### Services
- `core-api-go` (Go, Chi): online query runtime (retrieval, merge, rerank, prompting, citations, abstain)
- `ingest-py` (Python): ingestion pipeline + offline tooling
- `postgres` (pgvector + FTS): primary DB

### Option A Contract
Python writes artifacts into DB. Go only reads from DB at runtime.

---

## 3) Data Model and Schema (explicit trade-offs)

### Tables (V1 minimum)
- `tenants(tenant_id, name, embedding_model, embedding_dim, created_at)`
- `documents(doc_id, tenant_id, source_type, source_uri, title, content_hash, created_at)`
- `document_versions(doc_version_id, tenant_id, doc_id, version_label, effective_at, is_active, extracted_artifact_uri, created_at)`
- `chunks(chunk_id, tenant_id, doc_version_id, ordinal, heading_path jsonb, chunk_type, text, token_count, metadata jsonb, created_at)`
- `chunk_embeddings(chunk_id, tenant_id, embedding_model, embedding vector(dim), created_at)`
- `chunk_fts(chunk_id, tenant_id, tsv tsvector)`
- `ingestion_runs(run_id, tenant_id, status, started_at, finished_at, config jsonb, stats jsonb, error text)`

### 3.1) Chunk embeddings table: call the trade-off honestly
**Decision (V1): keep `chunk_embeddings` separate.**

**Trade-off**
- ✅ Pros: enables future **re-embed migrations** (multiple embedding models/versions) without rewriting `chunks`; supports partial backfills and A/B comparisons.
- ❌ Cons: adds a **multi-table join** on the hot path (vector → chunks → document_versions → documents).

**What “acceptable for V1” means (not hand-wavy)**
- V1 targets small corpora (10–50 docs, typically 1k–10k chunks). A 3–4 table join with proper indexes is fine.
- If you grow to ~50k+ chunks and see latency, add the **V1.5 optimization task** below.

**V1.5 named task: `MV_ACTIVE_CHUNKS`**
Create a materialized view/table for hot-path reads:
- `active_chunks_mv(tenant_id, chunk_id, doc_id, doc_version_id, title, version_label, heading_path, chunk_type, text, token_count, metadata, embedding, tsv)`
- Refresh per ingestion activation (or incremental per activated document_version).

### 3.2) Constraints / indexes (must)
- Tenant filter everywhere: `WHERE tenant_id = $1`
- Latest-only: always join `document_versions` and filter `is_active = true`
- One active version per (tenant_id, doc_id): partial unique index
- FTS index: GIN on `chunk_fts.tsv`

### 3.3) Vector index choice (V1 default = HNSW)
**Decision (V1): use HNSW**
- Works well at small and medium scale without IVF tuning.
- HNSW index on `chunk_embeddings.embedding`.
- IVF (ivfflat) deferred to V2 if you later optimize for very large corpora.

---

## 4) Embedding Model Choice (English-only)

**Default (V1):** `BAAI/bge-base-en-v1.5` (**768-dim**)
Rule: pin per tenant (`tenants.embedding_model`, `tenants.embedding_dim`). Changing requires explicit re-embed job.

---

## 5) Ingestion Pipeline (Python)

### Inputs
PDF, DOCX, HTML, images, spreadsheets.

### Outputs
- DB rows for documents/versions/chunks/embeddings/fts

### Batch embedding (required)
Embedding must be done **in batches**, not one-by-one.

Default rule:
- batch encode all chunks per document in one call
- `BATCH_SIZE<=256` (tune per GPU/CPU memory)
- preserve chunk ordering (chunk_id mapping)

Rationale: batching is dramatically faster (often 10–50× on large docs) and reduces overhead.

- Debug artifact referenced via `extracted_artifact_uri`

### `extracted_artifact_uri`
- V1 local: `file:///data/artifacts/<tenant>/<doc>/<version>.json`
- V2 cloud: `s3://...`

### Structure-aware extraction + chunking rules (with defaults)
Extractor produces blocks: `{type: heading|paragraph|table|list|code, text, meta}`

**Non-table chunk sizing**
- Target: **350–500 tokens** (default 450)
- Max: **800 tokens** (hard cap)
- Split boundaries: heading → paragraph → sentence
- Overlap: **0** in V1

**Tables**
- Never split arbitrarily.
- If huge: split by row groups targeting **≤800 tokens per chunk**, with a soft preference for **25–50 rows**. Always repeat header row. If a **single row** exceeds 800 tokens, keep it as one chunk and **log a warning** during ingestion.

### `chunks.metadata` JSONB schema
Minimum keys for Go: `summary`, `keywords` (empty allowed).

```json
{
  "summary": "string",
  "keywords": ["k1","k2"],
  "hypothetical_questions": ["q1","q2"],
  "source": {"page": 12, "bbox": null},
  "table": {"format": "markdown", "csv": "..."}
}
```

### ingestion_runs usage (no dead schema)
- Create run row at start (`running`)
- Update stats during pipeline
- On success: `succeeded` + finished_at
- On failure: `failed` + error with stage + trace

---

## 6) Query Runtime (Go)

### Endpoint
`POST /v1/query`
```json
{
  "tenant_id": "uuid",
  "question": "string",
  "top_k": 10,
  "debug": false
}
```

### Response contracts
**Success** — HTTP 200
```json
{"answer":"...","citations":[{"doc_id":"...","doc_version_id":"...","chunk_id":"...","title":"...","version_label":"..."}],"abstained":false,"debug":null}
```

**Abstain** — HTTP 200
```json
{"answer":"I don't have enough information in the current documents to answer that.","citations":[],"abstained":true,"debug":null}
```

**Bad request** — HTTP 400
```json
{"error":"bad_request","message":"..."}
```

**LLM failure** — HTTP 502
```json
{"error":"llm_unavailable","message":"..."}
```

**Internal** — HTTP 500
```json
{"error":"internal","message":"..."}
```

Notes:
- `debug` is populated only when request `debug=true`.
- On reranker fail-open: include `debug.reranker_skipped=true` (and `debug.reranker_error` if available).


### Auth story (explicit)
- **V1:** tenant_id is caller-provided and trusted (no auth) — local/internal only.
- **V2:** API key/JWT bound to tenant; derive tenant from auth.

### Retrieval pipeline
1) Vector top Kvec=50 (tenant + active versions)
2) FTS top Kfts=50 using:
   - `websearch_to_tsquery('english', question)`
   - rank: `ts_rank_cd(tsv, tsquery)`
3) Merge with **RRF** (k=60)
4) Rerank with Cohere (fail-open)
5) Context budgeting:
   - MAX_CONTEXT_TOKENS=6000
   - overhead=1000 (reserved for system prompt + question)
   - max_chunks=12

   Rationale: 6000 balances cost/latency for V1. Raise to 12-16k once eval
   confirms retrieval quality justifies more context. Not a technical limit —
   the LLM supports 128k+.

### Reranker provider (V1): Cohere Rerank v3.5
```env
RERANK_PROVIDER=cohere
COHERE_API_KEY=...
COHERE_RERANK_MODEL=rerank-v3.5
RERANK_TIMEOUT_MS=3000
RERANK_MAX_DOCS=50
RERANK_FAIL_OPEN=true
```

---

## 7) Abstain Logic (separate thresholds)

```env
ABSTAIN_RERANK_THRESHOLD=0.15
ABSTAIN_RRF_THRESHOLD=0.030
```

With 2 rank lists and k=60, a chunk ranked #1 in both lists scores 2 × (1/61) ≈ 0.033. A threshold of 0.030 means “roughly equivalent to a single top-1 ranking.” Recalibrate if you change k or add more rank lists.


- Apply rerank threshold only when reranker succeeded.
- Apply RRF threshold when reranker failed/disabled.
- Tune empirically with eval.

---

## 8) Prompt Template (V1 default)

**System**
You are a careful assistant answering questions using ONLY the provided context.
Rules:
1) If the answer is not clearly supported by the context, say you don't know and ask a clarifying question.
2) Do NOT use outside knowledge. Do NOT guess.
3) Every factual claim must include citations like [chunk:<CHUNK_ID>].
4) If the user asks for something outside scope, explain what's missing.

**Context format (per chunk)**
- Title: <title>
- Version: <version_label>
- Heading: <heading_path>
- ChunkID: <chunk_id>
- Text: <chunk text>

**Assistant output**
- Short paragraphs or bullets.
- Inline citations: ... [chunk:abc123]
- If abstaining: explain why + ask clarifying question.

---

**Abstain example (include verbatim in system prompt as one-shot)**
User: What is our parental leave policy in Germany?
Assistant: I don't have enough information in the current documents to
answer this specifically for Germany. The available documents cover US
policy only. Could you clarify which document should contain this, or
check whether the Germany-specific policy has been uploaded?
[No citations available]

## 9) Evaluation Framework (V1)

- `questions.jsonl` must have **30–50+ questions**.
- Metrics: Hit@K, MRR, abstain rate, latency (+ stage timings).
- `run_eval.py` outputs CSV.
- `run_redteam.py` runs injection/exfil/stale-policy probes.

---

## 10) Docker Compose Reliability

- `migrate` one-shot service runs migrations then exits.
- core-api retries DB and checks required tables/extensions.

---

## 11) Build Order (forces early eval)

1) Scaffold + DB + compose
2) Ingest **5 real docs**
3) Implement eval harness and run it
4) Iterate chunking/retrieval until metrics are sane
5) Then finish full ingestion + redteam

---

## 12) Roo Build Tasks

1) Scaffold repo + env
2) Compose + migrate service + artifact volume
3) Migrations + seed (HNSW vector index)
4) Python ingest CLI (chunk sizing defaults, ingestion_runs tracking)
5) Go query API (RRF + Cohere fail-open + prompt template + abstain thresholds)
6) Eval + redteam scripts
7) Optional V1.5: MV_ACTIVE_CHUNKS

---

## 13) Done Definition (V1)

- Ingest 10–50 docs incl. a table-heavy PDF.
- Query returns citations and respects latest-only.
- Reranker can fail without breaking query path.
- Eval outputs Hit@K + MRR; rerank improves MRR vs no rerank.
- Redteam shows no tenant leakage and reasonable abstains.

---

## Blunt challenges (don’t ignore)

1) If PDF table extraction is weak, nothing else matters.
2) If embedding model + chunking strategy mismatch, retrieval silently underperforms—run eval after first ingestion.
3) Don’t add HyDE/multi-query before you have MRR baseline.
4) Don’t skip startup reliability; compose ordering will bite you in CI/ECS.




## Logging and Observability (V1 minimum)

Runtime must emit **one structured JSON log line per query** (stdout), so you can correlate eval results and debug behavior.

Required fields:
- `ts` (RFC3339)
- `tenant_id`
- `request_id`
- `question_hash` (SHA-256 of normalized question; do not log raw question by default)
- `k_vec`, `k_fts`, `k_rerank`
- `num_vec_candidates`, `num_fts_candidates`, `num_merged_candidates`
- `reranker_used` (bool), `reranker_skipped` (bool), `reranker_latency_ms`
- `num_context_chunks`, `context_tokens_est`
- `abstained` (bool)
- `latency_ms_total` and stage timings: `latency_ms_vec`, `latency_ms_fts`, `latency_ms_merge`, `latency_ms_rerank`, `latency_ms_llm`
- `llm_provider`, `llm_model`
- `llm_prompt_tokens`, `llm_completion_tokens` (if available)
- `http_status`

Example (single line):
```json
{"ts":"2026-02-11T12:34:56Z","tenant_id":"...","request_id":"...","question_hash":"...","k_vec":50,"k_fts":50,"k_rerank":50,"num_vec_candidates":50,"num_fts_candidates":50,"num_merged_candidates":78,"reranker_used":true,"reranker_skipped":false,"reranker_latency_ms":210,"num_context_chunks":10,"context_tokens_est":5400,"abstained":false,"latency_ms_total":980,"latency_ms_vec":55,"latency_ms_fts":40,"latency_ms_merge":5,"latency_ms_rerank":210,"latency_ms_llm":650,"llm_provider":"anthropic","llm_model":"claude-4.6","llm_prompt_tokens":6200,"llm_completion_tokens":310,"http_status":200}
```
