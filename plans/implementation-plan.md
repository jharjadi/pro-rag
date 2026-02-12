# pro-rag V1 Implementation Plan

> Generated: 2026-02-12 | Spec: `plans/ragstack_rag_poc_spec_v7.md` | Architecture: `docs/ARCHITECTURE.md`

## Status Key
- ✅ Done
- 🔄 In Progress
- ⬚ Not Started

---

## Phase 0: Project Setup ✅ (commit b2e2e64)
- [x] Create project docs (spec, ARCHITECTURE.md, DECISIONS.md)
- [x] Set up Roo rules (.roo/rules-*)
- [x] Initialize git repo + push to GitHub (jharjadi/pro-rag)

---

## Phase 1: Project Scaffolding + Docker + DB ✅ (commit e074892)

### 1.1 — Repository structure
Create the directory layout:

```
pro-rag/
├── docker-compose.yml
├── Makefile
├── .env.example
├── .gitignore
├── DEVELOPMENT_RULES.md
├── query-api/
│   ├── Dockerfile
│   ├── pyproject.toml
│   ├── app/
│   │   ├── __init__.py
│   │   ├── main.py          # FastAPI app
│   │   ├── config.py         # Settings via pydantic-settings
│   │   ├── routes/
│   │   │   └── query.py      # POST /v1/query
│   │   ├── services/
│   │   ├── models/
│   │   └── db/
│   └── tests/
├── ingest/
│   ├── Dockerfile
│   ├── pyproject.toml
│   ├── ingest/
│   │   ├── __init__.py
│   │   ├── cli.py            # CLI entry point
│   │   ├── pipeline.py       # Orchestrator
│   │   ├── extract/
│   │   ├── chunk/
│   │   ├── embed/
│   │   ├── fts/
│   │   └── db/
│   └── tests/
├── migrations/
│   ├── 001_extensions.sql
│   ├── 002_tenants.sql
│   ├── 003_documents.sql
│   ├── 004_chunks.sql
│   └── ...
├── eval/
│   ├── questions.jsonl
│   ├── run_eval.py
│   └── run_redteam.py
├── docs/
├── plans/
└── data/
    └── artifacts/            # Extracted artifact storage (local)
```

### 1.2 — Docker Compose stack
- `postgres` service: Postgres 16 + pgvector extension, port 5432, persistent volume
- `migrate` service: one-shot, runs all SQL migrations in order, then exits
- `query-api` service: FastAPI app, depends on migrate, port 8000
- `ingest` service: profile-based (not always-on), mounts data volume

### 1.3 — Makefile targets (initial set)
```makefile
db-up          # docker compose up -d postgres
db-down        # docker compose down
db-migrate     # docker compose run --rm migrate
db-reset       # db-down + remove volume + db-up + db-migrate
api-run        # docker compose up query-api
api-test       # pytest query-api/tests/
ingest-run     # docker compose run ingest
ingest-test    # pytest ingest/tests/
test           # api-test + ingest-test
eval           # python eval/run_eval.py
e2e-smoke      # smoke test script
```

### 1.4 — Environment config
Create `.env.example` with all required variables:
```env
# Database
DATABASE_URL=postgresql://prorag:prorag@localhost:5432/prorag

# Embedding
EMBEDDING_MODEL=BAAI/bge-base-en-v1.5
EMBEDDING_DIM=768

# Reranker
RERANK_PROVIDER=cohere
COHERE_API_KEY=
COHERE_RERANK_MODEL=rerank-v3.5
RERANK_TIMEOUT_MS=3000
RERANK_MAX_DOCS=50
RERANK_FAIL_OPEN=true

# LLM
LLM_PROVIDER=anthropic
LLM_MODEL=claude-sonnet-4-20250514
LLM_API_KEY=

# Retrieval
K_VEC=50
K_FTS=50
RRF_K=60
MAX_CONTEXT_TOKENS=6000
CONTEXT_OVERHEAD=1000
MAX_CHUNKS=12

# Abstain
ABSTAIN_RERANK_THRESHOLD=0.15
ABSTAIN_RRF_THRESHOLD=0.030
```

### Definition of Done — Phase 1
- `make db-up` starts Postgres with pgvector
- `make db-migrate` runs all migrations successfully
- `make db-reset` works end-to-end
- Docker Compose brings up all services
- Directory structure matches the layout above

---

## Phase 2: DB Migrations + Seed Data ✅ (commit e074892, combined with Phase 1)

### 2.1 — Migration files
Create SQL migrations in order:
1. `001_extensions.sql` — Enable `uuid-ossp` and `vector` extensions
2. `002_tenants.sql` — Create `tenants` table
3. `003_documents.sql` — Create `documents` table with tenant FK
4. `004_document_versions.sql` — Create `document_versions` with partial unique index on (tenant_id, doc_id) WHERE is_active = true
5. `005_chunks.sql` — Create `chunks` table
6. `006_chunk_embeddings.sql` — Create `chunk_embeddings` table with HNSW vector index
7. `007_chunk_fts.sql` — Create `chunk_fts` table with GIN index on tsv
8. `008_ingestion_runs.sql` — Create `ingestion_runs` table

### 2.2 — Migration runner
Simple shell script or Python script that:
- Connects to Postgres
- Creates a `schema_migrations` tracking table
- Runs each .sql file in order, skipping already-applied ones
- Used by the `migrate` Docker service

### 2.3 — Seed data
Create a seed script that inserts:
- One test tenant (embedding_model=BAAI/bge-base-en-v1.5, embedding_dim=768)
- Used for local development and testing

### Definition of Done — Phase 2
- All 8 migration files exist and run cleanly
- `make db-migrate` applies all migrations idempotently
- `make db-reset` recreates from scratch
- Partial unique index enforces one active version per (tenant_id, doc_id)
- HNSW and GIN indexes are created

---

## Phase 3: Python Ingestion Pipeline — MVP ⬚

### 3.1 — Project setup
- `pyproject.toml` with dependencies: sentence-transformers, asyncpg/psycopg2, python-docx, pdfplumber/pymupdf, beautifulsoup4, tiktoken
- CLI entry point via Click or argparse

### 3.2 — Extraction layer (`ingest/extract/`)
- PDF extractor (pdfplumber for tables, pymupdf for text)
- DOCX extractor (python-docx)
- HTML extractor (beautifulsoup4)
- Output: list of blocks `{type, text, meta}`
- Table-aware: preserve tables as markdown

### 3.3 — Chunking layer (`ingest/chunk/`)
- Structure-aware chunker using heading → paragraph → sentence boundaries
- Non-table: target 350-500 tokens (default 450), hard cap 800
- Tables: never split arbitrarily; split by row groups ≤800 tokens, repeat header
- Single row >800 tokens: keep as one chunk + log warning
- Overlap: 0 in V1
- Token counting via tiktoken

### 3.4 — Embedding layer (`ingest/embed/`)
- Batch embedding using sentence-transformers
- Model: BAAI/bge-base-en-v1.5 (768-dim)
- BATCH_SIZE ≤ 256
- Preserve chunk ordering

### 3.5 — FTS layer (`ingest/fts/`)
- Generate tsvector for each chunk using Postgres `to_tsvector('english', text)`
- Write to `chunk_fts` table

### 3.6 — DB writer (`ingest/db/`)
- Write documents, document_versions, chunks, chunk_embeddings, chunk_fts
- All writes within a transaction
- Content hash for deduplication
- Extracted artifact URI: `file:///data/artifacts/<tenant>/<doc>/<version>.json`

### 3.7 — Ingestion run tracking
- Create `ingestion_runs` row at start (status=running)
- Update stats during pipeline (chunks processed, etc.)
- On success: status=succeeded + finished_at
- On failure: status=failed + error with stage + trace

### 3.8 — Metadata generation
- `chunks.metadata` JSONB with minimum keys: `summary`, `keywords`
- V1: keywords extracted from text (simple TF-IDF or keyword extraction)
- `hypothetical_questions`: empty in V1 (placeholder for V2)

### Definition of Done — Phase 3
- `make ingest-run` processes a single document end-to-end
- Chunks appear in DB with correct embeddings and FTS vectors
- Ingestion run is tracked (running → succeeded/failed)
- Token counts are within spec limits
- Tables are preserved, not shredded
- Unit tests for chunker (boundary cases, table handling)

---

## Phase 4: Ingest 5 Real Docs + Eval Harness ⬚

> **This is the critical early-eval phase.** Per spec §11: ingest real docs, run eval, iterate before building more.

### 4.1 — Prepare test corpus
- Select 5 real documents (mix of PDF, DOCX, HTML)
- At least 1 table-heavy PDF
- Place in `data/test-corpus/`

### 4.2 — Ingest test corpus
- Run ingestion on all 5 docs
- Verify chunks, embeddings, FTS in DB
- Check ingestion_runs for all 5

### 4.3 — Eval harness (`eval/`)
- Create `questions.jsonl` with 30-50 questions covering the test corpus
- Each question: `{question, expected_chunk_ids, expected_abstain}`
- `run_eval.py`:
  - Calls retrieval pipeline (vector + FTS + RRF)
  - Computes: Hit@K, MRR, abstain rate, latency + stage timings
  - Outputs CSV results
- Does NOT require the full query API yet — can call retrieval functions directly

### 4.4 — Iterate chunking/retrieval
- Run eval, analyze results
- Adjust chunk sizing, embedding model, retrieval params if needed
- Document findings in `docs/project-notes/`

### Definition of Done — Phase 4
- 5 docs ingested successfully
- 30-50 eval questions created
- `make eval` runs and outputs Hit@K, MRR, abstain rate
- Retrieval quality is reasonable (iterate if not)
- Findings documented

---

## Phase 5: Python Query API — FastAPI ⬚

### 5.1 — FastAPI app skeleton
- `app/main.py`: FastAPI app with lifespan (DB pool init/close)
- `app/config.py`: pydantic-settings for all env vars
- `app/db/pool.py`: asyncpg connection pool with retry logic
- Health check endpoint: `GET /health`

### 5.2 — Retrieval service (`app/services/retrieval.py`)
- Vector search: top K_VEC=50, filtered by tenant_id + active versions
- FTS search: `websearch_to_tsquery('english', question)`, ranked by `ts_rank_cd`
- RRF merge (k=60)
- Return merged candidate list with scores

### 5.3 — Reranker service (`app/services/reranker.py`)
- Cohere Rerank v3.5 integration
- Fail-open: if reranker fails, fall back to RRF scores
- Timeout: 3000ms
- Log reranker_skipped + reranker_error in debug

### 5.4 — Context budgeting (`app/services/context.py`)
- MAX_CONTEXT_TOKENS=6000, overhead=1000, max_chunks=12
- Select top chunks that fit within budget
- Format context per spec §8 template

### 5.5 — LLM service (`app/services/llm.py`)
- Anthropic Claude integration (primary)
- System prompt from spec §8
- Include one-shot abstain example
- Parse response for citations

### 5.6 — Abstain logic (`app/services/abstain.py`)
- Apply rerank threshold (0.15) when reranker succeeded
- Apply RRF threshold (0.030) when reranker failed/disabled
- Return abstain response with explanation

### 5.7 — Query endpoint (`app/routes/query.py`)
- `POST /v1/query` with request/response contracts from spec §6
- Wire together: retrieval → rerank → abstain check → context budget → LLM → citations
- Error responses: 400, 500, 502 per spec
- Debug mode: populate debug field when `debug=true`

### 5.8 — Structured logging
- One JSON log line per query (stdout)
- All required fields from spec §Logging
- Stage timings: vec, fts, merge, rerank, llm

### 5.9 — Startup reliability
- DB connection retry with backoff
- Check required tables and extensions exist
- Clear error messages on startup failure

### Definition of Done — Phase 5
- `POST /v1/query` returns correct answers with citations
- Abstain works when evidence is weak
- Reranker fail-open works (disable Cohere, query still works)
- Tenant isolation enforced (query with wrong tenant_id returns nothing)
- Latest-only enforced (deactivated versions not returned)
- Structured JSON logs emitted per query
- `make api-test` passes with tests for: happy path, abstain, fail-open, tenant isolation, error cases

---

## Phase 6: Full Ingestion + Red Team ⬚

### 6.1 — Expand test corpus
- Ingest 10-50 docs including table-heavy PDFs
- Verify ingestion at scale

### 6.2 — Expand eval questions
- Add more questions to reach 50+
- Include edge cases: table queries, cross-document, stale version queries

### 6.3 — Red team scripts (`eval/run_redteam.py`)
- Prompt injection probes
- Data exfiltration attempts (cross-tenant)
- Stale policy probes (deactivated versions)
- Output: pass/fail per probe with details

### 6.4 — Eval with full pipeline
- Run eval through the full query API (not just retrieval)
- Compare: with rerank vs without rerank (MRR improvement)
- Document results

### Definition of Done — Phase 6
- 10-50 docs ingested
- `make eval` shows acceptable Hit@K + MRR
- Rerank improves MRR vs no rerank
- `make e2e-smoke` passes
- Red team shows no tenant leakage and reasonable abstains

---

## Phase 7: Polish + V1 Done Definition ⬚

### 7.1 — Documentation
- Update ARCHITECTURE.md with final state
- Update DECISIONS.md with any new ADRs
- README.md with setup instructions

### 7.2 — Makefile completeness
- All targets from DEVELOPMENT_RULES.md work
- `make update-rules` and `make validate-rules` implemented

### 7.3 — Final V1 checklist (from spec §13)
- [ ] Ingest 10-50 docs incl. a table-heavy PDF
- [ ] Query returns citations and respects latest-only
- [ ] Reranker can fail without breaking query path
- [ ] Eval outputs Hit@K + MRR; rerank improves MRR vs no rerank
- [ ] Red team shows no tenant leakage and reasonable abstains

---

## Architecture Diagram

```mermaid
graph TB
    subgraph Docker Compose
        PG[Postgres 16 + pgvector]
        MIG[migrate - one-shot]
        QA[query-api - FastAPI]
        ING[ingest - Python CLI]
    end

    subgraph External APIs
        COH[Cohere Rerank v3.5]
        LLM[Anthropic Claude]
    end

    subgraph Storage
        ART[Local Artifacts Volume]
    end

    MIG -->|runs migrations| PG
    ING -->|extract + chunk + embed + write| PG
    ING -->|save artifacts| ART
    QA -->|read chunks + embeddings + FTS| PG
    QA -->|rerank - fail-open| COH
    QA -->|generate answer| LLM

    USER[API Client] -->|POST /v1/query| QA
    DEV[Developer] -->|CLI ingest command| ING
```

## Query Pipeline Flow

```mermaid
graph LR
    Q[Question] --> VEC[Vector Search K=50]
    Q --> FTS[FTS Search K=50]
    VEC --> RRF[RRF Merge k=60]
    FTS --> RRF
    RRF --> RR{Reranker}
    RR -->|success| RT[Apply Rerank Threshold 0.15]
    RR -->|fail-open| RF[Apply RRF Threshold 0.030]
    RT --> AB{Abstain?}
    RF --> AB
    AB -->|yes| ABSTAIN[Return Abstain Response]
    AB -->|no| CTX[Context Budgeting 6000 tokens max 12 chunks]
    CTX --> PROMPT[Build Prompt with Citations Template]
    PROMPT --> CALL_LLM[Call LLM]
    CALL_LLM --> RESP[Return Answer + Citations]
```
