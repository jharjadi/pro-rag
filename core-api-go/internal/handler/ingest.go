package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jharjadi/pro-rag/core-api-go/internal/config"
	authmw "github.com/jharjadi/pro-rag/core-api-go/internal/middleware"
)

// Allowed file extensions for upload.
var allowedExtensions = map[string]string{
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".html": "text/html",
	".htm":  "text/html",
}

// IngestHandler handles POST /v1/ingest — Go-orchestrated ingestion (spec v2.3 §4.1).
type IngestHandler struct {
	cfg    *config.Config
	pool   *pgxpool.Pool
	client *http.Client
}

// NewIngestHandler creates a new IngestHandler.
func NewIngestHandler(cfg *config.Config, pool *pgxpool.Pool) *IngestHandler {
	return &IngestHandler{
		cfg:  cfg,
		pool: pool,
		client: &http.Client{
			Timeout: cfg.IngestWorkerTimeout(),
		},
	}
}

// ingestResponse is the JSON response for POST /v1/ingest.
type ingestResponse struct {
	RunID  string `json:"run_id,omitempty"`
	DocID  string `json:"doc_id"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// jobPayload is the JSON payload sent to the ingest-worker (spec v2.3 §5).
type jobPayload struct {
	JobType          string `json:"job_type"`
	RunID            string `json:"run_id"`
	DocID            string `json:"doc_id"`
	TenantID         string `json:"tenant_id"`
	UploadURI        string `json:"upload_uri"`
	Title            string `json:"title"`
	SourceType       string `json:"source_type"`
	SourceURI        string `json:"source_uri"`
	OriginalFilename string `json:"original_filename"`
	ContentHash      string `json:"content_hash"`
	EmbeddingModel   string `json:"embedding_model"`
	EmbeddingDim     int    `json:"embedding_dim"`
	SubmittedAt      string `json:"submitted_at"`
}

// Ingest handles POST /v1/ingest (multipart/form-data).
// Spec v2.3 §4.1: Validate → Stream+Hash → Dedup → Create doc/run → Delegate → Respond.
func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// ── Get tenant_id + user_id from auth middleware context ─
	// Auth middleware handles both modes:
	//   AUTH_ENABLED=true  → derived from JWT claims
	//   AUTH_ENABLED=false → from query param (dev mode)
	// Spec v2.3 §2.1: tenant_id is derived from JWT claims, never trusted from browser payload.
	tenantID := authmw.TenantIDFromContext(ctx)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id is required")
		return
	}
	userID := authmw.UserIDFromContext(ctx)

	// ── Step 1: Validate ───────────────────────────────────
	// Parse multipart form with max size limit
	if err := r.ParseMultipartForm(h.cfg.UploadMaxSizeBytes()); err != nil {
		slog.Error("failed to parse multipart form", "error", err)
		writeError(w, http.StatusBadRequest, "bad_request", "invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "file field is required")
		return
	}
	defer file.Close()

	// Validate extension
	ext := strings.ToLower(filepath.Ext(header.Filename))
	contentType, ok := allowedExtensions[ext]
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request",
			fmt.Sprintf("unsupported file format: %s. Allowed: pdf, docx, html", ext))
		return
	}

	// Validate size
	if header.Size > h.cfg.UploadMaxSizeBytes() {
		writeError(w, http.StatusBadRequest, "bad_request",
			fmt.Sprintf("file too large: %d bytes. Max: %d MB", header.Size, h.cfg.UploadMaxSizeMB))
		return
	}

	// Get title (optional, defaults to filename)
	title := r.FormValue("title")
	if title == "" {
		title = strings.TrimSuffix(header.Filename, ext)
	}

	// ── Step 2: Stream file to disk + compute SHA-256 ──────
	runID := uuid.New().String()
	uploadDir := filepath.Join(h.cfg.UploadStorePath, tenantID, runID)
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		slog.Error("failed to create upload directory", "error", err, "path", uploadDir)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create upload directory")
		return
	}

	uploadPath := filepath.Join(uploadDir, header.Filename)
	dstFile, err := os.Create(uploadPath)
	if err != nil {
		slog.Error("failed to create upload file", "error", err, "path", uploadPath)
		writeError(w, http.StatusInternalServerError, "internal", "failed to save uploaded file")
		return
	}

	// Streaming SHA-256: io.TeeReader(file, hasher) → io.Copy(disk, tee)
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)
	written, err := io.Copy(dstFile, tee)
	dstFile.Close()
	if err != nil {
		slog.Error("failed to stream file to disk", "error", err)
		os.Remove(uploadPath)
		os.Remove(uploadDir)
		writeError(w, http.StatusInternalServerError, "internal", "failed to save uploaded file")
		return
	}

	contentHash := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	uploadURI := "file://" + uploadPath

	slog.Info("file uploaded",
		"filename", header.Filename,
		"size_bytes", written,
		"content_hash", contentHash,
		"upload_path", uploadPath,
	)

	// ── Step 3: Compute content-addressed source_uri ───────
	sourceURI := "upload://" + contentHash
	sourceType := strings.TrimPrefix(ext, ".")

	// ── Step 4: Get-or-create document + dedup check ───────
	docID, isNew, err := h.getOrCreateDocument(ctx, tenantID, sourceType, sourceURI, title)
	if err != nil {
		slog.Error("failed to get-or-create document", "error", err)
		os.Remove(uploadPath)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create document record")
		return
	}

	// Dedup check: if document exists, check active version hash
	if !isNew {
		activeHash, err := h.getActiveVersionHash(ctx, tenantID, docID)
		if err != nil {
			slog.Error("failed to check active version hash", "error", err)
			os.Remove(uploadPath)
			writeError(w, http.StatusInternalServerError, "internal", "failed to check document version")
			return
		}

		if activeHash == contentHash {
			// Skip — already ingested with same content
			os.Remove(uploadPath)
			os.Remove(uploadDir)

			slog.Info("ingest skipped — duplicate content",
				"doc_id", docID,
				"content_hash", contentHash,
				"tenant_id", tenantID,
			)

			writeJSON(w, http.StatusOK, ingestResponse{
				DocID:  docID,
				Status: "skipped",
				Reason: "already ingested, no changes",
			})
			return
		}
		// activeHash differs or no active version → proceed (new version or first processing)
	}

	// ── Step 5: Create ingestion_runs row ──────────────────
	runConfig := map[string]interface{}{
		"source_type":        sourceType,
		"upload_uri":         uploadURI,
		"title":              title,
		"original_filename":  header.Filename,
		"content_type":       contentType,
		"file_size_bytes":    written,
		"content_hash":       contentHash,
		"created_by_user_id": userID,
	}
	configJSON, _ := json.Marshal(runConfig)

	_, err = h.pool.Exec(ctx,
		`INSERT INTO ingestion_runs (run_id, tenant_id, doc_id, status, created_at, config, run_type)
		 VALUES ($1, $2, $3, 'queued', now(), $4, 'manual_upload')`,
		runID, tenantID, docID, configJSON,
	)
	if err != nil {
		slog.Error("failed to create ingestion run", "error", err)
		os.Remove(uploadPath)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create ingestion run")
		return
	}

	// ── Step 6: Delegate to worker ─────────────────────────
	payload := jobPayload{
		JobType:          "upload_ingest",
		RunID:            runID,
		DocID:            docID,
		TenantID:         tenantID,
		UploadURI:        uploadURI,
		Title:            title,
		SourceType:       sourceType,
		SourceURI:        sourceURI,
		OriginalFilename: header.Filename,
		ContentHash:      contentHash,
		EmbeddingModel:   "BAAI/bge-base-en-v1.5",
		EmbeddingDim:     768,
		SubmittedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	delegationErr := h.delegateToWorker(ctx, payload)
	if delegationErr != nil {
		// Mark run as failed
		slog.Error("worker delegation failed", "error", delegationErr, "run_id", runID)
		_, _ = h.pool.Exec(ctx,
			`UPDATE ingestion_runs SET status = 'failed', error = $2, finished_at = now(), updated_at = now()
			 WHERE run_id = $1`,
			runID, delegationErr.Error(),
		)

		writeError(w, http.StatusInternalServerError, "delegation_failed",
			fmt.Sprintf("ingestion job created but worker delegation failed: %s", delegationErr.Error()))
		return
	}

	// ── Step 7: Return response ────────────────────────────
	slog.Info("ingest job submitted",
		"event", "ingest_job_submitted",
		"tenant_id", tenantID,
		"run_id", runID,
		"doc_id", docID,
		"filename", header.Filename,
		"content_type", contentType,
		"size_bytes", written,
		"content_hash", contentHash,
		"source_uri", sourceURI,
		"upload_uri", uploadURI,
		"dedup_result", func() string {
			if isNew {
				return "new"
			}
			return "new_version"
		}(),
		"delegation_mode", "http",
		"delegation_success", true,
	)

	writeJSON(w, http.StatusAccepted, ingestResponse{
		RunID:  runID,
		DocID:  docID,
		Status: "queued",
	})
}

// getOrCreateDocument performs atomic INSERT ON CONFLICT for document dedup.
// Returns (doc_id, is_new, error).
func (h *IngestHandler) getOrCreateDocument(ctx context.Context, tenantID, sourceType, sourceURI, title string) (string, bool, error) {
	docID := uuid.New().String()

	var returnedDocID string
	var isNew bool

	err := h.pool.QueryRow(ctx,
		`INSERT INTO documents (doc_id, tenant_id, source_type, source_uri, title)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, source_uri)
		 DO UPDATE SET title = EXCLUDED.title
		 RETURNING doc_id, (xmax = 0) AS is_new`,
		docID, tenantID, sourceType, sourceURI, title,
	).Scan(&returnedDocID, &isNew)

	if err != nil {
		return "", false, fmt.Errorf("insert document: %w", err)
	}

	return returnedDocID, isNew, nil
}

// getActiveVersionHash returns the content_hash of the active version for a document.
// Returns empty string if no active version exists.
func (h *IngestHandler) getActiveVersionHash(ctx context.Context, tenantID, docID string) (string, error) {
	var hash string
	err := h.pool.QueryRow(ctx,
		`SELECT dv.content_hash
		 FROM document_versions dv
		 WHERE dv.tenant_id = $1
		   AND dv.doc_id = $2
		   AND dv.is_active = true`,
		tenantID, docID,
	).Scan(&hash)

	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil // No active version
		}
		return "", fmt.Errorf("query active version hash: %w", err)
	}

	return hash, nil
}

// delegateToWorker sends the job payload to the ingest-worker via HTTP POST.
func (h *IngestHandler) delegateToWorker(ctx context.Context, payload jobPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal job payload: %w", err)
	}

	url := h.cfg.IngestWorkerURL + "/internal/process"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if h.cfg.InternalAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.cfg.InternalAuthToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("worker unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted {
		return nil // Success
	}

	if resp.StatusCode == http.StatusServiceUnavailable {
		return fmt.Errorf("worker busy, all slots occupied")
	}

	// Read error body for diagnostics
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return fmt.Errorf("worker returned %d: %s", resp.StatusCode, string(respBody))
}
