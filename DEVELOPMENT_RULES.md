# DEVELOPMENT_RULES.md — pro-rag

## HYPERVELOCITY PRINCIPLES

This repo follows hypervelocity development: rapid, closed-loop iteration where
tests + repeatable CLI commands define correctness.

**If any rule here conflicts with `DEVELOPMENT_RULES.md`, `DEVELOPMENT_RULES.md` wins.**


### Core Principles
1. Closed-loop development (fast feedback)
2. Verification defines correctness (tests are truth)
3. CLI-first everything (no "magic UI steps")
4. Makefile-first (predictable interface for humans + agents)
5. Production parity (Postgres + pgvector locally, same schemas/migrations)
6. Observability by default (structured JSON logs, clear failure modes)

---

## AI / AGENT OPERATING RULES
If using Roo/Claude, also follow `.roo/rules-code/code-rules.md`.
- DEVELOPMENT_RULES.md is repo workflow law (Makefile/test/migrations).
- code-rules.md is agent procedure (start-of-task checklist, cost controls, minimal diffs).
If there is any conflict: **DEVELOPMENT_RULES.md wins.**


## CRITICAL RULES — NEVER VIOLATE

### 0) Makefile-First 🔴 CRITICAL
**Do not run "official" workflows via raw commands.**
If a Make target doesn't exist, add it.

✅ DO:
- `make db-up`
- `make db-migrate`
- `make api-test`
- `make ingest-test`
- `make test`
- `make eval`
- `make e2e-smoke`

❌ DON'T:
- run ad-hoc `pytest ...` as the official path
- run ad-hoc docker commands as the official path

Exception: debugging, or creating new Make targets.

### 1) Test Before Commit
Before any commit:
- run relevant unit tests for the changed service
- run at least one smoke path if behavior changed

### 2) Schema/Migrations Discipline
- No manual DB edits
- Every schema change requires a migration
- Migration must be runnable locally via Makefile

### 3) No Silent Refactors
Multi-file changes require:
- a brief plan (files touched, behavior change)
- tests updated/added
- minimal diff approach

---

## PRO-RAG DOMAIN LAW

### Domain-Specific Rules

Core principles:
- **Schema is law** — all data structures must conform to the defined schema (see `plans/ragstack_rag_poc_spec_v7.md` §3)
- **Option A contract** — Python writes to DB, query runtime reads from DB. No direct coupling between ingestion and query services.
- **Tenant isolation** — every query must filter by `tenant_id`. No cross-tenant data leakage.
- **Latest-only** — always filter `is_active = true` on `document_versions`. Never serve stale content.
- **Abstain over hallucinate** — the system must refuse to answer when evidence is weak. Wrong answers are worse than no answers.
- **Citations required** — every factual claim in a response must cite a chunk_id.
- Test coverage for critical paths (retrieval pipeline, abstain logic, tenant filtering)
- Clear error handling and structured error responses
- Ingestion runs must be tracked (`ingestion_runs` table)

---

## LOCAL DEV WORKFLOW

### Start dependencies
- `make db-up`

### Apply migrations
- `make db-migrate`

### Run services
- `make api-run` (query runtime)
- `make ingest-run` (ingestion pipeline)

### Tests
- `make test` (all tests)
- `make api-test` (query runtime tests)
- `make ingest-test` (ingestion pipeline tests)
- `make eval` (evaluation harness — Hit@K, MRR, abstain rate)
- `make e2e-smoke` (smoke tests)

---

## DEPLOYMENT

- V1 is **Docker Compose local-only** (dev tool, no auth)
- `migrate` one-shot service runs migrations then exits
- core-api retries DB and checks required tables/extensions
- V2 will add container registry + deploy targets

---

## LEARNING SYSTEM
After mistakes/insights:
- `make update-rules MSG="what you learned"`
Before major changes:
- `make validate-rules`
