# debug-rules.md (pro-rag)

> Rules for debugging mode. Goal: systematic diagnosis, evidence-based fixes.

---

## BEFORE DEBUGGING

1) **Read project context first**
   - Read `DEVELOPMENT_RULES.md` if it exists.
   - Check `docs/lessons-learned/` for known issues and past fixes.
   - Check `docs/project-notes/` for relevant context.

2) **Reproduce before fixing**
   - Always reproduce the error before attempting a fix.
   - Capture the exact error message, stack trace, or unexpected behavior.

3) **Use project debug commands**
   - `make debug-last-error` → extract last error with full context
   - `make debug-failing-tests` → analyze test failures
   - `make docker-debug` → analyze Docker container issues

---

## DURING DEBUGGING

4) **Isolate the problem**
   - Narrow down to the smallest reproducible case.
   - Check if the issue is in: query-api (Python/FastAPI), ingest (Python), DB (Postgres + pgvector), or infrastructure (Docker Compose).

5) **Check logs systematically**
   - Docker logs: `docker compose logs <service>`
   - Application logs: check stdout/stderr — query runtime emits structured JSON logs.
   - Check ingestion_runs table for pipeline failures (`status = 'failed'`, `error` field).

6) **Verify assumptions**
   - Don't assume — verify DB state, environment variables, network connectivity.
   - Use `make db-migrate` to ensure schema is up to date.
   - Check tenant_id filtering is correct (common source of "no results" bugs).

---

## AFTER FIXING

7) **Verify the fix**
   - Run the relevant tests: `make api-test`, `make ingest-test`, or `make integration-test`.
   - Confirm the original error no longer occurs.

8) **Capture the learning**
   - Run: `make update-rules MSG="what you learned"`
   - Add to `docs/lessons-learned/` if it's a recurring pattern.

---

## DOMAIN AWARENESS

- Remember: schema is law, tenant isolation is mandatory, abstain over hallucinate.
- If a bug involves the retrieval pipeline — check vector search, FTS, RRF merge, reranker, and abstain thresholds independently.
- If a bug involves ingestion — check ingestion_runs table, chunk token counts, embedding dimensions, FTS tsvector generation.
- If a bug involves cross-tenant data — this is a **critical security issue**. Fix immediately and add a regression test.
