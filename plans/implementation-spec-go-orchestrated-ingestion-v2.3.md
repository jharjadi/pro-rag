# Implementation Spec: Go-Orchestrated Ingestion (Queue-Ready) with Internal Python Worker

## Status
**V2.3** — Final pre-implementation version. Incorporates all review rounds: content_hash on document_versions, content-addressed upload URIs, INSERT ON CONFLICT dedup, streaming SHA-256, bounded worker concurrency, ACK semantics, crash guard split, document listing rules, explicit upload vs S3 versioning semantics.

> Supersedes: V2.2.1, V2.2, V2.1, V2.0, V1.0
> References: ADR-007 (auth + write contract), ADR-008 (S3 sources), Phase 8 implementation plan

---

## 1) Goal

Evolve the current ingestion flow from **Go proxy-pass-through** to **Go-orchestrated job submission**, so:

- **Web UI talks only to Go** (single public API gateway).
- **Go accepts uploads, enforces auth/RBAC, deduplicates at the document level, persists raw inputs, creates tracking records, and delegates processing** to the Python worker.
- **Python workers process files asynchronously**, performing extract → chunk → embed → FTS and **writing content tables to Postgres**, then updating ingestion run status.
- **Go serves all status and management reads** from Postgres directly.
- **If Python is removed**, the system still runs: users can log in, browse existing documents, chat, manage sources, view ingestion history. They just can't process new files — uploads will be accepted and tracked but remain in `queued` status.

---

## 2) Non-negotiable invariants

### 2.1 Tenant isolation
- `tenant_id` is derived from JWT claims, never trusted from browser payload.
- All writes (Go and Python) are scoped by `tenant_id`.
- Go passes `tenant_id` to Python via the job payload.

### 2.2 Latest-only serving
- Only `document_versions.is_active = true` is served by the query runtime.
- No change to query path.

### 2.3 Abstain behavior unchanged
- If a document has no active versions/chunks, runtime retrieval yields no evidence and existing abstain logic applies. No new abstain logic needed.

### 2.4 Citation validity unchanged
- Citations refer only to retrieved chunks from active document versions. No change to citation parser.

### 2.5 Write ownership (matches ADR-007)

| Table | Created by | Updated by | Read by |
|---|---|---|---|
| `tenants` | Go (or seed) | Go | Go |
| `users` | Go | Go | Go |
| `ingestion_sources` | Go | Go | Go |
| `source_objects` | Go | Go | Go |
| `documents` | **Go** | Go (title, metadata) | Go (query + management) |
| `ingestion_runs` | **Go** (`status=queued`, sets `created_at`) | **Python** (sets `started_at` on running, `finished_at` + status + stats + error on completion) | Go (management + polling) |
| `document_versions` | Python (owns `content_hash`) | Python (`is_active` toggling) | Go (query + management + dedup reads) |
| `chunks` | Python | — | Go (query) |
| `chunk_embeddings` | Python | — | Go (query) |
| `chunk_fts` | Python | — | Go (query) |

**Key design decisions:**
- `content_hash` lives on `document_versions`, not `documents`. Python sets it during version creation. Go reads it for dedup checks. No race condition with version activation.
- `documents` table stores identity and metadata only (`source_uri`, `title`, `source_type`). No content hash.
- Go never updates content_hash. Go never writes content tables.

**Litmus test:** `docker compose stop ingest-worker` → system still serves queries, users can log in, browse documents, manage sources, view ingestion runs (showing `queued` status for pending jobs). They cannot process new files.

---

## 3) Target architecture

```
┌─────────────┐
│  Web (Next.js)│  Browser UI
│  :3000       │  BFF proxy → Go only
└──────┬───────┘
       │
       ▼
┌──────────────┐     ┌──────────────────────────────────┐
│  core-api-go │────▶│  Postgres                         │
│  :8000       │     │  Go writes: tenants, users,       │
│              │     │    sources, source_objects,        │
│  SYSTEM OWNER│     │    documents, ingestion_runs      │
│  Auth + RBAC │     │  Python writes: document_versions,│
│  Orchestrator│     │    chunks, embeddings, fts        │
│              │     │  Python updates: ingestion_runs   │
│              │     └──────────────────────────────────┘
│              │
│              │────▶ Object Store (/data/uploads/...)
│              │       Go writes raw files (streaming)
│              │       Python reads raw files
│              │
│              │────▶ embed-svc :8001 (question embedding)
│              │────▶ Cohere Rerank (fail-open)
│              │────▶ LLM API (Anthropic Claude)
│              │
│              │──[V1: HTTP]──▶ ingest-worker :8002
│              │──[V2: Queue]─▶ SQS → ingest-worker
└──────────────┘

┌─────────────┐     ┌───────────────┐
│  embed-svc   │     │ ingest-worker  │  Internal only
│  (Flask)     │     │ (Python)       │  Bounded concurrency
│  :8001       │     │ :8002          │  Extract+chunk+embed
└─────────────┘     │                │  Writes content tables
                    │                │  Updates run status
                    └───────────────┘
```

### Service responsibilities

| Service | Owns | Does NOT own |
|---|---|---|
| **core-api-go** | Auth/JWT, RBAC, user CRUD, source CRUD, S3 listing/diffing, **doc-level dedup** (reads active version hash), document row creation (INSERT ON CONFLICT), ingestion run creation, raw file persistence (streaming), job delegation, all management reads, query runtime | File extraction, chunking, embedding, content table writes, content_hash generation |
| **ingest-worker** | Extract → chunk → embed → FTS → write content tables (incl. `content_hash` on `document_versions`) → update `ingestion_runs` status/stats/error, write extracted artifacts. **Bounded concurrency** (max concurrent jobs). | Auth, users, sources, orchestration, document/run creation, S3 access, dedup decisions |
| **embed-svc** | Question embedding at query time | Everything else |

---

## 4) API contracts (Go public)

### 4.1 Upload an ingestion job

`POST /v1/ingest` (multipart/form-data)

Form fields:
- `file`: binary (max 50MB)
- `title`: optional string (defaults to filename)

Auth: JWT required. Go derives `{tenant_id, user_id, role}` from token.

**Go's orchestration steps (in order):**

**Step 1: Validate**
- Allowed extensions: pdf, docx, html
- File size ≤ 50MB

**Step 2: Stream file to disk + compute SHA-256 simultaneously**
- **Mandatory streaming:** Go MUST compute the hash while writing the file to the object store. Never buffer the full file in memory.
- Implementation: `io.TeeReader(multipart_file, sha256_hasher)` → `io.Copy(disk_file, tee_reader)`
- This produces both the persisted file and the content_hash in a single pass.
- V1 path: `/data/uploads/<tenant_id>/<run_id>/<original_filename>`
- `upload_uri`: `file:///data/uploads/<tenant_id>/<run_id>/<original_filename>`
- `content_hash`: `sha256:<hex_digest>`

**Step 3: Compute content-addressed source_uri**
- For uploads: `source_uri = "upload://sha256:<content_hash>"`
- This is the **stable document identity** for dedup. Same content always maps to the same document, regardless of filename or uploader.
- Original filename is stored in `ingestion_runs.config.original_filename` and `documents.title`.
- For S3 sync: `source_uri = "s3://bucket/key"` (path-based, already stable).

**Step 4: Get-or-create document + dedup check (atomic)**

Single transaction using the unique constraint on `(tenant_id, source_uri)`:

```sql
-- Step 4a: Atomic get-or-create document
INSERT INTO documents (doc_id, tenant_id, source_type, source_uri, title)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (tenant_id, source_uri)
DO UPDATE SET title = EXCLUDED.title
RETURNING doc_id, (xmax = 0) AS is_new;
```

- `is_new = true` → document was just created (first time this source_uri seen). Proceed to step 5.
- `is_new = false` → document already existed. Check active version hash:

```sql
-- Step 4b: Check if active version matches content_hash
SELECT dv.content_hash
FROM document_versions dv
WHERE dv.tenant_id = $1
  AND dv.doc_id = $2
  AND dv.is_active = true;
```

**Dedup outcomes:**

| `is_new` | Active version exists | Hash matches | Action |
|---|---|---|---|
| `true` | — | — | New document. Proceed to create run + delegate. |
| `false` | No active version | — | Document exists but no active content (previously deactivated or never processed). Proceed to create run + delegate. |
| `false` | Yes | ✅ matches | **Skip.** Return `200 {status: "skipped"}`. Delete uploaded file. No new rows. |
| `false` | Yes | ❌ differs | **New version** (S3 sync scenario only — see versioning semantics below). Proceed to create run + delegate with existing `doc_id`. |

**Concurrent upload safety:**
- Two simultaneous uploads of the same file: both compute the same `source_uri`. `INSERT ... ON CONFLICT` serializes document creation. The first proceeds; the second sees the doc exists and checks for an active version.
  - If the first hasn't finished yet (no active version): the second also creates a run. Both process. Last completed wins (documented in §7.4).
  - If the first has completed (active version with matching hash): the second returns "skipped."
- Two simultaneous uploads of different files: different `source_uri` → no conflict → both proceed independently.

**Upload versioning semantics (content-addressed):**

Uploads use content-addressed document identity: `source_uri = upload://sha256:<content_hash>`. This means:

- **Re-uploading the same file** (identical bytes) → same `source_uri` → same document → **skip** (already ingested).
- **Uploading a modified file** (different bytes) → different `source_uri` → **new document** (not a new version of the old one).
- **Uploads do not support multi-version history.** Each unique file content is its own document with a single version.

This is a deliberate product decision for V1. Users who want version history for evolving documents should use S3 sync, where `source_uri = s3://bucket/key` is path-based and the same key with different content creates a new version of the same document.

| Source | `source_uri` | Same path, different content | Same content |
|---|---|---|---|
| Upload | `upload://sha256:<hash>` | New document | Skip |
| S3 sync | `s3://bucket/key` | New version of same doc | Skip |

**Step 5: Create `ingestion_runs` row**
- `run_id` (new UUID), `tenant_id`, `status = 'queued'`, `created_at = now()`
- `started_at = NULL` (set by worker when processing begins)
- `config` JSONB:
  ```json
  {
    "source_type": "upload",
    "upload_uri": "file:///data/uploads/t1/run-uuid/foo.pdf",
    "title": "IT Security Policy",
    "original_filename": "foo.pdf",
    "content_type": "application/pdf",
    "file_size_bytes": 1234567,
    "content_hash": "sha256:abcdef...",
    "created_by_user_id": "uuid"
  }
  ```
- Note: `content_hash` in config is informational (for display/logging). The authoritative `content_hash` is on `document_versions`, written by Python.

**Step 6: Delegate to worker**
- V1 (HTTP): `POST http://ingest-worker:8002/internal/process` with job payload JSON
- V2 (Queue): publish job payload as SQS message
- Authentication: include `Authorization: Bearer <INTERNAL_AUTH_TOKEN>` header
- **If delegation fails:** update `ingestion_runs.status = 'failed'`, `error = 'worker unavailable'`. See §11.2 for failure taxonomy.

**Step 7: Return response**
- Normal: `202 Accepted` with `{run_id, doc_id, status: "queued"}`
- Skipped (dedup): `200 OK` with `{doc_id, status: "skipped", reason: "already ingested, no changes"}`

**Failure scenarios:**

| Failure | Behavior | DB state |
|---|---|---|
| File validation fails | 400 Bad Request | No rows, no file |
| Streaming/disk write fails | 500 Internal | No rows, partial file cleaned up |
| Content hash dedup match | 200 OK `status: skipped` | No new rows, uploaded file deleted |
| DB write fails (after file stored) | 500 Internal, orphaned file cleaned by TTL | No rows (rolled back) |
| Worker delegation fails | Run exists with `status=failed` | doc + run rows exist |
| Worker busy (503) | Run exists with `status=failed` | doc + run rows exist |
| Concurrent duplicate upload | Second serialized by ON CONFLICT; returns skipped or creates second run | See concurrent safety above |

### 4.2 Poll ingestion run status

`GET /v1/ingestion-runs/:id`

Go reads Postgres directly:
```json
{
  "run_id": "uuid",
  "tenant_id": "uuid",
  "status": "queued|running|succeeded|failed",
  "run_type": "manual_upload",
  "created_at": "2026-02-15T12:00:00Z",
  "started_at": "2026-02-15T12:00:02Z",
  "finished_at": "2026-02-15T12:00:07Z",
  "config": {
    "source_type": "upload",
    "created_by_user_id": "uuid",
    "upload_uri": "file:///data/uploads/...",
    "title": "IT Security Policy",
    "original_filename": "it_security.pdf",
    "content_type": "application/pdf",
    "file_size_bytes": 1234567,
    "content_hash": "sha256:abcdef..."
  },
  "stats": {
    "docs_processed": 1,
    "chunks_created": 14,
    "tokens_total": 1477
  },
  "error": null
}
```

**Timing fields:**
- `created_at` — when Go created the run (queued)
- `started_at` — when Python began processing (running). NULL if still queued.
- `finished_at` — when Python completed (succeeded/failed). NULL if queued or running.
- Queue wait time = `started_at - created_at`
- Processing time = `finished_at - started_at`

### 4.3 List ingestion runs

`GET /v1/ingestion-runs?page=1&limit=20`

Go reads Postgres. Returns runs for tenant (from JWT). Supports pagination and filtering by status.

### 4.4 List documents

`GET /v1/documents?page=1&limit=20&search=...`

**Document listing only includes documents with at least one version (active or historical).** Documents that were created by Go but never processed by the worker (e.g., delegation failed, run still queued) are excluded from the default listing.

Query pattern:
```sql
SELECT d.*, dv.doc_version_id, dv.version_label, dv.is_active, ...
FROM documents d
INNER JOIN document_versions dv ON d.doc_id = dv.doc_id AND d.tenant_id = dv.tenant_id
WHERE d.tenant_id = $1
  AND dv.is_active = true
ORDER BY d.created_at DESC
LIMIT $2 OFFSET $3;
```

**Rationale:** After a transient worker outage, Go may have created `documents` rows for uploads that were never processed. Showing these as empty entries in the UI would look broken. Once the worker processes them (or the run is retried), they appear naturally.

Documents with no active version but with historical versions (e.g., deactivated documents) can be shown with an "inactive" badge if a `status` filter is provided.

### 4.5 Admin: orphaned documents (V2, optional)

For operational debugging, an admin endpoint could list documents with zero versions:
```sql
SELECT d.* FROM documents d
LEFT JOIN document_versions dv ON d.doc_id = dv.doc_id
WHERE d.tenant_id = $1 AND dv.doc_version_id IS NULL;
```

Not required for V1 but useful for diagnosing worker outage impacts.

---

## 5) Job payload schema (shared between HTTP and queue modes)

The same schema is used regardless of transport (HTTP body or SQS message). **No file binary is included** — the worker always reads from `upload_uri`.

```json
{
  "job_type": "upload_ingest",
  "run_id": "uuid",
  "doc_id": "uuid",
  "tenant_id": "uuid",
  "upload_uri": "file:///data/uploads/t1/run-uuid/foo.pdf",
  "title": "IT Security Policy",
  "source_type": "pdf",
  "source_uri": "upload://sha256:abcdef1234567890...",
  "original_filename": "foo.pdf",
  "content_hash": "sha256:abcdef1234567890...",
  "embedding_model": "BAAI/bge-base-en-v1.5",
  "embedding_dim": 768,
  "submitted_at": "2026-02-15T00:00:00Z"
}
```

**Design principles:**
- **Self-contained:** The worker does not need to call Go back or look up orchestration state.
- **Transport-agnostic:** Identical payload for HTTP POST body and SQS message body.
- **No file binary:** Worker reads from `upload_uri`. Keeps the payload small and compatible with SQS (256KB message size limit).
- **content_hash included:** The worker uses this to set `document_versions.content_hash` during version creation. Go computed it during upload; the worker trusts it.

---

## 6) Internal worker auth

### 6.1 Shared HMAC token

Go and ingest-worker share `INTERNAL_AUTH_TOKEN` (env var, random 32+ bytes).

- Go includes `Authorization: Bearer <INTERNAL_AUTH_TOKEN>` on all calls to `/internal/*`.
- Worker validates the token on every request. Rejects with 401 if missing or mismatched.
- V1: simple shared token. V2: upgrade to mTLS if needed.

### 6.2 Network isolation

- Worker uses `expose: ["8002"]` in Docker Compose (not `ports`). Not accessible from host.
- Only reachable on the Docker network by service name (`ingest-worker:8002`).
- In production (ECS/k8s): use security groups or network policies to restrict access to Go only.

---

## 7) Worker behavior (Python ingest-worker)

### 7.1 Concurrency model (bounded)

The worker runs a **bounded number of concurrent jobs** to prevent resource exhaustion.

**V1 (HTTP mode):**
- Worker uses an internal bounded task pool: `concurrent.futures.ThreadPoolExecutor(max_workers=3)` or `asyncio.Semaphore(3)`.
- When `POST /internal/process` is received:
  - If a slot is available: accept job, return `202 Accepted`, process in background.
  - If all slots are full: return `503 Service Unavailable`. Go marks the run `failed` with `error = 'worker busy, all slots occupied'`.
- Background tasks are in-process. If the worker process exits, in-flight jobs are lost. The crash guard (§11) handles this — stale `running` runs with no heartbeat are marked `failed`.

**V2 (Queue mode):**
- Worker polls SQS with `MaxNumberOfMessages=1` and uses the same bounded pool.
- Backpressure is natural: worker only polls when a slot is available.
- No 503 scenario — the queue buffers work.

**Configuration:** `WORKER_MAX_CONCURRENT_JOBS=3` (env var, default 3).

### 7.2 Endpoint: `POST /internal/process`

Request: `application/json` body containing job payload (§5). No file binary.

Auth: `Authorization: Bearer <INTERNAL_AUTH_TOKEN>` header. Reject 401 if invalid.

Immediate response:
- `202 Accepted` with `{"status": "accepted", "run_id": "uuid"}` — job queued internally
- `503 Service Unavailable` with `{"error": "worker busy"}` — all slots occupied

Background task:

1. **Transition run to `running`:**
   ```sql
   UPDATE ingestion_runs
   SET status = 'running',
       started_at = COALESCE(started_at, now()),
       updated_at = now()
   WHERE run_id = $1 AND status IN ('queued', 'failed');
   ```
   `COALESCE(started_at, now())` ensures `started_at` is only set on the first transition to `running`. On retries (re-processing a `failed` run), the original `started_at` is preserved for accurate timing metrics.

   If no row updated (status is `running` or `succeeded`): check current status.
   - `succeeded` → no-op, return success (duplicate delivery)
   - `running` with `updated_at` < 15 min ago → skip (another worker instance is active)
   - `running` with `updated_at` ≥ 15 min ago → treat as abandoned, re-process (update status back to `running` with fresh `updated_at`)

2. **Read raw file** from `upload_uri` (local filesystem read in V1)

3. **Run ingestion pipeline** (with heartbeat updates after each stage):
   - Extract text → **heartbeat**
   - Structure-aware chunking (target 350-500 tokens, hard cap 800) → **heartbeat**
   - Batch embedding (BAAI/bge-base-en-v1.5, batch size ≤ 256) → **heartbeat**
   - Generate FTS tsvectors → **heartbeat**

4. **Write content tables atomically (single transaction):**
   - Create `document_versions` row with `content_hash` (from job payload)
   - Deactivate any existing active version for this `doc_id` in the same transaction
   - Insert `chunks` rows
   - Insert `chunk_embeddings` rows
   - Insert `chunk_fts` rows
   - Write extracted artifact to `file:///data/artifacts/<tenant>/<doc>/<version>.json`
   - Partial unique index enforces at most one active version per `(tenant_id, doc_id)`

5. **Update run on completion:**
   - Success:
     ```sql
     UPDATE ingestion_runs
     SET status = 'succeeded', finished_at = now(), updated_at = now(),
         stats = $2
     WHERE run_id = $1;
     ```
   - Failure:
     ```sql
     UPDATE ingestion_runs
     SET status = 'failed', finished_at = now(), updated_at = now(),
         error = $2, stats = $3
     WHERE run_id = $1;
     ```

6. **Cleanup raw upload** — on success, delete the file at `upload_uri`. On failure, leave it for debugging (TTL cleanup handles it).

### 7.3 Heartbeat (required)

The worker **must** update `ingestion_runs.updated_at` after each major pipeline stage:

```sql
UPDATE ingestion_runs SET updated_at = now() WHERE run_id = $1;
```

Heartbeat cadence:
- After extraction complete
- After chunking complete
- After embedding complete
- After FTS generation complete
- Final update includes status transition (succeeded/failed)

This is ~4-5 lightweight UPDATE calls per job. Negligible overhead compared to embedding (seconds). Enables the crash guard to distinguish "actively processing" from "truly stuck."

### 7.4 Idempotency rules

**Run-level (duplicate delivery handling):**
- `status = 'succeeded'` → no-op, return success
- `status = 'running'` with recent `updated_at` (< 15 min) → skip (another instance is active)
- `status = 'running'` with stale `updated_at` (≥ 15 min) → re-process (original worker likely crashed)
- `status = 'failed'` → re-process (explicit retry)
- `status = 'queued'` → normal path (transition to running)

**Document-level dedup:**
- Dedup is handled at the **Go layer** (§4.1 step 4), not the worker. By the time the worker receives a job, Go has already decided this file needs processing.
- The worker does not perform content_hash dedup checks. It trusts Go's dedup decision.

**Version activation:**
- Atomic within a single DB transaction: deactivate old version, activate new version.
- Partial unique index `document_versions(tenant_id, doc_id) WHERE is_active = true` enforces at most one active version.

**Concurrent version handling (new version uploaded while previous run still processing):**
- Both runs are accepted and processed.
- Each run's version activation is atomic (deactivate old, activate new, in one transaction).
- The **last run to complete** wins — its version becomes the active version.
- This is acceptable for V1. V2 can add sequencing if needed.
- This scenario is documented as a known behavior, not a bug.

**V2 recommendation: per-run advisory lock for multi-worker scaling.**
In queue mode with multiple worker instances, two workers could both decide a stale `running` job is abandoned and attempt to re-process simultaneously. To prevent this:
- Worker acquires a Postgres advisory lock on `run_id` hash before transitioning to `running`.
- If lock not acquired: skip (another worker is handling it).
- Lock released on completion or failure (session-scoped).
This is optional in V1 (single worker instance) but recommended before scaling to multiple workers.

### 7.5 Queue mode (V2): SQS consumer

Same processing logic as §7.2, but:
- Worker polls SQS queue instead of listening on HTTP
- Only polls when a concurrency slot is available (natural backpressure)
- On success: ACK message (delete from queue)
- On pipeline error: mark run `failed` in DB, **then ACK message**
- On transient infrastructure failure (DB connection lost, OOM): do **not** ACK. Message retries via SQS visibility timeout.
- After `maxReceiveCount` (default 3): message moves to DLQ

**Critical ACK rule:**

> **Once `status=failed` is written to Postgres, always ACK/delete the queue message.** This prevents status flapping (`failed→running→failed...`). If you want to retry a failed run, that's a new job submitted by Go, not a queue-level retry.
>
> **If the worker cannot persist `status=failed` to Postgres** (DB unavailable), do **NOT** ACK the message. The terminal status must be durably recorded before the message is removed. Let SQS retry — on the next attempt, either DB is back (persist failed + ACK) or it fails again (don't ACK, eventually DLQ).

---

## 8) Storage strategy

### 8.1 Raw upload store

- **Purpose:** Decouple file reception from processing. Go persists the raw file immediately; Python reads it when it processes the job.
- **Streaming requirement:** Go MUST stream the multipart upload to disk while computing SHA-256. Implementation: `io.TeeReader(src, hasher)` → `io.Copy(dst, tee)`. Never buffer the full file in memory. This is mandatory.
- **V1 implementation:** Shared Docker volume mounted to both Go and Python containers.
  - Go writes: `/data/uploads/<tenant_id>/<run_id>/<original_filename>`
  - Python reads: same path
- **V2 upgrade:** Replace local volume with internal S3 bucket. Change `upload_uri` scheme from `file:///` to `s3://`. No other changes needed.

**Cleanup policy:**
- Successful ingestion: worker deletes the raw upload file after processing completes.
- Dedup skip: Go deletes the uploaded file immediately after determining skip.
- Failed ingestion: raw file retained for debugging. Go runs periodic cleanup of failed uploads older than `UPLOAD_FAILED_TTL_DAYS` (default 7).
- Orphaned files (no matching run): Go runs periodic cleanup of uploads older than `UPLOAD_CLEANUP_TTL_HOURS` (default 24).

### 8.2 Extracted artifact store (unchanged)

- Python writes extracted artifacts to: `file:///data/artifacts/<tenant>/<doc>/<version>.json`
- Go never reads or writes artifacts. These are for debugging and reprocessing.
- V2: migrate to `s3://internal-bucket/artifacts/...`

### 8.3 Docker volume configuration

```yaml
volumes:
  uploads:    # Raw uploaded files (Go writes, Python reads)
  artifacts:  # Extracted artifacts (Python writes)
  pgdata:     # Postgres data
```

---

## 9) Data model changes

### 9.1 `documents` table — remove `content_hash`, add unique constraint

`content_hash` moves to `document_versions`. The `documents` table stores identity and metadata only.

```sql
-- Remove content_hash if it exists
ALTER TABLE documents DROP COLUMN IF EXISTS content_hash;
```

Final `documents` schema:
```sql
CREATE TABLE documents (
    doc_id      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID NOT NULL REFERENCES tenants(tenant_id),
    source_type TEXT NOT NULL,
    source_uri  TEXT NOT NULL,
    title       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_documents_tenant ON documents(tenant_id);
CREATE UNIQUE INDEX idx_documents_source_uri ON documents(tenant_id, source_uri);
```

The UNIQUE index on `(tenant_id, source_uri)` enables the `INSERT ... ON CONFLICT` dedup pattern in §4.1. For uploads, `source_uri = upload://sha256:<hash>` ensures one document per unique content per tenant. For S3 sync, `source_uri = s3://bucket/key` ensures one document per S3 object per tenant.

### 9.2 `document_versions` table — add `content_hash`

```sql
ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS content_hash TEXT NOT NULL;
```

`content_hash` is the SHA-256 of the raw file bytes that produced this version. Set by the Python worker during version creation (value passed from Go via job payload). Used by Go's dedup query to check if the active version matches a new upload.

### 9.3 `ingestion_runs` — add `queued` status and timing columns

```sql
-- Allow 'queued' status
ALTER TABLE ingestion_runs DROP CONSTRAINT IF EXISTS ingestion_runs_status_check;
ALTER TABLE ingestion_runs ADD CONSTRAINT ingestion_runs_status_check
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed'));

-- started_at is nullable (set by worker, not Go)
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;

-- updated_at for heartbeat
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT now();
```

**State machine:**
```
queued → running → succeeded
queued → running → failed
queued → failed  (worker unavailable at delegation time)
```

**Timing semantics:**
- `created_at` — when Go created the run row (job accepted)
- `started_at` — when Python began processing (NULL while queued; preserved across retries via COALESCE)
- `updated_at` — last heartbeat from Python (used by crash guard)
- `finished_at` — when Python completed (NULL while queued or running)

### 9.4 `ingestion_runs` — add orchestration columns

```sql
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS run_type TEXT NOT NULL DEFAULT 'manual_upload'
    CHECK (run_type IN ('manual_upload', 's3_sync'));
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS source_id UUID REFERENCES ingestion_sources(source_id);
ALTER TABLE ingestion_runs ADD COLUMN IF NOT EXISTS initiated_by UUID REFERENCES users(user_id);
```

### 9.5 `ingestion_runs.config` JSONB schema

No migration needed (JSONB is flexible). Standard keys:

```json
{
  "source_type": "upload",
  "upload_uri": "file:///data/uploads/t1/run-uuid/foo.pdf",
  "title": "IT Security Policy",
  "original_filename": "foo.pdf",
  "content_type": "application/pdf",
  "file_size_bytes": 1234567,
  "content_hash": "sha256:abcdef...",
  "created_by_user_id": "uuid"
}
```

Note: `content_hash` in config is informational. The authoritative `content_hash` is on `document_versions`.

### 9.6 Indexes and constraints (explicit DDL)

**Content tables (existing — confirm present):**

```sql
-- One active version per (tenant_id, doc_id)
CREATE UNIQUE INDEX idx_doc_versions_one_active
    ON document_versions (tenant_id, doc_id)
    WHERE is_active = true;

-- HNSW vector index
CREATE INDEX idx_chunk_embeddings_hnsw
    ON chunk_embeddings USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- FTS GIN index
CREATE INDEX idx_chunk_fts_gin ON chunk_fts USING gin(tsv);

-- Tenant filter indexes on content tables
CREATE INDEX idx_chunks_tenant_version ON chunks(tenant_id, doc_version_id);
CREATE INDEX idx_chunk_embeddings_tenant ON chunk_embeddings(tenant_id);
CREATE INDEX idx_chunk_fts_tenant ON chunk_fts(tenant_id);
```

**Orchestration tables (new):**

```sql
-- Document identity (unique per tenant + source_uri) — enables INSERT ON CONFLICT
CREATE UNIQUE INDEX idx_documents_source_uri ON documents(tenant_id, source_uri);

-- Dedup: find active version hash for a document
CREATE INDEX idx_doc_versions_dedup
    ON document_versions(tenant_id, doc_id, content_hash)
    WHERE is_active = true;

-- Ingestion run listing by tenant + status
CREATE INDEX idx_ingestion_runs_tenant_status
    ON ingestion_runs(tenant_id, status);

-- Crash guard: find stale runs
CREATE INDEX idx_ingestion_runs_stale
    ON ingestion_runs(status, updated_at)
    WHERE status IN ('queued', 'running');
```

Note: `ingestion_runs.run_id` is the primary key — no additional index needed for polling by `run_id`.

### 9.7 No changes to content table schemas

`chunks`, `chunk_embeddings`, `chunk_fts` — no schema changes. Python writes these exactly as it does today.

---

## 10) Auth and RBAC (reference: ADR-007)

### Authentication
- Go validates credentials against `users` table (bcrypt, cost factor 12)
- Go signs JWT with HMAC-SHA256 using `JWT_SECRET` env var
- JWT claims: `{sub: user_id, tenant_id, role, exp}`
- Token expiry: 24h (configurable via `JWT_EXPIRY_HOURS`)
- Go middleware verifies JWT on all protected endpoints (no DB lookup per request)

### RBAC

| Action | `admin` | `user` |
|---|---|---|
| Upload documents | ✅ | ✅ |
| View documents / chunks | ✅ | ✅ |
| Chat / query | ✅ | ✅ |
| View ingestion runs | ✅ | ✅ |
| Create/edit/delete S3 sources | ✅ | ❌ |
| Trigger sync | ✅ | ❌ |
| Manage users | ✅ | ❌ |
| Deactivate documents | ✅ | ❌ |

### Auth mode

`AUTH_ENABLED` env var (default `true`).

- **Standard compose profile:** `AUTH_ENABLED=true`. JWT required on all protected endpoints. `tenant_id` derived from JWT claims.
- **Development override:** `docker-compose.dev.yml` sets `AUTH_ENABLED=false`. Accepts `tenant_id` from query param (backward compat for local testing without auth).

```yaml
# docker-compose.dev.yml (override)
services:
  core-api-go:
    environment:
      AUTH_ENABLED: "false"
```

---

## 11) Crash guard, stuck runs, and failure taxonomy

### 11.1 Go startup crash guard (split by status)

On startup, Go runs two cleanup queries:

**Stale `queued` runs** (job was never picked up):
```sql
UPDATE ingestion_runs
SET status = 'failed',
    error = 'interrupted — job was never picked up (service restarted)',
    finished_at = now(),
    updated_at = now()
WHERE status = 'queued'
  AND created_at < now() - INTERVAL '1 hour';
```

**Stale `running` runs** (worker probably crashed — no heartbeat):
```sql
UPDATE ingestion_runs
SET status = 'failed',
    error = 'interrupted — worker stopped responding (no heartbeat)',
    finished_at = now(),
    updated_at = now()
WHERE status = 'running'
  AND updated_at < now() - INTERVAL '15 minutes';
```

**Why split:**
- `queued` uses `created_at` with a long window (1 hour) — the job may be waiting in a queue or for a worker slot.
- `running` uses `updated_at` with a shorter window (15 minutes) — if the worker hasn't sent a heartbeat in 15 minutes, it's likely dead. Safe because heartbeats are required (§7.3) and fire after each pipeline stage.

### 11.2 Delegation failure taxonomy

Three distinct failure scenarios, all resulting in `status=failed` with a descriptive error:

| Scenario | When | Root cause | Go action |
|---|---|---|---|
| **HTTP delegation failure** | V1: Go calls worker, gets error/timeout/503 | Worker down or busy | Mark run `failed` immediately with `error = 'worker unavailable'` or `'worker busy, all slots occupied'` |
| **Queue publish failure** | V2: Go publishes to SQS, gets error | Queue down or misconfigured | Mark run `failed` immediately with `error = 'queue publish failed'` |
| **Queued TTL expiry** | Any mode: job queued but never picked up | Worker down, queue stuck, or persistent issue | Crash guard marks `failed` with `error = 'job was never picked up'` |

All three produce a `failed` run visible in the UI. The user can re-upload to retry.

---

## 12) Observability

### Go structured logs (per ingestion request)

One JSON log line when `POST /v1/ingest` is handled:

```json
{
  "ts": "2026-02-15T12:00:00Z",
  "event": "ingest_job_submitted",
  "tenant_id": "uuid",
  "user_id": "uuid",
  "run_id": "uuid",
  "doc_id": "uuid",
  "dedup_result": "new|new_version|skipped",
  "filename": "it_security.pdf",
  "content_type": "application/pdf",
  "size_bytes": 1234567,
  "content_hash": "sha256:abcdef...",
  "source_uri": "upload://sha256:abcdef...",
  "upload_uri": "file:///data/uploads/...",
  "delegation_mode": "http",
  "delegation_success": true
}
```

### Python structured logs (per job)

One JSON log line per job processed:

```json
{
  "ts": "2026-02-15T12:00:05Z",
  "event": "ingest_job_complete",
  "run_id": "uuid",
  "tenant_id": "uuid",
  "doc_id": "uuid",
  "status": "succeeded",
  "chunks_created": 14,
  "tokens_total": 1477,
  "duration_ms": 4500,
  "queue_wait_ms": 2000,
  "stages": {
    "extract_ms": 800,
    "chunk_ms": 200,
    "embed_ms": 2500,
    "fts_ms": 100,
    "db_write_ms": 900
  }
}
```

### Query logging (unchanged)

One structured JSON log line per query as specified in spec §Logging. No changes.

---

## 13) Failure modes

| Failure | Behavior | User-visible | DB state |
|---|---|---|---|
| File validation fails | Go returns 400 | Immediate error in upload form | No rows created |
| Streaming/disk write fails | Go returns 500 | "Upload failed, please retry" | No rows created |
| Content hash dedup match | Go returns 200 `status: skipped` | "Already ingested, no changes" | No new rows |
| DB write fails (after file stored) | Go returns 500, orphaned file cleaned by TTL | "Upload failed, please retry" | No rows (rolled back) |
| Worker delegation fails (HTTP) | Go marks run `status=failed` | "failed: worker unavailable" | doc + run rows exist |
| Worker busy (503) | Go marks run `status=failed` | "failed: worker busy" | doc + run rows exist |
| Worker crashes mid-processing | Crash guard marks `failed` after 15 min stale heartbeat | "running" → eventually "failed" | doc + run rows, no content |
| Corrupt document | Python marks run `failed` with extraction error | "failed: extraction error: ..." | doc + run rows, no content |
| Worker completely down | Go marks run `failed` at delegation | "Ingestion service unavailable" + existing docs/chat still work | doc + run rows exist |
| Concurrent identical upload | Serialized by INSERT ON CONFLICT; second returns skipped or also processes | First: "queued", second: "skipped" or "queued" | See §4.1 concurrent safety |
| New version while previous still running | Both accepted. Last completed wins. | Both runs visible in history | Both runs exist; final active = last completed |
| Queue: duplicate delivery (V2) | Worker detects via status check, no-ops | No change | No change |
| Queue: DB down when persisting failed (V2) | Don't ACK. SQS retries. | Run may show stale status | Eventually consistent |

---

## 14) S3 sync integration (uses same worker contract)

S3 sync (ADR-008, Phase 8d) uses the same ingest-worker interface:

1. **Go orchestrates sync:** Lists S3 objects, detects changes (per ADR-008 idempotency rules).
2. **For each changed object:** Go downloads file from S3, streams to upload store (computing SHA-256), creates/reuses `documents` row (same INSERT ON CONFLICT pattern with `source_uri = s3://bucket/key`) + creates `ingestion_runs` row, delegates to ingest-worker with the same job payload.
3. **Worker processes identically:** Same extract → chunk → embed → write flow.
4. **Deletion handling:** Go deactivates document versions directly (no worker involvement). **S3 deletion is deactivation, not row deletion.** Go sets the active `document_version` to `is_active = false` and updates `source_objects.status = 'deleted'`. No rows are deleted from any table — all historical data (versions, chunks, embeddings) is preserved for audit. The query runtime naturally returns no results for this document, and existing abstain behavior applies.

**Differences from manual upload:**
- `run_type = 's3_sync'` (vs `manual_upload`)
- `source_id` populated on `ingestion_runs`
- `source_uri = 's3://bucket/key'` (path-based, vs content-addressed `upload://sha256:<hash>`)
- Go downloads file from S3 before persisting to upload store (worker never touches S3)
- Go manages advisory lock for per-source concurrency (§ADR-008)

**Versioning difference:** S3 sync uses path-based identity (`s3://bucket/key`), so changing a file at the same S3 key creates a new version of the existing document (old version deactivated, new version activated). Uploads use content-addressed identity, so different content always creates a new document. This distinction is intentional — S3 sync models "a document that evolves over time," while uploads model "a specific piece of content."

This keeps AWS credentials out of the Python worker entirely.

---

## 15) Testing checklist

### Unit tests

**Go:**
- Multipart upload parsing + streaming to disk with `io.TeeReader` SHA-256
- Content-addressed source_uri construction (`upload://sha256:<hash>`)
- INSERT ON CONFLICT dedup: new doc (`is_new=true`), existing doc with matching hash (skip), existing doc with no active version (proceed)
- Concurrent INSERT ON CONFLICT safety (two goroutines, same source_uri)
- `ingestion_runs` row creation with `status=queued`, `created_at`, `started_at=NULL`
- Job payload construction
- Delegation failure handling: worker unavailable → run failed, worker busy (503) → run failed
- Crash guard: stale queued (>1h created_at) marked failed, stale running (>15min updated_at) marked failed, recently heartbeated running NOT touched
- Document listing excludes docs with zero versions
- Auth middleware: JWT validation, role enforcement
- Internal auth token validation

**Python:**
- Job payload deserialization
- Run status transitions: queued→running, running→succeeded, running→failed
- `COALESCE(started_at, now())` preserves started_at on retries
- Duplicate delivery handling: already succeeded → no-op, stale running → re-process
- Heartbeat updates at each pipeline stage
- `content_hash` written to `document_versions` during creation
- Atomic version activation/deactivation in single transaction
- Pipeline error handling (extraction failure, embedding failure)
- Internal auth token validation (reject 401 without valid token)
- Concurrency: 503 returned when all slots occupied
- Raw upload file cleanup on success

### Integration tests

- Upload via Go → file streamed → SHA-256 computed → worker processes → run queued→running→succeeded → document appears in list → queryable with citations
- Upload with worker down → run marked `failed: worker unavailable` → existing data still served → document NOT in listing (zero versions)
- Upload duplicate file (same content) → skipped at Go layer (no new rows), uploaded file deleted
- Upload new version via S3 sync (same source_uri, different content) → new version created, old deactivated
- **Concurrent upload of identical file** (two requests simultaneously) → INSERT ON CONFLICT serializes; first creates rows, second blocks then returns skipped (or both process, last wins)
- **New version while previous run still running** → both accepted, last completed version becomes active
- Crash guard: start run, kill worker, restart Go → stale run marked failed
- S3 sync flow: list → detect changes → per-file delegation → versions updated
- S3 deletion: remove object → Go deactivates version → query abstains → no rows deleted
- Worker busy: fill all slots, send another job → 503, run marked failed

### E2E (UI)

- Upload from browser → ingestion page shows `queued` → polls → `running` → `succeeded` → document in list → queryable in chat
- Upload with worker down → shows "failed: worker unavailable"
- Upload duplicate → shows "already ingested, no changes"
- S3 sync from admin UI → progress → documents appear

---

## 16) Rollout / migration plan

### Step 1: Schema changes

- Move `content_hash` from `documents` to `document_versions`
- Add unique index on `documents(tenant_id, source_uri)`
- Add `queued` to `ingestion_runs` status constraint
- Add `started_at`, `updated_at` columns to `ingestion_runs`
- Add `run_type`, `source_id`, `initiated_by` columns
- Add dedup + crash guard indexes
- **Test:** Migrations run cleanly, existing data intact

### Step 2: Refactor write ownership (critical change)

- Go creates `documents` (INSERT ON CONFLICT) and `ingestion_runs` rows before delegating
- Go computes SHA-256 (streaming) and performs dedup check
- Python stops creating `documents` and `ingestion_runs` rows
- Python sets `content_hash` on `document_versions`
- Python continues to create content tables and update run status
- Move crash guard from Python startup to Go startup
- Update document listing to exclude docs with zero versions
- **Test:** Upload flow works end-to-end with new ownership

### Step 3: Add raw upload object store

- Add shared Docker volume (`uploads`)
- Go streams files to disk via `io.TeeReader`
- Python reads files from `upload_uri`
- Add cleanup jobs for successful/failed/orphaned uploads
- **Test:** Files persisted and readable by worker, cleanup works

### Step 4: Add internal worker auth + heartbeat + bounded concurrency

- Add `INTERNAL_AUTH_TOKEN` env var
- Go sends token on internal calls, worker validates
- Worker uses bounded task pool (`max_workers=3`)
- Worker returns 503 when busy; Go marks run failed
- Worker sends heartbeat after each pipeline stage
- Update crash guard to use `updated_at` for running jobs
- **Test:** Unauthorized rejected, 503 when busy, heartbeat visible, crash guard correct

### Step 5 (V2): Swap HTTP for queue

- Add SQS (LocalStack for dev)
- Change Go delegation from HTTP POST to SQS publish
- Add queue consumer to ingest-worker
- Implement ACK-after-fail rule (including DB-down scenario)
- Add per-run advisory lock for multi-worker safety
- No changes to payload schema, worker logic, or DB writes
- **Test:** Same E2E tests pass with queue transport

---

## 17) Environment variables (new/changed)

```env
# Auth (Phase 8a)
JWT_SECRET=<random-32-bytes>
JWT_EXPIRY_HOURS=24
AUTH_ENABLED=true                        # Default true; dev override sets false

# Internal service auth
INTERNAL_AUTH_TOKEN=<random-32-bytes>

# Ingestion orchestration
INGEST_MODE=http                         # http (V1) or queue (V2)
UPLOAD_STORE_PATH=/data/uploads
UPLOAD_MAX_SIZE_MB=50
UPLOAD_CLEANUP_TTL_HOURS=24              # TTL for orphaned uploads
UPLOAD_FAILED_TTL_DAYS=7                 # TTL for failed upload files

# Worker
INGEST_WORKER_URL=http://ingest-worker:8002
INGEST_WORKER_TIMEOUT_MS=300000          # 5 min HTTP timeout
WORKER_MAX_CONCURRENT_JOBS=3             # Bounded concurrency

# Crash guard
CRASH_GUARD_QUEUED_TTL_HOURS=1           # Mark queued runs failed after this
CRASH_GUARD_RUNNING_STALE_MIN=15         # Mark running runs failed if no heartbeat

# Queue (V2 only)
# SQS_QUEUE_URL=http://localstack:4566/000000000000/ingest_jobs
# SQS_REGION=ap-southeast-2
# SQS_DLQ_URL=...
# SQS_MAX_RECEIVE_COUNT=3
```

---

## 18) Docker Compose changes

### Standard profile (auth enabled)

```yaml
services:
  core-api-go:
    # ... existing config ...
    volumes:
      - uploads:/data/uploads
    environment:
      AUTH_ENABLED: "true"
      JWT_SECRET: ${JWT_SECRET}
      INTERNAL_AUTH_TOKEN: ${INTERNAL_AUTH_TOKEN}
      INGEST_MODE: http
      UPLOAD_STORE_PATH: /data/uploads
      INGEST_WORKER_URL: http://ingest-worker:8002

  ingest-worker:
    build:
      context: ./ingest-worker
      dockerfile: Dockerfile
    container_name: prorag-ingest-worker
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://prorag:prorag_dev@postgres:5432/prorag?sslmode=disable
      EMBEDDING_MODEL: BAAI/bge-base-en-v1.5
      EMBEDDING_DIM: 768
      INTERNAL_AUTH_TOKEN: ${INTERNAL_AUTH_TOKEN}
      WORKER_MAX_CONCURRENT_JOBS: 3
    expose:
      - "8002"
    volumes:
      - uploads:/data/uploads
      - artifacts:/data/artifacts
      - ./data:/data
    healthcheck:
      test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8002/health')"]
      interval: 5s
      timeout: 10s
      retries: 6

volumes:
  uploads:
  artifacts:
  pgdata:
```

### Development override (no auth)

```yaml
# docker-compose.dev.yml
services:
  core-api-go:
    environment:
      AUTH_ENABLED: "false"
```

Usage: `docker compose -f docker-compose.yml -f docker-compose.dev.yml up`
