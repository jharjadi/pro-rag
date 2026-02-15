// Shared TypeScript types for the pro-rag web UI.
// Manually maintained — V2: OpenAPI generation from Go structs.

// ── Documents ────────────────────────────────────────────

export interface ActiveVersionSummary {
  doc_version_id: string;
  version_label: string;
  effective_at: string;
  chunk_count: number;
  total_tokens: number;
}

export interface DocumentListItem {
  doc_id: string;
  title: string;
  source_type: string;
  source_uri: string;
  content_hash: string;
  created_at: string;
  active_version: ActiveVersionSummary | null;
}

export interface DocumentListResponse {
  documents: DocumentListItem[];
  total: number;
  page: number;
  limit: number;
}

export interface VersionDetail {
  doc_version_id: string;
  version_label: string;
  is_active: boolean;
  effective_at: string;
  chunk_count: number;
  total_tokens: number;
  created_at: string;
}

export interface DocumentDetailResponse {
  doc_id: string;
  title: string;
  source_type: string;
  source_uri: string;
  content_hash: string;
  created_at: string;
  versions: VersionDetail[];
}

export interface ChunkListItem {
  chunk_id: string;
  ordinal: number;
  heading_path: string[] | null;
  chunk_type: string;
  text: string;
  token_count: number;
  metadata: Record<string, unknown>;
}

export interface ChunkListResponse {
  chunks: ChunkListItem[];
  total: number;
  page: number;
  limit: number;
}

export interface DeactivateResponse {
  status: string;
  doc_id: string;
  doc_version_id: string;
}

// ── Ingestion ────────────────────────────────────────────

export interface IngestionRunItem {
  run_id: string;
  doc_id: string | null;
  status: "queued" | "running" | "succeeded" | "failed";
  run_type: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
  duration_ms: number | null;
  config: Record<string, unknown>;
  stats: {
    docs_processed?: number;
    chunks_created?: number;
    tokens_total?: number;
  };
  error: string | null;
}

export interface IngestionRunListResponse {
  runs: IngestionRunItem[];
  total: number;
  page: number;
  limit: number;
}

export interface IngestResponse {
  run_id?: string;
  doc_id: string;
  status: string;
  reason?: string;
}

// ── Query ────────────────────────────────────────────────

export interface Citation {
  doc_id: string;
  doc_version_id: string;
  chunk_id: string;
  title: string;
  version_label: string;
}

export interface DebugInfo {
  vec_candidates: number;
  fts_candidates: number;
  merged_candidates: number;
  reranker_used: boolean;
  reranker_skipped: boolean;
  reranker_error?: string;
  top_scores?: number[];
  context_chunks: number;
  context_tokens_est: number;
}

export interface QueryResponse {
  answer: string;
  citations: Citation[];
  abstained: boolean;
  clarifying_question?: string;
  debug?: DebugInfo;
}

export interface ErrorResponse {
  error: string;
  message: string;
}
