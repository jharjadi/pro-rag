# pro-rag

Production-shaped RAG (Retrieval-Augmented Generation) system for English-only internal knowledge bases. Multi-tenant, citation-required, abstain-over-hallucinate.

## Architecture

```
┌──────────────┐
│  Browser     │
│  (Next.js)   │──── http://localhost:3000
└──────┬───────┘
       │ BFF proxy (all API calls)
       ▼
┌──────────────┐     ┌──────────────────────────────────┐
│  core-api-go │────▶│  Postgres 16 + pgvector           │
│  (Go / Chi)  │     │  HNSW vector index + GIN FTS      │
│  :8000       │     └──────────────────────────────────┘
│              │────▶ embed-svc (question embedding)
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ Anthropic Claude (LLM)
│              │────▶ ingest-api (internal, file upload proxy)
└──────────────┘
       │ internal Docker network only
       ▼
┌──────────────┐     ┌──────────────┐
│  ingest-api  │     │  embed-svc   │
│  (FastAPI)   │     │  (Flask)     │
│  :8002       │     │  :8001       │
│  internal    │     └──────────────┘
└──────────────┘
       │
       ▼
┌──────────────┐
│  ingest      │  Python pipeline (extract → chunk → embed → write)
│  (Python)    │
└──────────────┘
```

| Service | Language | Port | Role |
|---------|----------|------|------|
| **web** | Next.js / React | 3000 | Web UI: dashboard, document management, chat, ingestion history |
| **core-api-go** | Go / Chi | 8000 | **Single API gateway**: query runtime, management APIs, ingest proxy |
| **ingest-api** | Python / FastAPI | 8002 (internal) | HTTP wrapper for ingestion pipeline (no external port) |
| **embed-svc** | Python / Flask | 8001 | Embedding sidecar: wraps sentence-transformers for question embedding |
| **ingest** | Python | — | CLI ingestion pipeline: extract (DOCX/PDF/HTML) → chunk → embed → FTS → write |
| **postgres** | Postgres 16 + pgvector | 5432 | Canonical data store with HNSW vector index and GIN FTS index |
| **migrate** | Shell | — | One-shot migration runner |

### Key Design Decisions

- **Go as single API gateway**: All external traffic routes through Go (:8000). Enables single-point auth, rate limiting, and logging in V2. See [ADR-006](docs/DECISIONS.md).
- **Two languages enforce the Option A contract**: Go reads from DB, Python writes to DB. No shared code across the boundary. See [ADR-004](docs/DECISIONS.md).
- **Embedding sidecar**: Same model for ingestion and query ensures vector compatibility. See [ADR-005](docs/DECISIONS.md).
- **Separate chunk_embeddings table**: Enables future re-embed migrations. See [ADR-002](docs/DECISIONS.md).
- **ingest-api is internal-only**: No external port. Only reachable via Go proxy on the Docker network.

## Query Pipeline

```
Question → Embed → Vector Search (K=50) ─┐
                   FTS Search (K=50) ─────┤ (parallel)
                                          ▼
                   Zero candidates? → Abstain
                                          │
                   RRF Merge (k=60) ──────┤
                   Rerank (Cohere, fail-open) ──┤
                   Threshold check → Abstain if weak
                                          │
                   Context Budget (6000 tokens, max 12 chunks)
                   Build Prompt → Call LLM
                   Parse Citations → Response
```

## Prerequisites

- Docker & Docker Compose
- Go 1.23+ (for local `go test`)
- Node.js 18+ (for local web UI development)
- Python 3.11+ with venv (for ingestion + eval)
- API keys: `ANTHROPIC_API_KEY`, `COHERE_API_KEY`

## Quickstart

```bash
# 1. Clone and configure
cp .env.example .env
# Edit .env — set ANTHROPIC_API_KEY and COHERE_API_KEY

# 2. Start database
make db-up

# 3. Run migrations + seed
make db-migrate
make db-seed

# 4. Generate test corpus (15 docs: DOCX, HTML, PDF)
make generate-corpus
make generate-corpus-expanded

# 5. Set up Python venv for ingestion
cd ingest && python -m venv .venv && .venv/bin/pip install -e ".[dev]" && cd ..

# 6. Ingest all documents
make ingest-corpus-all

# 7. Start the full stack
docker compose up -d

# 8. Open the web UI
open http://localhost:3000
```

## Web UI

The web UI is a Next.js application at `http://localhost:3000`. All API calls go through BFF proxy routes to the Go gateway at `:8000`.

### Pages

| Page | URL | Description |
|------|-----|-------------|
| **Dashboard** | `/` | System stats, recent ingestion runs, quick actions, health indicator |
| **Documents** | `/documents` | Searchable document list with pagination, active version info |
| **Document Detail** | `/documents/:id` | Version history, chunk browser with token counts, deactivate action |
| **Upload** | `/documents/new` | Drag-and-drop file upload (DOCX, PDF, HTML), async with progress polling |
| **Chat** | `/chat` | Full chat interface with citations, debug panel, abstain styling |
| **Ingestion History** | `/ingestion` | Ingestion runs table with status, duration, auto-refresh for running jobs |

### Local Development

```bash
# Install dependencies
make web-install

# Start dev server (requires core-api-go running on :8000)
make web-dev

# Production build
make web-build
```

### Docker (Production)

The web service is included in `docker-compose.yml` and starts automatically with `docker compose up -d`.

## API

All API endpoints are served by the Go gateway at `:8000`.

### Health

#### `GET /health`
Returns `{"status":"ok"}` when the service is healthy.

### Query

#### `POST /v1/query`

**Request:**
```json
{
  "tenant_id": "00000000-0000-0000-0000-000000000001",
  "question": "What is the password policy?",
  "top_k": 10,
  "debug": false
}
```

**Response (success):**
```json
{
  "answer": "According to the IT Security Policy, passwords must be at least 12 characters... [chunk:abc123]",
  "citations": [
    {
      "chunk_id": "abc123",
      "doc_id": "...",
      "title": "Acme Corp IT Security Policy",
      "version_id": "...",
      "heading_path": "Password Requirements"
    }
  ],
  "abstained": false
}
```

**Response (abstain):**
```json
{
  "answer": "I don't have enough information in the available documents to answer this question confidently.",
  "citations": [],
  "abstained": true,
  "clarifying_question": "Could you rephrase your question or provide more context?"
}
```

**Error responses:** `400` (bad request), `500` (internal error), `502` (LLM unavailable).

### Document Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/documents?tenant_id=...&page=1&limit=20&search=...` | List documents with pagination and search |
| `GET` | `/v1/documents/:id?tenant_id=...` | Document detail with version history and chunk stats |
| `GET` | `/v1/documents/:id/chunks?tenant_id=...&page=1&limit=50` | List chunks for a document |
| `POST` | `/v1/documents/:id/deactivate?tenant_id=...` | Soft-deactivate a document |

### Ingestion

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/v1/ingest` | Upload file (multipart form) — proxied to internal ingest-api |
| `GET` | `/v1/ingestion-runs?tenant_id=...&page=1&limit=20` | List ingestion runs |
| `GET` | `/v1/ingestion-runs/:id?tenant_id=...` | Get single ingestion run detail |

## Make Targets

### Database
| Target | Description |
|--------|-------------|
| `make db-up` | Start Postgres (pgvector) |
| `make db-down` | Stop all services |
| `make db-migrate` | Run SQL migrations |
| `make db-reset` | Destroy DB + recreate + migrate |
| `make db-seed` | Seed test tenant |
| `make db-psql` | Open psql shell |

### Query API (Go)
| Target | Description |
|--------|-------------|
| `make api-build` | Build core-api-go binary |
| `make api-run` | Start core-api-go via Docker Compose |
| `make api-test` | Run Go tests (`go test ./...`) |

### Web UI (Next.js)
| Target | Description |
|--------|-------------|
| `make web-install` | Install web dependencies (`npm install`) |
| `make web-dev` | Start Next.js dev server |
| `make web-build` | Build Next.js for production |

### Ingestion (Python)
| Target | Description |
|--------|-------------|
| `make ingest-run` | Run ingestion pipeline via Docker |
| `make ingest-test` | Run Python tests (`pytest`) |
| `make ingest-corpus` | Ingest 5 original test corpus docs |
| `make ingest-corpus-all` | Ingest all 15 expanded corpus docs |
| `make generate-corpus` | Generate 5 original DOCX files |
| `make generate-corpus-expanded` | Generate 10 additional docs (DOCX/HTML/PDF) |

### Testing & Evaluation
| Target | Description |
|--------|-------------|
| `make test` | Run all tests (api-test + ingest-test) |
| `make eval` | Run retrieval-only evaluation (default) |
| `make eval-retrieval` | Run retrieval-only evaluation (DB direct) |
| `make eval-full` | Run full pipeline evaluation (calls API) |
| `make e2e-smoke` | End-to-end smoke test — API only (13 assertions) |
| `make e2e-web` | E2E web integration test — upload → list → query → citations → deactivate |
| `make redteam` | Red team probes (injection/exfil/stale) |

### Meta
| Target | Description |
|--------|-------------|
| `make update-rules MSG="..."` | Capture a learning |
| `make validate-rules` | Check project consistency |

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example) for the full list with defaults.

Key settings:

| Variable | Default | Description |
|----------|---------|-------------|
| `K_VEC` | 50 | Vector search top-K |
| `K_FTS` | 50 | FTS search top-K |
| `RRF_K` | 60 | RRF merge constant |
| `ABSTAIN_RERANK_THRESHOLD` | 0.15 | Rerank score below which to abstain |
| `ABSTAIN_RRF_THRESHOLD` | 0.030 | RRF score below which to abstain (when reranker fails) |
| `MAX_CONTEXT_TOKENS` | 6000 | Max tokens in LLM context window |
| `MAX_CONTEXT_CHUNKS` | 12 | Max chunks sent to LLM |
| `RERANK_FAIL_OPEN` | true | Continue without reranker on failure |
| `INGEST_API_URL` | `http://ingest-api:8002` | Internal ingest-api URL (Go proxy target) |

## Domain Rules

1. **Tenant isolation** — every query filters by `tenant_id`. No cross-tenant data leakage.
2. **Latest-only** — always filter `is_active = true` on document versions. Never serve stale content.
3. **Abstain over hallucinate** — refuse to answer when evidence is weak.
4. **Citations required** — every factual claim must cite a `chunk_id`.
5. **Schema is law** — all data structures conform to the spec (`plans/ragstack_rag_poc_spec_v7.md` §3).

## Project Structure

```
pro-rag/
├── core-api-go/          # Go API gateway (Chi router)
│   ├── cmd/server/       # main.go — HTTP server with graceful shutdown
│   ├── internal/
│   │   ├── config/       # Environment variable loading
│   │   ├── db/           # pgx connection pool + startup checks
│   │   ├── handler/      # HTTP handlers (query, documents, ingestion, ingest proxy)
│   │   ├── model/        # Request/response types, query log
│   │   └── service/      # Business logic (retrieval, rerank, RRF, abstain, context, LLM, citations, prompt)
│   └── tests/
├── web/                  # Next.js web UI
│   ├── app/              # App Router pages (dashboard, documents, chat, ingestion)
│   │   ├── api/          # BFF proxy routes (all traffic → Go :8000)
│   │   ├── chat/         # Chat page
│   │   ├── documents/    # Document list, detail, upload pages
│   │   └── ingestion/    # Ingestion history page
│   ├── components/       # Shared React components (sidebar)
│   └── lib/              # API client, types, utilities
├── ingest-api/           # Python ingest HTTP wrapper (FastAPI, internal-only)
├── embed-svc/            # Python embedding sidecar (Flask)
├── ingest/               # Python ingestion pipeline
│   ├── ingest/
│   │   ├── extract/      # DOCX, PDF, HTML extractors
│   │   ├── chunk/        # Structure-aware chunker + metadata
│   │   ├── embed/        # Batch embedding (sentence-transformers)
│   │   ├── fts/          # FTS tsvector generation
│   │   └── db/           # DB writer (transactional)
│   └── tests/
├── migrations/           # 8 SQL migrations + seed data
├── migrate/              # Migration runner script
├── eval/                 # Evaluation harness + red team probes
│   ├── questions.jsonl   # 92 eval questions
│   ├── run_eval.py       # Hit@K, MRR, abstain rate, latency
│   └── run_redteam.py    # Injection, exfil, stale probes
├── scripts/              # Corpus generators + E2E smoke test
├── data/                 # Test corpus (generated)
├── docs/                 # ARCHITECTURE.md, DECISIONS.md, project notes
└── plans/                # Spec + implementation plan
```

## Documentation

- [`plans/ragstack_rag_poc_spec_v7.md`](plans/ragstack_rag_poc_spec_v7.md) — Full V1 specification
- [`plans/implementation-plan.md`](plans/implementation-plan.md) — Implementation plan with phase tracking
- [`plans/web-ui-spec.md`](plans/web-ui-spec.md) — Web UI implementation spec
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Architecture overview
- [`docs/DECISIONS.md`](docs/DECISIONS.md) — Architectural Decision Records (ADR-001 through ADR-006)
- [`docs/project-notes/phase4a-retrieval-eval.md`](docs/project-notes/phase4a-retrieval-eval.md) — Phase 4a retrieval eval results
- [`DEVELOPMENT_RULES.md`](DEVELOPMENT_RULES.md) — Development workflow rules

## License

Private — internal use only.
