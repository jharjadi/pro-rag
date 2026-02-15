# PR Checklist: Implement Go-Orchestrated Ingestion (Spec v2.3)

This is a task-by-task, review-ready checklist to implement the **Go-orchestrated, queue-ready ingestion** described in `implementation-spec-go-orchestrated-ingestion-v2.3.md`.

Assumptions:
- Public entrypoint: `core-api-go` (Go) at `:8000`
- Internal services: `ingest-api` and/or `ingest-worker` (Python) not publicly exposed
- Postgres is shared
- Dev queue uses LocalStack SQS (or equivalent)
- Raw uploads stored on a shared Docker volume mounted into Go + Python

---

## 0) Pre-flight: Confirm invariants (must pass in review)
- [ ] Tenant isolation: tenant_id derived from JWT; never accepted from browser payload
- [ ] Latest-only: runtime serves only `document_versions.is_active=true`
- [ ] Abstain: unchanged; no evidence → abstain
- [ ] Citations: unchanged; citations only from retrieved chunks/active versions
- [ ] DB writes: Go writes orchestration tables only (as per spec v2.3); Python writes content tables + run status updates

---

## 1) Database migrations

### 1.1 Users & auth tables (if not already present)
- [ ] Create `users` table (tenant_id, user_id, email, password_hash, role, is_active, created_at, updated_at)
- [ ] Unique index: `(tenant_id, email)`
- [ ] Seed first admin user strategy (migration seed or bootstrap endpoint)

### 1.2 Ingestion orchestration tables
- [ ] Ensure `documents` table supports:
  - `tenant_id`
  - `source_type` (upload|s3)
  - `source_uri` (upload://sha256:<hash> for uploads; s3://bucket/key for S3)
  - `title`, `created_at`, `updated_at`
- [ ] Ensure unique index: `(tenant_id, source_uri)` (required for get-or-create)
- [ ] Ensure `ingestion_runs` supports:
  - `run_id` (uuid PK)
  - `tenant_id`
  - `doc_id`
  - `status` (queued|running|succeeded|failed)
  - `created_at`, `updated_at`, `started_at`, `finished_at`
  - `config` JSONB and `stats` JSONB and `error` text
- [ ] Indexes:
  - `ingestion_runs(tenant_id, created_at DESC)`
  - `ingestion_runs(run_id)` if not PK
  - `ingestion_runs(status, updated_at)` (for crash guard scanning)

### 1.3 Content tables (should already exist)
- [ ] Confirm `document_versions` has:
  - `content_hash`
  - `is_active` boolean
- [ ] Partial unique index: one active version per doc:
  - `UNIQUE (doc_id) WHERE is_active=true`

---

## 2) Docker compose / dev infrastructure

### 2.1 Shared volumes
- [ ] Add shared volumes:
  - `/data/uploads` mounted into Go and Python worker containers
  - `/data/artifacts` mounted into Python containers

### 2.2 Queue (dev)
Choose one:
- [ ] LocalStack SQS: create queue + DLQ, wire env vars
- [ ] Redis queue: ensure spec-compatible payload and semantics

If SQS/LocalStack:
- [ ] Queue URLs env:
  - `SQS_INGEST_QUEUE_URL`
  - `SQS_INGEST_DLQ_URL`
  - `AWS_REGION`
  - `AWS_ENDPOINT_URL` (LocalStack)
- [ ] LocalStack init script creates queue and DLQ

### 2.3 Network isolation
- [ ] Do not publish Python ports publicly
- [ ] Go is the only published API port

---

## 3) Go: Auth + RBAC (UI-only control plane)

### 3.1 JWT issuance & verification
- [ ] Implement `/v1/auth/login` (password auth; returns JWT as httpOnly cookie preferred)
- [ ] Middleware verifies JWT and attaches principal:
  - `{tenant_id, user_id, role}`

### 3.2 RBAC gates
- [ ] `user`: upload + view runs/docs
- [ ] `admin`: manage users, sources, trigger sync (if sources implemented)

### 3.3 AUTH_ENABLED flag
- [ ] Default `AUTH_ENABLED=true` in main compose profile
- [ ] Optional `AUTH_ENABLED=false` only in a dedicated dev profile (explicit)

---

## 4) Go: Upload ingestion orchestration

### 4.1 Endpoint: `POST /v1/ingest` (multipart)
- [ ] Parse multipart streaming (no full buffering)
- [ ] Compute SHA-256 while streaming to disk using `io.TeeReader`
- [ ] Construct upload identity:
  - `source_uri = "upload://sha256:" + hex(sha256)`
- [ ] Persist file:
  - `/data/uploads/<tenant_id>/<run_id>/<original_filename>`
  - `upload_uri = file:///data/uploads/<tenant_id>/<run_id>/<original_filename>`

### 4.2 Documents get-or-create (transaction)
- [ ] `INSERT documents ... ON CONFLICT (tenant_id, source_uri) DO UPDATE ... RETURNING doc_id`
- [ ] Ensure doc row created even for duplicates (safe)

### 4.3 Run creation (transaction)
- [ ] Insert `ingestion_runs` with:
  - `status='queued'`
  - `config` includes created_by_user_id, upload_uri, title, original_filename, content_type, source_type='upload'
- [ ] Return `202 {run_id, status:"queued"}`

### 4.4 Dedup short-circuit (optional but recommended)
- [ ] If a document exists and has an **active version** with same hash, return `200/202` with:
  - `status="skipped"`
  - `reason="duplicate content"`
  - `doc_id` and a run_id (either reuse existing or create a cheap “skipped” run)

### 4.5 Enqueue job
- [ ] Publish message to SQS:
```json
{
  "job_type": "upload_ingest",
  "run_id": "...",
  "tenant_id": "...",
  "user_id": "...",
  "doc_id": "...",
  "source_uri": "upload://sha256:...",
  "upload_uri": "file:///data/uploads/...",
  "title": "...",
  "received_at": "..."
}
```
- [ ] On publish failure:
  - mark run `failed` with error
  - return 500 (or 202 failed depending on UI expectations)

---

## 5) Go: Ingestion run read endpoints

### 5.1 `GET /v1/ingestion-runs/:run_id`
- [ ] Read from Postgres
- [ ] Ensure tenant filter enforced
- [ ] Return status/config/stats/error

### 5.2 `GET /v1/ingestion-runs`
- [ ] Filter by tenant, order by created_at desc
- [ ] Paginate (limit/offset)

---

## 6) Go: Crash guard (startup + periodic)

### 6.1 Startup crash guard
- [ ] On Go startup, scan for stale runs and mark failed:
  - queued stale: `now - created_at > QUEUED_TTL`
  - running stale: `now - updated_at > RUNNING_TTL`
- [ ] Ensure “mark failed” only if still in queued/running

### 6.2 Periodic crash guard (optional)
- [ ] Background ticker to run same scan every N minutes

---

## 7) Python: ingest-worker (queue consumer)

### 7.1 Queue poll loop
- [ ] Poll SQS with long polling
- [ ] Max concurrency (semaphore) and max inflight messages

### 7.2 Run transition (atomic)
- [ ] Attempt transition queued → running:
  - `UPDATE ingestion_runs SET status='running', started_at=COALESCE(started_at, now()) WHERE run_id=? AND tenant_id=? AND status='queued'`
- [ ] If 0 rows affected:
  - If run already succeeded/failed: ACK and drop
  - If run already running: do not duplicate work (ACK or extend visibility based on policy)

### 7.3 Processing pipeline
- [ ] Read file from `upload_uri` (local FS in V1)
- [ ] Extract → chunk → embed → FTS → write DB
- [ ] Content writes:
  - Insert new `document_versions` with `content_hash`, `is_active=true`
  - Deactivate old active version(s) in same transaction
  - Insert chunks + embeddings + fts rows

### 7.4 Update run terminal status
- [ ] On success:
  - `status='succeeded'`, `finished_at=now()`, stats JSONB populated
- [ ] On failure:
  - `status='failed'`, `finished_at=now()`, `error` stable string
- [ ] **ACK only after DB update**
- [ ] If DB update fails: do NOT ACK → allow retry

### 7.5 Heartbeats
- [ ] Update `ingestion_runs.updated_at` periodically during long jobs (milestones)

### 7.6 Cleanup (optional)
- [ ] After success, optionally delete raw upload file (or leave for Go cleanup job)

---

## 8) UI changes (Next.js)

### 8.1 Upload screen
- [ ] Upload to Go `/v1/ingest`
- [ ] Receive `run_id` and show progress

### 8.2 Runs screen
- [ ] Poll `/v1/ingestion-runs/:run_id` until terminal
- [ ] Display config + stats gracefully (ignore unknown keys)

### 8.3 Documents screen
- [ ] Show documents that have at least one active version by default
- [ ] Show `source_type` and `source_uri` columns (optional)

### 8.4 Login + session
- [ ] Login form calls `/v1/auth/login`
- [ ] Store token as httpOnly cookie preferred
- [ ] Role-gate admin pages

---

## 9) Logging / observability

### Go logs
- [ ] Single JSON log per upload: tenant_id, user_id, run_id, doc_id, filename, size_bytes, sha256, upload_uri, queue_message_id

### Python logs
- [ ] Single JSON log per job stage: run_id, tenant_id, status_transition, durations, chunks_count, error

---

## 10) Tests (must-have)

### Unit tests
- [ ] Go: streaming hash + save-to-disk without buffering
- [ ] Go: documents get-or-create ON CONFLICT
- [ ] Python: queued→running transition correctness and idempotency
- [ ] Python: atomic version activation (one active)

### Integration tests
- [ ] LocalStack SQS + worker: end-to-end upload → succeeded
- [ ] Failure: corrupt PDF → failed and DLQ after retries

### Concurrency tests
- [ ] Two concurrent identical uploads:
  - expect either one run reused/skipped OR both run but final state consistent, no two active versions
- [ ] Upload new content while previous run still running:
  - define policy: last completed wins, still one active version

---

## 11) Rollout plan

- [ ] Add `INGEST_MODE=orchestrated` only after queue+worker verified
- [ ] Keep old proxy path behind `INGEST_MODE=proxy` for rollback (temporary)
- [ ] Operational checklist: DLQ monitoring, stale run scan alerts, worker scaling notes

---

## 12) Optional follow-ons (after PR lands)
- [ ] Add S3 source CRUD + sync jobs using same queue framework
- [ ] Add per-source advisory lock for sync overlap
- [ ] Move raw upload store from local volume to internal S3 (no API change)
