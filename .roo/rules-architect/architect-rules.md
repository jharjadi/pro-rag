# architect-rules.md (pro-rag)

> Rules for architect/planning mode. Goal: sound design decisions grounded in project constraints.

---


## ARCHITECT MODE (OPUS ALLOWED) — OUTPUT LIMITS

- Opus 4.6 is allowed in architect mode.
- Keep outputs short and executable:
  - Max ~1 page unless explicitly requested
  - One recommended approach (no option menus)
  - Concrete: endpoints, tables, files, tests, DoD
- Do not broaden scope beyond the current roadmap slice.
- Do not rewrite existing architecture unless required by the slice.
- Prefer ADRs that are 5–10 lines each.


## BEFORE DESIGNING

1) **Read project context first**
   - Read `DEVELOPMENT_RULES.md` if it exists.
   - Read `plans/ragstack_rag_poc_spec_v7.md` for the full V1 spec.
   - Read `docs/ARCHITECTURE.md` → `docs/DECISIONS.md` in that order.
   - Check `docs/project-notes/` and `docs/lessons-learned/` for relevant context.

2) **Understand current state**
   - Review existing code structure before proposing changes.
   - Check what's already implemented vs. what's planned.
   - Don't redesign what already works unless there's a clear reason.

---

## WHEN DESIGNING

3) **Keep it simple (MVP-first)**
   - Prefer the simplest solution that meets the requirement.
   - Avoid over-engineering — this is a V1 POC.
   - Design for the current scale (10–50 docs, 1k–10k chunks), not hypothetical future scale.

4) **Respect domain invariants**
   - Schema is law — any schema change must be documented in `docs/DECISIONS.md`.
   - Option A contract: Python writes DB, query runtime reads DB.
   - Tenant isolation: every query filters by `tenant_id`.
   - Latest-only: always filter `is_active = true`.
   - Abstain over hallucinate: wrong answers are worse than no answers.
   - Test coverage is required for critical paths.
   - Clear error handling and user feedback are non-negotiable.

5) **Document decisions**
   - All architectural decisions must be recorded in `docs/DECISIONS.md`.
   - Include: what was decided, why, what alternatives were considered.

6) **Plan for testability**
   - Every design should include how it will be tested.
   - Identify which Make targets need to be created or updated.

---

## ARCHITECTURE CONSTRAINTS (V1)

- **query-api** (Python/FastAPI): Online query runtime — hybrid retrieval, RRF merge, Cohere rerank (fail-open), LLM prompting, citations, abstain logic
- **ingest** (Python): Ingestion pipeline — extract, restructure, chunk, embed, FTS, write to Postgres
- **Postgres** (pgvector + FTS): Canonical data store — HNSW vector index, GIN FTS index
- **No UI in V1** — API-only, eval scripts for validation
- **Docker Compose**: Local dev stack with migrate one-shot service

---

## OUTPUT FORMAT

When proposing architecture changes, include:
- Files that will be created or modified.
- Database migrations needed (if any).
- Impact on existing tests.
- Definition of done.
