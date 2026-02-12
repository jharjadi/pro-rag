# code-rules.md (pro-rag)

> Repo-law for AI-assisted development (Roo/Claude) and humans.
> Goal: ship fast **without** breaking trust: test-first, quality-first.
> Secondary goal: control cost by minimizing context, diffs, and unnecessary model escalation.

---

## MODEL COST POLICY (MANDATORY)

**Default model:** Sonnet 4.5  
**Opus 4.6 is exception-only.** Lower models are allowed for boilerplate/summaries.

### Opus escalation gate (must satisfy ALL)
Only recommend switching to **Opus 4.6** if:
1) The task is blocked after **one** complete Sonnet attempt (plan → implement → test).
2) The issue requires cross-module reasoning (query-api ↔ ingest ↔ DB) or non-trivial architecture tradeoffs.
3) The next step cannot be reduced to a small, testable change.

If recommending Opus, the agent must:
- Say exactly: **"Recommend switching to Opus 4.6 for this step"**
- Give **1–3** reasons
- Propose a cheaper alternative first (reduce scope/context; smaller diff)

### Token minimization rules (cost control)
- **No full-repo context** by default. Open only files needed for the task.
- **Max 3 files changed per iteration** unless explicitly approved.
- Keep responses concise; avoid giant diffs or restating large files.
- No refactors unless required to satisfy spec/tests.

---

## IMPORTANT WORKFLOW RULES (ALWAYS)

1) **Read project rules first**
- If `DEVELOPMENT_RULES.md` exists in the project root, read it **FIRST** before starting any task.
- Then read: `plans/ragstack_rag_poc_spec_v7.md` (V1 spec) → `docs/ARCHITECTURE.md` → `docs/DECISIONS.md` (if present).

2) **No untested commits**
- NEVER commit code before running the relevant tests and a minimal smoke check.

3) **Use repo commands, not freestyle**
- ALWAYS use project scripts or Make targets (e.g., `make api-test`).
- If a needed command/target doesn't exist, **add it** (do not treat raw commands as the official path).

4) **Follow the workflow**
**Test → Verify → Commit → Build → Deploy**

5) **Test locally (service-specific) before commit**
At minimum:
- Query API changes: `make api-test`
- Ingestion changes: `make ingest-test`
- DB/schema changes: `make db-migrate` then `make api-test`
- Cross-boundary changes: `make integration-test` (or add it if missing)
- Eval changes: `make eval`

6) **After fixing a mistake, capture the learning**
Run:
`make update-rules MSG="what you learned"`

7) **Check project context before major work**
- Check `docs/project-notes/` before major changes.
- Check `docs/lessons-learned/` to avoid repeating mistakes.

8) **Validate rules before major changes or deployments**
Run:
`make validate-rules`

9) **No silent refactors**
- For multi-file refactors, state a plan first: files touched, behavior changes, tests updated.
- Keep diffs minimal and reviewable.
- If refactor is not required for the task, do not do it.

---

## PRO-RAG DOMAIN RULES

These are non-negotiable. They enforce the product promise.

1) **Schema is law**
- All data structures MUST conform to the defined schema (`plans/ragstack_rag_poc_spec_v7.md` §3).
- Any schema change requires:
  - DB migration
  - Update `docs/DECISIONS.md`

2) **Option A contract**
- Python ingestion writes to DB. Query runtime reads from DB.
- No direct coupling between ingestion and query services.

3) **Tenant isolation**
- Every DB query must filter by `tenant_id`.
- No cross-tenant data leakage — ever.

4) **Latest-only**
- Always join `document_versions` and filter `is_active = true`.
- Never serve stale content.

5) **Abstain over hallucinate**
- The system must refuse to answer when evidence is weak.
- Apply rerank threshold when reranker succeeds; RRF threshold when it fails.

6) **Citations required**
- Every factual claim must cite a `chunk_id`.
- No uncited assertions in generated answers.

7) **Test coverage for critical paths**
- Retrieval pipeline, abstain logic, tenant filtering must have tests.
- No commits that reduce test coverage without justification.

8) **Clear error handling**
- All error cases must be handled explicitly.
- Use the defined HTTP error response contracts (400, 500, 502).

9) **Structured logging**
- One JSON log line per query with all required fields (see spec §Logging).
- Include stage timings for performance analysis.

---

## START-OF-TASK CHECKLIST (AGENTS + HUMANS)

Before writing code, explicitly answer:
- What files will change? (max 3 unless approved)
- What tests will be run?
- What is the definition of done?
- Does this touch schema/critical paths/integrations? If yes, list impacts.
- Is Opus required? If yes, state the escalation gate criteria.
