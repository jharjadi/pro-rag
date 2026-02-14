# pro-rag Web UI — Implementation Spec v2

> Date: 2026-02-14 | Depends on: V1 complete (all phases ✅)
> v2: Incorporates review feedback — simplified ingestion path, pagination, async ingest, CORS-free proxy, corrected HTTP semantics.

## Problem

The V1 system is API-only + CLI. Users must use `curl` to query and the Python CLI to ingest documents. There is no way to:
- Browse or search ingested documents
- Upload new documents via browser
- Deactivate documents
- View ingestion run history/status
- Chat with the knowledge base visually
- See chunk-level detail for debugging

## Tech Stack Decision

**Next.js 15 (App Router) + TypeScript + Tailwind CSS + shadcn/ui**

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Framework | Next.js 15 (App Router) | SSR, API routes as BFF proxy, file-based routing |
| Language | TypeScript | Type safety for API contracts |
| Styling | Tailwind CSS | Utility-first, fast iteration |
| Components | shadcn/ui | Accessible, customizable, dark mode built-in |
| HTTP Client | fetch (native) | No extra deps needed |
| State | React state + URL params | Simple, no Redux needed for V1 |
| Container | Node 20 Alpine | Small image, fast startup |

**Alternatives considered:**
- Single HTML file (served by Go): Too limited for document management, file uploads, routing.
- Vite + React SPA: No SSR, no API routes for BFF proxy, more manual setup.
- Vue/Nuxt: Viable, but React/Next.js has broader ecosystem and better TypeScript support.

## Architecture

```mermaid
graph TB
    subgraph Browser - port 3000
        PAGES[Next.js Pages]
    end

    subgraph Next.js Server - BFF
        API_ROUTES[API Routes - proxy layer]
    end

    subgraph Backend Services
        GO[core-api-go :8000 - queries + read APIs]
        INGEST_API[ingest-api :8002 - file upload + ingestion]
        EMBED[embed-svc :8001]
        PG[Postgres]
    end

    PAGES -->|all API calls| API_ROUTES
    API_ROUTES -->|queries, documents, runs| GO
    API_ROUTES -->|file upload + ingest| INGEST_API
    GO -->|read| PG
    GO -->|embed questions| EMBED
    INGEST_API -->|write| PG
```

**Key design decisions:**

1. **All browser API calls go through Next.js API routes (BFF proxy).** No direct browser→Go or browser→Python calls. This eliminates CORS entirely and gives us a single entry point for auth in V2.

2. **Ingestion goes directly from Next.js to Python ingest-api.** The Go service is not in the ingestion path — it never was. The Option A contract says "Python writes, Go reads." Proxying writes through Go would violate the spirit of that contract and add a failure mode for no benefit.

3. **Async ingestion.** `POST /ingest` returns immediately with `{status: "processing", run_id: "..."}`. The UI polls `GET /v1/ingestion-runs/:id` for completion. The `ingestion_runs` table already supports this.

## Pages and Features

### 1. Dashboard (`/`)
- Document count, chunk count, total tokens
- Recent ingestion runs (last 5) with status badges
- Quick-action buttons: Upload Document, Ask Question
- System health indicator (calls `GET /health`)

### 2. Documents (`/documents`)
- Table view: title, source type, status (active/inactive), chunk count, created date
- Search/filter by title
- Sort by date, title, chunk count
- Pagination (20 per page)
- Click row → document detail page
- "Upload New" button → `/documents/new`

### 3. Upload Document (`/documents/new`)
- Drag-and-drop file upload (DOCX, PDF, HTML)
- **Max file size: 50MB** (enforced in both Next.js API route and ingest-api)
- Title input (auto-populated from filename)
- Upload progress indicator
- **Async flow:** submit → show "Processing..." with spinner → poll ingestion run status → on success: link to document detail; on failure: show error with details
- Supported formats shown clearly

### 4. Document Detail (`/documents/:id`)
- Document metadata: title, source type, source URI, content hash, created date
- Active version info: version label, effective date, chunk count, total tokens
- Version history (all versions, active highlighted)
- **Chunk browser** (separate paginated section, 50 per page):
  - Ordinal, heading path, text preview (truncated), token count
  - **Token count visualization:** color-coded tag (green <400, yellow 400-600, orange 600-800, red =800 cap)
  - Click to expand full chunk text
- Actions: deactivate document (with confirmation dialog)

### 5. Chat (`/chat`)
- Full-screen chat interface
- **Message history within session (client-side only)**
  - Known V1 limitation: refreshing the page loses history
  - V2: persist conversations in DB with `conversation_id`
- Inline `[chunk:...]` citation markers highlighted and clickable
- Citations panel: source doc title, heading path, chunk text preview
- Debug panel (collapsible): vec/FTS candidates, reranker info, top scores, latency breakdown
- Abstain responses styled with orange badge + clarifying question
- Example questions for empty state
- **Tenant ID:** hardcoded to test tenant `00000000-0000-0000-0000-000000000001` in V1 (no selector UI). V2: add `GET /v1/tenants` and tenant picker.
- **Error states:**
  - LLM unavailable (502): "The AI service is temporarily unavailable. Please try again in a moment."
  - Timeout (no response in 30s): show timeout message + retry button
  - Network error: connection indicator banner at top
  - Bad request (400): show validation error message

### 6. Ingestion Runs (`/ingestion`)
- Table view: run ID (short), status badge (running/succeeded/failed), started, finished, duration
- Stats columns: docs processed, chunks created, tokens total
- Error details expandable for failed runs
- **Auto-refresh every 5s when any run has status=running**
- Pagination (20 per page)

## Backend APIs

### Existing (no changes)
- `GET /health` — health check
- `POST /v1/query` — query the knowledge base

### New: core-api-go (read-only + deactivate)

All endpoints require `tenant_id` query parameter. All return JSON.

#### `GET /v1/documents?tenant_id=...&page=1&limit=20&search=...`

```json
{
  "documents": [
    {
      "doc_id": "uuid",
      "title": "Acme Corp IT Security Policy",
      "source_type": "docx",
      "source_uri": "file:///data/test-corpus/it_security_policy.docx",
      "content_hash": "sha256...",
      "created_at": "2026-02-14T...",
      "active_version": {
        "doc_version_id": "uuid",
        "version_label": "v1",
        "effective_at": "2026-02-14T...",
        "chunk_count": 14,
        "total_tokens": 1477
      }
    }
  ],
  "total": 15,
  "page": 1,
  "limit": 20
}
```

#### `GET /v1/documents/:id?tenant_id=...`

Returns document metadata + version history. **Chunks are NOT inline** — use the chunks endpoint.

```json
{
  "doc_id": "uuid",
  "title": "...",
  "source_type": "docx",
  "source_uri": "...",
  "content_hash": "...",
  "created_at": "...",
  "versions": [
    {
      "doc_version_id": "uuid",
      "version_label": "v1",
      "is_active": true,
      "effective_at": "...",
      "chunk_count": 14,
      "total_tokens": 1477,
      "created_at": "..."
    }
  ]
}
```

#### `GET /v1/documents/:id/chunks?tenant_id=...&page=1&limit=50&version_id=...`

Paginated chunk list for a document. Defaults to the active version. Pass optional `version_id` to browse chunks for a specific (possibly inactive) version — useful for debugging version history.

```json
{
  "chunks": [
    {
      "chunk_id": "uuid",
      "ordinal": 1,
      "heading_path": "Introduction",
      "chunk_type": "text",
      "text": "...",
      "token_count": 120,
      "metadata": {}
    }
  ],
  "total": 14,
  "page": 1,
  "limit": 50
}
```

#### `POST /v1/documents/:id/deactivate?tenant_id=...`

Deactivates the active version (sets `is_active = false`). Does not delete data.

```json
{"status": "deactivated", "doc_id": "uuid", "doc_version_id": "uuid"}
```

#### `GET /v1/ingestion-runs?tenant_id=...&page=1&limit=20`

```json
{
  "runs": [
    {
      "run_id": "uuid",
      "status": "succeeded",
      "started_at": "...",
      "finished_at": "...",
      "duration_ms": 4500,
      "config": {},
      "stats": {"docs_processed": 1, "chunks_created": 14, "tokens_total": 1477},
      "error": null
    }
  ],
  "total": 5,
  "page": 1,
  "limit": 20
}
```

#### `GET /v1/ingestion-runs/:id?tenant_id=...`

Single ingestion run detail (used for polling during async ingest).

```json
{
  "run_id": "uuid",
  "status": "succeeded",
  "started_at": "...",
  "finished_at": "...",
  "duration_ms": 4500,
  "config": {},
  "stats": {"docs_processed": 1, "chunks_created": 14, "tokens_total": 1477},
  "error": null
}
```

### New: ingest-api (Python — HTTP wrapper around existing pipeline)

#### `POST /ingest` (multipart form)

Accepts file upload + metadata, triggers ingestion pipeline **asynchronously**.

Request:
```
Content-Type: multipart/form-data
- file: <binary> (max 50MB)
- tenant_id: uuid
- title: string
```

Response (immediate — pipeline runs in background):
```json
{
  "status": "processing",
  "run_id": "uuid",
  "doc_id": "uuid"
}
```

The caller polls `GET /v1/ingestion-runs/:run_id` for completion.

#### `GET /health`
```json
{"status": "ok"}
```

## Implementation Phases

```mermaid
graph LR
    P7A[Phase 7a: Go Management APIs] --> P7D[Phase 7d: Next.js Scaffold + Documents]
    P7B[Phase 7b: Python Ingest API] --> P7D_UPLOAD[Phase 7d upload page only]
    P7A --> P7D_BROWSE[Phase 7d document browsing]
    P7D_UPLOAD --> P7E[Phase 7e: Chat Page]
    P7D_BROWSE --> P7E
    P7D --> P7E[Phase 7e: Chat Page]
    P7E --> P7F[Phase 7f: Dashboard + Ingestion History]
    P7F --> P7G[Phase 7g: Polish + Integration Testing]
```

**Note:** Phase 7a and 7b can run in parallel (no dependencies between them).

### Phase 7a: Backend Management APIs (Go)

Add read-only document/ingestion APIs to core-api-go. Pure DB reads — no new external dependencies.

**Endpoints:**
- `GET /v1/documents` — list with pagination + search
- `GET /v1/documents/:id` — document detail with versions
- `GET /v1/documents/:id/chunks` — paginated chunks
- `POST /v1/documents/:id/deactivate` — soft deactivate
- `GET /v1/ingestion-runs` — list with pagination
- `GET /v1/ingestion-runs/:id` — single run detail

**Files:**
- `core-api-go/internal/handler/documents.go` — document handlers
- `core-api-go/internal/handler/ingestion.go` — ingestion run handlers
- `core-api-go/internal/handler/documents_test.go` — tests
- `core-api-go/internal/handler/ingestion_test.go` — tests

**DoD:** `make api-test` passes with new endpoint tests.

### Phase 7b: Python Ingest HTTP Wrapper (parallel with 7a)

Thin FastAPI service wrapping the existing `ingest.pipeline.ingest_document`.

**Files:**
- `ingest-api/app.py` — FastAPI app (~80 lines)
- `ingest-api/Dockerfile`
- `ingest-api/requirements.txt`
- Docker Compose service on port 8002

**Behavior:**
- `POST /ingest`: save uploaded file to temp dir, create `ingestion_runs` row with status=running, spawn background task to run pipeline, return `{status: "processing", run_id, doc_id}` immediately
- Background task: run `ingest_document()`, update `ingestion_runs` on success/failure
- **File size limit: 50MB** (enforced via FastAPI `UploadFile` config)
- **Crash guard on startup:** On service start, find any `ingestion_runs` with `status='running'` older than 10 minutes and mark them as `status='failed'` with `error='interrupted — service restarted'`. This prevents permanently-spinning ingestion indicators in the UI after a container restart.
- Health check: `GET /health`

**DoD:** Can upload a file via curl and see it appear in the DB after polling the run status.

### Phase 7d: Next.js Scaffold + Document Management

Set up the Next.js project and build document management first (so the full upload→browse→query flow works).

**Includes:**
- Next.js 15 + TypeScript + Tailwind + shadcn/ui scaffold
- Docker Compose service on port 3000 (depends on core-api-go: `service_healthy`)
- API client module (`lib/api.ts`) with typed fetch wrappers
- Shared types (`lib/types.ts`) — manually maintained, V2: OpenAPI generation
- **All API calls proxied through Next.js API routes** (no CORS)
- Layout with sidebar navigation
- Documents list page (`/documents`)
- Document detail page (`/documents/:id`) with chunk browser + token visualization
- Upload page (`/documents/new`) with drag-and-drop + async polling

**DoD:** Can upload a document in the browser, see it in the list, browse its chunks.

### Phase 7e: Chat Page

Build the chat interface. Documents are already available from Phase 7d.

**Includes:**
- Chat page (`/chat`) — full interface with citations, debug panel, error states
- Abstain styling, citation highlighting, example questions
- Error handling: LLM unavailable, timeout, network error
- Hardcoded tenant ID (V1)

**DoD:** Can ask questions and get answers with citations in the browser.

### Phase 7f: Dashboard + Ingestion History

**Includes:**
- Dashboard (`/`) — stats cards, recent runs, quick actions, health indicator
- Ingestion runs page (`/ingestion`) — table with auto-refresh for running jobs
- Stats API calls (document count, chunk count, total tokens)

**DoD:** Dashboard shows accurate stats, ingestion page shows run history with auto-refresh.

### Phase 7g: Polish + Integration Testing

**Includes:**
- E2E test: upload document → verify in list → query about it → see citations
- Responsive layout (mobile-friendly)
- Loading states, error states, empty states for all pages
- Keyboard shortcuts (Enter to send in chat)
- Update README.md and docs
- `make web-dev` and `make web-build` targets

**DoD:** Full flow works end-to-end, all states handled, docs updated.

## File Structure

```
web/
├── package.json
├── tsconfig.json
├── tailwind.config.ts
├── next.config.ts
├── Dockerfile
├── app/
│   ├── layout.tsx              # Root layout with sidebar nav
│   ├── page.tsx                # Dashboard
│   ├── chat/
│   │   └── page.tsx            # Chat interface
│   ├── documents/
│   │   ├── page.tsx            # Document list
│   │   ├── new/
│   │   │   └── page.tsx        # Upload form
│   │   └── [id]/
│   │       └── page.tsx        # Document detail + chunks
│   ├── ingestion/
│   │   └── page.tsx            # Ingestion run history
│   └── api/                    # BFF proxy routes
│       ├── query/
│       │   └── route.ts        # Proxy to POST /v1/query
│       ├── documents/
│       │   └── route.ts        # Proxy to GET /v1/documents
│       ├── documents/[id]/
│       │   ├── route.ts        # Proxy to GET /v1/documents/:id
│       │   ├── chunks/
│       │   │   └── route.ts    # Proxy to GET /v1/documents/:id/chunks
│       │   └── deactivate/
│       │       └── route.ts    # Proxy to POST /v1/documents/:id/deactivate
│       ├── ingest/
│       │   └── route.ts        # Proxy to POST ingest-api/ingest
│       ├── ingestion-runs/
│       │   └── route.ts        # Proxy to GET /v1/ingestion-runs
│       └── health/
│           └── route.ts        # Proxy to GET /health
├── components/
│   ├── ui/                     # shadcn/ui components
│   ├── chat/
│   │   ├── chat-input.tsx
│   │   ├── chat-message.tsx
│   │   ├── citation-panel.tsx
│   │   └── debug-panel.tsx
│   ├── documents/
│   │   ├── document-table.tsx
│   │   ├── chunk-browser.tsx
│   │   └── upload-form.tsx
│   ├── layout/
│   │   ├── sidebar.tsx
│   │   └── header.tsx
│   └── dashboard/
│       └── stats-cards.tsx
├── lib/
│   ├── api.ts                  # Typed API client (calls /api/* routes)
│   └── types.ts                # Shared TypeScript types
└── public/
```

## Docker Compose Additions

```yaml
# Python ingest API (HTTP wrapper)
ingest-api:
  build:
    context: ./ingest-api
    dockerfile: Dockerfile
  container_name: prorag-ingest-api
  depends_on:
    postgres:
      condition: service_healthy
  environment:
    DATABASE_URL: postgres://prorag:prorag_dev@postgres:5432/prorag?sslmode=disable
    EMBEDDING_MODEL: BAAI/bge-base-en-v1.5
    EMBEDDING_DIM: 768
    MAX_UPLOAD_SIZE_MB: 50
  ports:
    - "8002:8002"
  volumes:
    - ./data:/data
    - artifacts:/data/artifacts
  healthcheck:
    test: ["CMD", "python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8002/health')"]
    interval: 5s
    timeout: 10s
    retries: 6

# Next.js Web UI
web:
  build:
    context: ./web
    dockerfile: Dockerfile
  container_name: prorag-web
  depends_on:
    core-api-go:
      condition: service_healthy
    ingest-api:
      condition: service_healthy
  environment:
    API_URL: http://core-api-go:8000
    INGEST_API_URL: http://ingest-api:8002
  ports:
    - "3000:3000"
```

## Known V1 Limitations

1. **Chat history is client-side only.** Refreshing the page loses conversation. V2: persist in DB with `conversation_id`.
2. **Single tenant.** Hardcoded to test tenant UUID. V2: add `GET /v1/tenants` and tenant picker UI.
3. **Type sharing is manual.** TypeScript types and Go structs maintained separately. V2: OpenAPI spec generation from Go structs.
4. **No auth.** V1 is local/internal only. V2: API key/JWT bound to tenant.
5. **Synchronous chunk browsing.** For documents with 500+ chunks, pagination handles it but there's no lazy loading. Acceptable for V1 scale (10-50 docs).

## Definition of Done (all phases)

- All 6 pages render and function correctly
- Upload a document via browser → appears in document list → queryable in chat
- Chat shows answers with clickable citations and debug info
- Document deactivation works (deactivated docs not returned in queries)
- Ingestion history shows run status with auto-refresh
- Chunk browser shows token count visualization
- Error states handled: LLM down, timeout, network error, validation errors
- Dark theme, responsive layout
- `make api-test` passes (new Go endpoints tested)
- README.md updated with web UI instructions
