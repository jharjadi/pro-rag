# ARCHITECTURE.md — pro-rag V1

> Full spec: `plans/ragstack_rag_poc_spec_v7.md`
> Orchestration spec: `plans/implementation-spec-go-orchestrated-ingestion-v2.3.md`
> Web UI spec: `plans/web-ui-spec.md`

## Overview

Production-shaped RAG system for English-only internal knowledge base with a web UI for document management, chat, and ingestion monitoring. Go orchestrates all ingestion (file upload, dedup, job creation) and delegates content processing to a Python worker.

```
┌─────────────┐
│  web (Next.js)│  Browser UI: dashboard, documents, chat, ingestion
│  :3000       │  BFF proxy — all calls go through Next.js API routes
└──────┬───────┘
       │ (all API calls)
       ▼
┌──────────────┐     ┌──────────────────────────────────┐
│  core-api-go │◀───│  Postgres (pgvector + FTS)        │
│  (Go / Chi)  │     │  - tenants, users, documents      │
│  :8000       │     │  - document_versions, chunks       │
│  SINGLE API  │     │  - embeddings, fts, ingestion_runs │
│  GATEWAY +   │     │                                    │
│  ORCHESTRATOR│────▶│  HNSW vector index + GIN FTS      │
│              │     └──────────────────────────────────┘
│              │────▶ embed-svc (question embedding)
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ LLM API (Anthropic Claude)
│              │────▶ ingest-worker (internal, HTTP delegation)
└──────────────┘
       │
       │ POST /internal/process (Bearer token)
       ▼
┌──────────────┐
│ ingest-worker│  Internal only (no external port)
│ (Flask)      │  Bounded concurrency (ThreadPoolExecutor)
│ :8002        │  Runs pipeline: extract → chunk → embed → FTS → write
└──────────────┘

┌─────────────┐
│  embed-svc   │  Embedding sidecar (sentence-transformers)
│  (Flask)     │  Used by Go for query embedding
│  :8001       │
└─────────────┘

┌─────────────┐
│  ingest CLI  │  On-demand CLI for batch ingestion (direct DB writes)
│  (Python)    │
└─────────────┘
```

## Services

| Service | Language | Port | Role |
|---------|----------|------|------|
| **web** | Next.js 15 / TypeScript | 3000 | Web UI: dashboard, documents, chat, ingestion history. BFF proxy to Go API. |
| **core-api-go** | Go / Chi | 8000 | **Single API gateway + ingestion orchestrator.** Accepts uploads, computes SHA-256, creates documents/runs, delegates to worker. Query runtime + document management APIs. All external traffic routes here. |
| **ingest-worker** | Python / Flask | 8002 (internal) | Ingestion worker. Receives jobs from Go via HTTP. Runs pipeline (extract → chunk → embed → FTS → write). Bounded concurrency (max 3 jobs). Heartbeats to DB. Internal only — no external port. |
| **embed-svc** | Python / Flask | 8001 | Embedding sidecar: wraps sentence-transformers for question embedding |
| **ingest** | Python CLI | - | Batch ingestion pipeline: extract, chunk, embed, FTS, write to Postgres (direct DB path) |
| **postgres** | Postgres 16 + pgvector | 5432 | Canonical data store: HNSW vector index, GIN FTS index |
| **migrate** | One-shot | - | Runs DB migrations then exits |

## Write Ownership (ADR-007)

| Table | Go writes | Python writes |
|-------|-----------|---------------|
| `documents` | ✅ (INSERT ON CONFLICT) | — |
| `ingestion_runs` | ✅ (status=queued) | ✅ (status transitions, heartbeat, stats) |
| `document_versions` | — | ✅ (content_hash, version creation) |
| `chunks` | — | ✅ |
| `chunk_embeddings` | — | ✅ |
| `chunk_fts` | — | ✅ |

## Ingestion Flow (spec v2.3 §4.1)

1. **Upload** → Go accepts multipart file, streams to disk, computes SHA-256
2. **Dedup** → `source_uri = upload://sha256:<hash>`, INSERT ON CONFLICT on `(tenant_id, source_uri)`
3. **Content dedup** → Check active version's `content_hash`; skip if identical
4. **Run creation** → Go creates `ingestion_runs` row with `status=queued`
5. **Delegation** → Go POSTs job payload to `ingest-worker` with Bearer token
6. **Processing** → Worker transitions to `running`, runs pipeline, writes content tables
7. **Completion** → Worker updates run to `succeeded`/`failed`

## Key Contracts

- **Write ownership split**: Go writes orchestration tables (documents, runs). Python writes content tables (versions, chunks, embeddings, FTS). See ADR-007.
- **Go as single gateway + orchestrator**: All external traffic routes through Go (:8000). Go orchestrates ingestion (upload, dedup, job creation) and delegates processing to the worker.
- **Internal auth**: Go and worker share an HMAC token (`INTERNAL_AUTH_TOKEN`). Worker validates Bearer token on every request.
- **Crash guard**: Go marks stale queued (>1h) and running (>15min no heartbeat) runs as failed on startup.
- **BFF proxy**: All browser API calls go through Next.js API routes → Go. No direct browser→Go or browser→Python calls. Eliminates CORS.
- **Tenant isolation**: Every query filters by `tenant_id`.
- **Latest-only**: Always filter `is_active = true` on `document_versions`.
- **Abstain over hallucinate**: Refuse to answer when evidence is weak.
- **Content-addressed dedup**: Same file content always maps to same document via `upload://sha256:<hash>`.

## Shared Volumes

| Volume | Go access | Worker access | Purpose |
|--------|-----------|---------------|---------|
| `uploads` | read/write | read-only | Raw uploaded files |
| `artifacts` | — | read/write | Extracted block artifacts (JSON) |

## Web UI Pages

| Page | Route | Description |
|------|-------|-------------|
| Dashboard | `/` | Stats cards, recent ingestion runs, quick actions, health indicator |
| Documents | `/documents` | Table with search, filter, pagination |
| Document Detail | `/documents/:id` | Metadata, versions, chunk browser with token visualization |
| Upload | `/documents/new` | Drag-and-drop file upload with async polling |
| Chat | `/chat` | Full chat interface with citations, debug panel, abstain styling |
| Ingestion | `/ingestion` | Run history with auto-refresh for running jobs |

## Language Decision

Go for query runtime + ingestion orchestration, Python for content processing. See `docs/DECISIONS.md` ADR-004, ADR-007.
