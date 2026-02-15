# ARCHITECTURE.md — pro-rag V1

> Full spec: `plans/ragstack_rag_poc_spec_v7.md`
> Web UI spec: `plans/web-ui-spec.md`

## Overview

Production-shaped RAG system for English-only internal knowledge base with a web UI for document management, chat, and ingestion monitoring.

```
┌─────────────┐
│  web (Next.js)│  Browser UI: dashboard, documents, chat, ingestion
│  :3000       │  BFF proxy — all calls go through Next.js API routes
└──────┬───────┘
       │ (all API calls)
       ▼
┌──────────────┐     ┌──────────────────────────────────┐
│  core-api-go │◀───│  Postgres (pgvector + FTS)        │
│  (Go / Chi)  │     │  - tenants, documents, versions   │
│  :8000       │     │  - chunks, embeddings, fts        │
│  SINGLE API  │     │  - ingestion_runs                 │
│  GATEWAY     │     │                                    │
│              │────▶│  HNSW vector index + GIN FTS      │
│              │     └──────────────────────────────────┘
│              │────▶ embed-svc (question embedding)
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ LLM API (Anthropic Claude)
│              │────▶ ingest-api (internal, file upload proxy)
└──────────────┘

┌─────────────┐     ┌─────────────┐
│  embed-svc   │     │  ingest-api  │  Internal only (no external port)
│  (Flask)     │     │  (FastAPI)   │  Wraps ingest pipeline
│  :8001       │     │  :8002       │  Async file upload + ingestion
└─────────────┘     └─────────────┘

┌─────────────┐
│  ingest CLI  │  On-demand CLI for batch ingestion
│  (Python)    │
└─────────────┘
```

## Services

| Service | Language | Port | Role |
|---------|----------|------|------|
| **web** | Next.js 15 / TypeScript | 3000 | Web UI: dashboard, documents, chat, ingestion history. BFF proxy to Go API. |
| **core-api-go** | Go / Chi | 8000 | **Single API gateway.** Query runtime + document management APIs + ingest proxy. All external traffic routes here. |
| **embed-svc** | Python / Flask | 8001 | Embedding sidecar: wraps sentence-transformers for question embedding |
| **ingest-api** | Python / FastAPI | 8002 (internal) | HTTP wrapper around ingestion pipeline. Async file upload. Internal only — no external port. |
| **ingest** | Python CLI | - | Batch ingestion pipeline: extract, chunk, embed, FTS, write to Postgres |
| **postgres** | Postgres 16 + pgvector | 5432 | Canonical data store: HNSW vector index, GIN FTS index |
| **migrate** | One-shot | - | Runs DB migrations then exits |

## Key Contracts

- **Option A**: Python ingestion writes to DB. Go query runtime reads from DB. No direct coupling. Two languages enforce this boundary physically.
- **Go as single gateway**: All external traffic routes through Go (:8000). The ingest-api is internal-only. This gives one entry point for auth, rate limiting, logging, and audit in V2.
- **BFF proxy**: All browser API calls go through Next.js API routes → Go. No direct browser→Go or browser→Python calls. Eliminates CORS.
- **Tenant isolation**: Every query filters by `tenant_id`.
- **Latest-only**: Always filter `is_active = true` on `document_versions`.
- **Abstain over hallucinate**: Refuse to answer when evidence is weak.

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

Go for query runtime, Python for ingestion. See `docs/DECISIONS.md` ADR-004 (supersedes ADR-001).
