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
│  core-api-go │◀───│  HNSW vector index + GIN FTS      │
│  (Go / Chi)  │     └──────────────────────────────────┘
│              │────▶ embed-svc (question embedding)
│              │────▶ Cohere Rerank API (fail-open)
│              │────▶ LLM API (Anthropic Claude)
└─────────────┘

┌─────────────┐
│  embed-svc   │  Lightweight Python sidecar
│  (Flask)     │  sentence-transformers / BAAI/bge-base-en-v1.5
└─────────────┘
```

## Services

| Service | Language | Role |
|---------|----------|------|
| **core-api-go** | Go / Chi | Online query runtime: hybrid retrieval, RRF merge, Cohere rerank, LLM prompting, citations, abstain |
| **embed-svc** | Python / Flask | Embedding sidecar: wraps sentence-transformers for question embedding (used by core-api-go) |
| **ingest** | Python | Ingestion pipeline: extract, restructure, chunk, embed, FTS, write to Postgres |
| **postgres** | Postgres 16 + pgvector | Canonical data store: HNSW vector index, GIN FTS index |
| **migrate** | One-shot | Runs DB migrations then exits |

## Key Contracts

- **Option A**: Python ingestion writes to DB. Go query runtime reads from DB. No direct coupling. Two languages enforce this boundary physically.
- **Tenant isolation**: Every query filters by `tenant_id`.
- **Latest-only**: Always filter `is_active = true` on `document_versions`.
- **Abstain over hallucinate**: Refuse to answer when evidence is weak.

## Language Decision

Go for query runtime, Python for ingestion. See `docs/DECISIONS.md` ADR-004 (supersedes ADR-001).
