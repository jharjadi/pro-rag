package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jharjadi/pro-rag/core-api-go/internal/config"
	authmw "github.com/jharjadi/pro-rag/core-api-go/internal/middleware"
)

// withAuthContext injects tenant_id, user_id, and role into the request context,
// simulating what the auth middleware does.
func withAuthContext(r *http.Request, tenantID, userID, role string) *http.Request {
	ctx := context.WithValue(r.Context(), authmw.ContextKeyTenantID, tenantID)
	ctx = context.WithValue(ctx, authmw.ContextKeyUserID, userID)
	ctx = context.WithValue(ctx, authmw.ContextKeyRole, role)
	return r.WithContext(ctx)
}

// ── delegateToWorker tests ───────────────────────────────

func TestDelegateToWorker_Success(t *testing.T) {
	// Mock worker returns 202 Accepted
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/internal/process" {
			t.Errorf("expected /internal/process, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}

		// Verify payload
		var payload jobPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}
		if payload.RunID != "run-123" {
			t.Errorf("expected run_id=run-123, got %s", payload.RunID)
		}
		if payload.TenantID != "tenant-abc" {
			t.Errorf("expected tenant_id=tenant-abc, got %s", payload.TenantID)
		}

		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, `{"status":"accepted","run_id":"run-123"}`)
	}))
	defer worker.Close()

	cfg := &config.Config{
		IngestWorkerURL:       worker.URL,
		IngestWorkerTimeoutMS: 5000,
		InternalAuthToken:     "test-token",
	}

	h := &IngestHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.IngestWorkerTimeout()},
	}

	payload := jobPayload{
		JobType:     "upload_ingest",
		RunID:       "run-123",
		TenantID:    "tenant-abc",
		DocID:       "doc-456",
		UploadURI:   "file:///data/uploads/tenant-abc/run-123/test.pdf",
		Title:       "Test Document",
		SourceType:  "pdf",
		SourceURI:   "upload://sha256:abc123",
		ContentHash: "sha256:abc123",
		SubmittedAt: time.Now().UTC().Format(time.RFC3339),
	}

	err := h.delegateToWorker(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestDelegateToWorker_WorkerBusy503(t *testing.T) {
	// Mock worker returns 503 Service Unavailable (all slots occupied)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"worker busy"}`)
	}))
	defer worker.Close()

	cfg := &config.Config{
		IngestWorkerURL:       worker.URL,
		IngestWorkerTimeoutMS: 5000,
		InternalAuthToken:     "test-token",
	}

	h := &IngestHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.IngestWorkerTimeout()},
	}

	payload := jobPayload{
		RunID:    "run-busy",
		TenantID: "tenant-abc",
	}

	err := h.delegateToWorker(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error for 503, got nil")
	}
	if err.Error() != "worker busy, all slots occupied" {
		t.Errorf("expected 'worker busy, all slots occupied', got: %s", err.Error())
	}
}

func TestDelegateToWorker_WorkerUnavailable(t *testing.T) {
	// No server running — connection refused
	cfg := &config.Config{
		IngestWorkerURL:       "http://127.0.0.1:19999", // nothing listening
		IngestWorkerTimeoutMS: 1000,
		InternalAuthToken:     "test-token",
	}

	h := &IngestHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.IngestWorkerTimeout()},
	}

	payload := jobPayload{
		RunID:    "run-unavail",
		TenantID: "tenant-abc",
	}

	err := h.delegateToWorker(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error for unavailable worker, got nil")
	}
	// Error should mention "worker unavailable"
	if got := err.Error(); len(got) < 10 {
		t.Errorf("expected descriptive error, got: %s", got)
	}
}

func TestDelegateToWorker_WorkerReturnsUnexpectedStatus(t *testing.T) {
	// Mock worker returns 500
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"internal server error"}`)
	}))
	defer worker.Close()

	cfg := &config.Config{
		IngestWorkerURL:       worker.URL,
		IngestWorkerTimeoutMS: 5000,
		InternalAuthToken:     "",
	}

	h := &IngestHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.IngestWorkerTimeout()},
	}

	payload := jobPayload{
		RunID:    "run-500",
		TenantID: "tenant-abc",
	}

	err := h.delegateToWorker(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	expected := `worker returned 500: {"error":"internal server error"}`
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestDelegateToWorker_NoAuthToken(t *testing.T) {
	// When InternalAuthToken is empty, no Authorization header should be sent
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Errorf("expected no Authorization header, got %s", auth)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer worker.Close()

	cfg := &config.Config{
		IngestWorkerURL:       worker.URL,
		IngestWorkerTimeoutMS: 5000,
		InternalAuthToken:     "", // empty = no auth
	}

	h := &IngestHandler{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.IngestWorkerTimeout()},
	}

	err := h.delegateToWorker(context.Background(), jobPayload{RunID: "run-noauth"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// ── Streaming SHA-256 + content-addressed source_uri tests ─

func TestStreamingSHA256_ContentAddressedURI(t *testing.T) {
	// Simulate the streaming SHA-256 pattern from the handler:
	// io.TeeReader(file, hasher) → io.Copy(dst, tee)
	content := []byte("This is a test PDF file content for hashing")

	hasher := sha256.New()
	tee := io.TeeReader(bytes.NewReader(content), hasher)

	var dst bytes.Buffer
	written, err := io.Copy(&dst, tee)
	if err != nil {
		t.Fatalf("io.Copy failed: %v", err)
	}

	if written != int64(len(content)) {
		t.Errorf("expected %d bytes written, got %d", len(content), written)
	}

	contentHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	sourceURI := "upload://" + contentHash

	// Verify deterministic: same content → same hash
	expectedHash := sha256.Sum256(content)
	expectedContentHash := "sha256:" + hex.EncodeToString(expectedHash[:])
	expectedSourceURI := "upload://" + expectedContentHash

	if contentHash != expectedContentHash {
		t.Errorf("content hash mismatch: got %s, want %s", contentHash, expectedContentHash)
	}
	if sourceURI != expectedSourceURI {
		t.Errorf("source URI mismatch: got %s, want %s", sourceURI, expectedSourceURI)
	}

	// Verify the written content matches the original
	if !bytes.Equal(dst.Bytes(), content) {
		t.Error("written content does not match original")
	}
}

func TestStreamingSHA256_DifferentContentDifferentHash(t *testing.T) {
	content1 := []byte("Version 1 of the document")
	content2 := []byte("Version 2 of the document")

	hash1 := sha256.Sum256(content1)
	hash2 := sha256.Sum256(content2)

	uri1 := "upload://sha256:" + hex.EncodeToString(hash1[:])
	uri2 := "upload://sha256:" + hex.EncodeToString(hash2[:])

	if uri1 == uri2 {
		t.Error("different content should produce different source URIs")
	}
}

func TestStreamingSHA256_IdenticalContentIdenticalHash(t *testing.T) {
	content := []byte("Identical content uploaded twice")

	hash1 := sha256.Sum256(content)
	hash2 := sha256.Sum256(content)

	uri1 := "upload://sha256:" + hex.EncodeToString(hash1[:])
	uri2 := "upload://sha256:" + hex.EncodeToString(hash2[:])

	if uri1 != uri2 {
		t.Error("identical content should produce identical source URIs")
	}
}

// ── Allowed extensions validation tests ──────────────────

func TestAllowedExtensions(t *testing.T) {
	tests := []struct {
		ext     string
		allowed bool
	}{
		{".pdf", true},
		{".docx", true},
		{".html", true},
		{".htm", true},
		{".txt", false},
		{".exe", false},
		{".xlsx", false},
		{".csv", false},
		{".doc", false},
		{".pptx", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			_, ok := allowedExtensions[tt.ext]
			if ok != tt.allowed {
				t.Errorf("extension %s: expected allowed=%v, got %v", tt.ext, tt.allowed, ok)
			}
		})
	}
}

// ── Job payload construction tests ───────────────────────

func TestJobPayload_JSONSerialization(t *testing.T) {
	payload := jobPayload{
		JobType:          "upload_ingest",
		RunID:            "run-uuid-123",
		DocID:            "doc-uuid-456",
		TenantID:         "tenant-uuid-789",
		UploadURI:        "file:///data/uploads/tenant-uuid-789/run-uuid-123/test.pdf",
		Title:            "Test Document",
		SourceType:       "pdf",
		SourceURI:        "upload://sha256:abcdef1234567890",
		OriginalFilename: "test.pdf",
		ContentHash:      "sha256:abcdef1234567890",
		EmbeddingModel:   "BAAI/bge-base-en-v1.5",
		EmbeddingDim:     768,
		SubmittedAt:      "2026-02-15T00:00:00Z",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	var decoded jobPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if decoded.JobType != "upload_ingest" {
		t.Errorf("job_type: got %s, want upload_ingest", decoded.JobType)
	}
	if decoded.RunID != "run-uuid-123" {
		t.Errorf("run_id: got %s, want run-uuid-123", decoded.RunID)
	}
	if decoded.EmbeddingDim != 768 {
		t.Errorf("embedding_dim: got %d, want 768", decoded.EmbeddingDim)
	}
	if decoded.ContentHash != "sha256:abcdef1234567890" {
		t.Errorf("content_hash: got %s, want sha256:abcdef1234567890", decoded.ContentHash)
	}
}

func TestJobPayload_AllFieldsPresent(t *testing.T) {
	payload := jobPayload{
		JobType:          "upload_ingest",
		RunID:            "r1",
		DocID:            "d1",
		TenantID:         "t1",
		UploadURI:        "file:///data/uploads/t1/r1/f.pdf",
		Title:            "Title",
		SourceType:       "pdf",
		SourceURI:        "upload://sha256:abc",
		OriginalFilename: "f.pdf",
		ContentHash:      "sha256:abc",
		EmbeddingModel:   "BAAI/bge-base-en-v1.5",
		EmbeddingDim:     768,
		SubmittedAt:      "2026-02-15T00:00:00Z",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	requiredFields := []string{
		"job_type", "run_id", "doc_id", "tenant_id", "upload_uri",
		"title", "source_type", "source_uri", "original_filename",
		"content_hash", "embedding_model", "embedding_dim", "submitted_at",
	}

	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("missing required field in JSON: %s", field)
		}
	}
}

// ── Ingest handler validation tests (full HTTP) ──────────

func TestIngest_MissingTenantID(t *testing.T) {
	cfg := &config.Config{
		UploadMaxSizeMB: 50,
	}
	h := &IngestHandler{
		cfg:    cfg,
		pool:   nil, // will fail if DB is reached
		client: &http.Client{},
	}

	// Create a multipart request without tenant_id in context (no auth middleware)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.pdf")
	part.Write([]byte("fake pdf content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// No auth context injected — tenant_id will be empty

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["message"] != "tenant_id is required" {
		t.Errorf("expected 'tenant_id is required', got %q", errResp["message"])
	}
}

func TestIngest_MissingFile(t *testing.T) {
	cfg := &config.Config{
		UploadMaxSizeMB: 50,
	}
	h := &IngestHandler{
		cfg:    cfg,
		pool:   nil,
		client: &http.Client{},
	}

	// Create a multipart request with no file — tenant_id injected via context
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = withAuthContext(req, "00000000-0000-0000-0000-000000000001", "dev-user", "admin")

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["message"] != "file field is required" {
		t.Errorf("expected 'file field is required', got %q", errResp["message"])
	}
}

func TestIngest_UnsupportedExtension(t *testing.T) {
	cfg := &config.Config{
		UploadMaxSizeMB: 50,
	}
	h := &IngestHandler{
		cfg:    cfg,
		pool:   nil,
		client: &http.Client{},
	}

	// Create a multipart request with an unsupported file extension
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("plain text content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/ingest", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = withAuthContext(req, "00000000-0000-0000-0000-000000000001", "dev-user", "admin")

	rr := httptest.NewRecorder()
	h.Ingest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	var errResp map[string]string
	json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp["error"] != "bad_request" {
		t.Errorf("expected error code 'bad_request', got %q", errResp["error"])
	}
}

// ── Ingest response type tests ───────────────────────────

func TestIngestResponse_SkippedJSON(t *testing.T) {
	resp := ingestResponse{
		DocID:  "doc-123",
		Status: "skipped",
		Reason: "already ingested, no changes",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["status"] != "skipped" {
		t.Errorf("status: got %v, want skipped", decoded["status"])
	}
	if decoded["reason"] != "already ingested, no changes" {
		t.Errorf("reason: got %v, want 'already ingested, no changes'", decoded["reason"])
	}
	// run_id should be omitted when empty
	if _, ok := decoded["run_id"]; ok {
		t.Error("run_id should be omitted for skipped response")
	}
}

func TestIngestResponse_QueuedJSON(t *testing.T) {
	resp := ingestResponse{
		RunID:  "run-456",
		DocID:  "doc-789",
		Status: "queued",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["status"] != "queued" {
		t.Errorf("status: got %v, want queued", decoded["status"])
	}
	if decoded["run_id"] != "run-456" {
		t.Errorf("run_id: got %v, want run-456", decoded["run_id"])
	}
	// reason should be omitted when empty
	if _, ok := decoded["reason"]; ok {
		t.Error("reason should be omitted for queued response")
	}
}
