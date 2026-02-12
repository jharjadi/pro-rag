# ARCHITECTURE.md — pro-rag V1

> Full spec: `plans/ragstack_rag_poc_spec_v7.md`

## Overview

Production-shaped RAG system for English-only internal knowledge base.

```
┌─────────────┐     ┌──────────────────────────────────┐
│  ingest-py   │────▶│  Postgres (pgvector + FTS)        │
│  (Python)    │     │  - tenants, documents, versions   │
└─────────────┘     │  - chunks, embeddings, fts        │
                     │  - ingestion_runs                 │
┌─────────────┐     │                                    │
│  query-api   │◀───│  HNSW vector index + GIN FTS      │
│  (Python/    │     └──────────────────────────────────┘
│   FastAPI)   │
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ LLM API (Anthropic/OpenAI)
└─────────────┘
```

## Services (V1 — All-Python)

| Service | Language | Role |
|---------|----------|------|
| **query-api** | Python / FastAPI | Online query runtime: hybrid retrieval, RRF merge, Cohere rerank, LLM prompting, citations, abstain |
| **ingest** | Python | Ingestion pipeline: extract, restructure, chunk, embed, FTS, write to Postgres |
| **postgres** | Postgres 16 + pgvector | Canonical data store: HNSW vector index, GIN FTS index |
| **migrate** | One-shot | Runs DB migrations then exits |

## Key Contracts

- **Option A**: Python ingestion writes to DB. Query runtime reads from DB. No direct coupling.
- **Tenant isolation**: Every query filters by `tenant_id`.
- **Latest-only**: Always filter `is_active = true` on `document_versions`.
- **Abstain over hallucinate**: Refuse to answer when evidence is weak.

## Decision: All-Python for V1

See `docs/DECISIONS.md` ADR-001.
