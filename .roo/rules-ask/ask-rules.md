# ask-rules.md (pro-rag)

> Rules for ask/explanation mode. Goal: accurate, context-aware answers grounded in project reality.

---

## BEFORE ANSWERING

1) **Read project context first**
   - Read `DEVELOPMENT_RULES.md` if it exists.
   - Read `plans/ragstack_rag_poc_spec_v7.md` → `docs/ARCHITECTURE.md` → `docs/DECISIONS.md` as needed.
   - Check `docs/project-notes/` and `docs/lessons-learned/` for relevant context.

2) **Ground answers in the codebase**
   - Always check actual code, schema, and configuration before answering questions about how the system works.
   - Do not speculate — read the source of truth.

---

## WHEN ANSWERING

3) **Be precise and reference files**
   - Cite specific files, line numbers, and code when explaining behavior.
   - If the answer depends on configuration, show the actual config values.

4) **Respect domain rules**
   - Schema is law — answers about data must align with the spec (`plans/ragstack_rag_poc_spec_v7.md` §3).
   - Test coverage is required — don't suggest workflows that skip testing.
   - Clear error handling — don't suggest approaches that hide errors.

5) **Acknowledge unknowns**
   - If something isn't documented or implemented yet, say so clearly.
   - Don't fabricate answers about features that don't exist.

---

## DOMAIN AWARENESS

- **pro-rag** is a production-shaped RAG (Retrieval-Augmented Generation) system for an English-only internal company knowledge base.
- Core concepts: document ingestion (extract → chunk → embed → FTS), hybrid retrieval (vector + FTS → RRF merge → rerank), LLM answer generation with citations, abstain logic.
- Multi-tenant by design (`tenant_id` on all tables).
- V1 is all-Python: FastAPI query runtime + Python ingestion pipeline + Postgres (pgvector + FTS).
- Option A contract: Python ingestion writes to DB, query runtime reads from DB.
