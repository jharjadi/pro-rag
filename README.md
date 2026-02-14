# pro-rag

Production-shaped RAG (Retrieval-Augmented Generation) system for English-only internal knowledge bases. Multi-tenant, citation-required, abstain-over-hallucinate.

## Architecture

```
┌─────────────┐     ┌──────────────────────────────────┐
│  ingest      │────▶│  Postgres 16 + pgvector           │
│  (Python)    │     │  HNSW vector index + GIN FTS      │
└─────────────┘     └──────────────────────────────────┘
                              ▲
┌─────────────┐               │
│  core-api-go │───────────────┘
│  (Go / Chi)  │────▶ embed-svc (question embedding)
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ Anthropic Claude (LLM)
└─────────────┘

┌─────────────┐
│  embed-svc   │  Python sidecar — sentence-transformers
│  (Flask)     │  BAAI/bge-base-en-v1.5 (768-dim)
└─────────────┘
```

| Service | Language | Role |
|---------|----------|------|
| **core-api-go** | Go / Chi | Query runtime: hybrid retrieval → RRF merge → Cohere rerank → LLM → citations |
| **embed-svc** | Python / Flask | Embedding sidecar: wraps sentence-transformers for question embedding |
| **ingest** | Python | Ingestion pipeline: extract (DOCX/PDF/HTML) → chunk → embed → FTS → write to Postgres |
| **postgres** | Postgres 16 + pgvector | Canonical data store with HNSW vector index and GIN FTS index |
| **migrate** | Shell | One-shot migration runner |

### Key Design Decisions

- **Two languages enforce the Option A contract**: Go reads from DB, Python writes to DB. No shared code across the boundary. See [ADR-004](docs/DECISIONS.md).
- **Embedding sidecar**: Same model for ingestion and query ensures vector compatibility. See [ADR-005](docs/DECISIONS.md).
- **Separate chunk_embeddings table**: Enables future re-embed migrations. See [ADR-002](docs/DECISIONS.md).

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

# 7. Start the full stack (postgres + embed-svc + core-api-go)
docker compose up -d

# 8. Open the web UI
open http://localhost:8000

# Or query via curl
curl -s -X POST http://localhost:8000/v1/query \
  -H "Content-Type: application/json" \
  -d '{
    "tenant_id": "00000000-0000-0000-0000-000000000001",
    "question": "What is the password policy?",
    "top_k": 10,
    "debug": true
  }' | python3 -m json.tool
```

## Web UI

Navigate to `http://localhost:8000` in your browser to use the chat interface. Features:

- **Chat interface** — type questions and get answers with inline citation markers
- **Example questions** — click to try pre-built queries
- **Citations panel** — shows source documents and heading paths for each citation
- **Debug toggle** — enable to see retrieval stats (vec/FTS candidates, reranker info, context tokens)
- **Tenant selector** — configurable tenant ID in the header

The UI is a single HTML file ([`web/index.html`](web/index.html)) served by the Go API. No build step required.

## API

### `GET /health`

Returns `{"status":"ok"}` when the service is healthy.

### `POST /v1/query`

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
| `make e2e-smoke` | End-to-end smoke test (13 assertions) |
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

## Domain Rules

1. **Tenant isolation** — every query filters by `tenant_id`. No cross-tenant data leakage.
2. **Latest-only** — always filter `is_active = true` on document versions. Never serve stale content.
3. **Abstain over hallucinate** — refuse to answer when evidence is weak.
4. **Citations required** — every factual claim must cite a `chunk_id`.
5. **Schema is law** — all data structures conform to the spec (`plans/ragstack_rag_poc_spec_v7.md` §3).

## Project Structure

```
pro-rag/
├── core-api-go/          # Go query runtime (Chi router)
│   ├── cmd/server/       # main.go — HTTP server with graceful shutdown
│   ├── internal/
│   │   ├── config/       # Environment variable loading
│   │   ├── db/           # pgx connection pool + startup checks
│   │   ├── handler/      # HTTP handlers (POST /v1/query)
│   │   ├── model/        # Request/response types, query log
│   │   └── service/      # Business logic (retrieval, rerank, RRF, abstain, context, LLM, citations, prompt)
│   └── tests/
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
├── web/                  # Chat UI (single HTML file, served by Go API)
├── scripts/              # Corpus generators + E2E smoke test
├── data/                 # Test corpus (generated)
├── docs/                 # ARCHITECTURE.md, DECISIONS.md, project notes
└── plans/                # Spec + implementation plan
```

## Documentation

- [`plans/ragstack_rag_poc_spec_v7.md`](plans/ragstack_rag_poc_spec_v7.md) — Full V1 specification
- [`plans/implementation-plan.md`](plans/implementation-plan.md) — Implementation plan with phase tracking
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Architecture overview
- [`docs/DECISIONS.md`](docs/DECISIONS.md) — Architectural Decision Records (ADR-001 through ADR-005)
- [`docs/project-notes/phase4a-retrieval-eval.md`](docs/project-notes/phase4a-retrieval-eval.md) — Phase 4a retrieval eval results
- [`DEVELOPMENT_RULES.md`](DEVELOPMENT_RULES.md) — Development workflow rules

## License

Private — internal use only.
