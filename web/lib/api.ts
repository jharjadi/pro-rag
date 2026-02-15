// Typed API client for pro-rag web UI.
// All calls go through Next.js API routes (BFF proxy) → Go API gateway.

import type {
  ChunkListResponse,
  DeactivateResponse,
  DocumentDetailResponse,
  DocumentListResponse,
  IngestResponse,
  IngestionRunItem,
  IngestionRunListResponse,
  QueryResponse,
} from "./types";

const TENANT_ID = "00000000-0000-0000-0000-000000000001"; // V1: hardcoded

class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
    this.name = "ApiError";
  }
}

async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: "unknown", message: res.statusText }));
    throw new ApiError(res.status, body.error || "unknown", body.message || res.statusText);
  }
  return res.json() as Promise<T>;
}

// ── Documents ────────────────────────────────────────────

export async function listDocuments(
  page = 1,
  limit = 20,
  search = ""
): Promise<DocumentListResponse> {
  const params = new URLSearchParams({
    tenant_id: TENANT_ID,
    page: String(page),
    limit: String(limit),
  });
  if (search) params.set("search", search);
  return apiFetch<DocumentListResponse>(`/api/documents?${params}`);
}

export async function getDocument(id: string): Promise<DocumentDetailResponse> {
  return apiFetch<DocumentDetailResponse>(
    `/api/documents/${id}?tenant_id=${TENANT_ID}`
  );
}

export async function listChunks(
  docId: string,
  page = 1,
  limit = 50,
  versionId?: string
): Promise<ChunkListResponse> {
  const params = new URLSearchParams({
    tenant_id: TENANT_ID,
    page: String(page),
    limit: String(limit),
  });
  if (versionId) params.set("version_id", versionId);
  return apiFetch<ChunkListResponse>(
    `/api/documents/${docId}/chunks?${params}`
  );
}

export async function deactivateDocument(id: string): Promise<DeactivateResponse> {
  return apiFetch<DeactivateResponse>(
    `/api/documents/${id}/deactivate?tenant_id=${TENANT_ID}`,
    { method: "POST" }
  );
}

// ── Ingestion ────────────────────────────────────────────

export async function uploadDocument(
  file: File,
  title: string
): Promise<IngestResponse> {
  const formData = new FormData();
  formData.append("file", file);
  formData.append("title", title);

  // Pass tenant_id as query param (auth middleware reads from query params in dev mode)
  return apiFetch<IngestResponse>(`/api/ingest?tenant_id=${TENANT_ID}`, {
    method: "POST",
    body: formData,
  });
}

export async function listIngestionRuns(
  page = 1,
  limit = 20
): Promise<IngestionRunListResponse> {
  const params = new URLSearchParams({
    tenant_id: TENANT_ID,
    page: String(page),
    limit: String(limit),
  });
  return apiFetch<IngestionRunListResponse>(`/api/ingestion-runs?${params}`);
}

export async function getIngestionRun(id: string): Promise<IngestionRunItem> {
  return apiFetch<IngestionRunItem>(
    `/api/ingestion-runs/${id}?tenant_id=${TENANT_ID}`
  );
}

// ── Query ────────────────────────────────────────────────

export async function queryKnowledgeBase(
  question: string,
  debug = false
): Promise<QueryResponse> {
  return apiFetch<QueryResponse>("/api/query", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      tenant_id: TENANT_ID,
      question,
      top_k: 10,
      debug,
    }),
  });
}

// ── Health ───────────────────────────────────────────────

export async function checkHealth(): Promise<{ status: string }> {
  return apiFetch<{ status: string }>("/api/health");
}

export { ApiError };
