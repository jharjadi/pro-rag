# orchestrator-rules.md (pro-rag)

> Rules for orchestrator mode. Goal: coordinate complex multi-step tasks across modes effectively.

---

## BEFORE ORCHESTRATING

1) **Read project context first**
   - Read `DEVELOPMENT_RULES.md` if it exists.
   - Read `plans/ragstack_rag_poc_spec_v7.md` → `docs/ARCHITECTURE.md` → `docs/DECISIONS.md` as needed.
   - Check `docs/project-notes/` and `docs/lessons-learned/` for relevant context.

2) **Understand the full scope**
   - Break down the task into clear, ordered subtasks before starting.
   - Identify which modes each subtask requires (architect, code, debug, ask).
   - Identify dependencies between subtasks.

---

## WHEN ORCHESTRATING

3) **Plan first, execute second**
   - Create a clear task list with dependencies before delegating work.
   - Each subtask should have a clear definition of done.
   - Use the todo list to track progress across subtasks.

4) **Delegate to the right mode**
   - Architecture/design decisions → Architect mode
   - Code implementation → Code mode
   - Troubleshooting/errors → Debug mode
   - Questions/explanations → Ask mode

5) **Respect domain invariants across all subtasks**
   - Schema is law — coordinate schema changes across services.
   - Option A contract — ingestion writes, query runtime reads. No coupling.
   - Tenant isolation — verify tenant_id filtering in all DB queries.
   - Latest-only — verify is_active filtering on document_versions.
   - Abstain logic — verify thresholds are applied correctly.
   - Test coverage — ensure all subtasks include appropriate tests.
   - Clear error handling — verify error cases are handled consistently.

6) **Verify integration points**
   - When subtasks span services (query-api ↔ ingest ↔ Postgres), verify contracts.
   - Run `make integration-test` (or create it) for cross-boundary changes.
   - Ensure DB migrations are applied before testing dependent code.

---

## WORKFLOW

7) **Follow the standard workflow**
   - Plan → Architect → Implement → Test → Verify → Commit → Build → Deploy
   - Never skip testing between implementation and commit.

8) **Follow the build order (spec §11)**
   - Scaffold + DB + compose first
   - Ingest 5 real docs early
   - Implement eval harness and run it
   - Iterate chunking/retrieval until metrics are sane
   - Then finish full ingestion + redteam

9) **Capture learnings**
   - After completing a complex orchestration, capture what worked and what didn't.
   - Run: `make update-rules MSG="what you learned"`
   - Update `docs/project-notes/` with context for future tasks.

---

## OUTPUT FORMAT

When orchestrating, maintain:
- A clear task breakdown with status tracking.
- Which mode handled each subtask.
- Integration verification results.
- Final definition of done checklist.
