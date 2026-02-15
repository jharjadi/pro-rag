// Package handler implements HTTP handlers for the management APIs.
package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// DocumentHandler handles document management endpoints.
type DocumentHandler struct {
	pool *pgxpool.Pool
}

// NewDocumentHandler creates a new DocumentHandler.
func NewDocumentHandler(pool *pgxpool.Pool) *DocumentHandler {
	return &DocumentHandler{pool: pool}
}

// List handles GET /v1/documents?tenant_id=...&page=1&limit=20&search=...
func (h *DocumentHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	pg := model.DefaultPagination(page, limit)
	search := r.URL.Query().Get("search")

	// Count total documents
	var total int
	countQuery := `SELECT COUNT(*) FROM documents WHERE tenant_id = $1`
	countArgs := []interface{}{tenantID}
	if search != "" {
		countQuery += ` AND title ILIKE $2`
		countArgs = append(countArgs, "%"+search+"%")
	}
	if err := h.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		slog.Error("failed to count documents", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to count documents")
		return
	}

	// Fetch documents with active version info
	query := `
		SELECT
			d.doc_id, d.title, d.source_type, d.source_uri, d.content_hash, d.created_at,
			dv.doc_version_id, dv.version_label, dv.effective_at,
			COALESCE(cs.chunk_count, 0), COALESCE(cs.total_tokens, 0)
		FROM documents d
		LEFT JOIN document_versions dv
			ON dv.doc_id = d.doc_id AND dv.tenant_id = d.tenant_id AND dv.is_active = true
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS chunk_count, COALESCE(SUM(token_count), 0) AS total_tokens
			FROM chunks c
			WHERE c.doc_version_id = dv.doc_version_id AND c.tenant_id = d.tenant_id
		) cs ON true
		WHERE d.tenant_id = $1`
	args := []interface{}{tenantID}
	argIdx := 2

	if search != "" {
		query += ` AND d.title ILIKE $` + strconv.Itoa(argIdx)
		args = append(args, "%"+search+"%")
		argIdx++
	}

	query += ` ORDER BY d.created_at DESC LIMIT $` + strconv.Itoa(argIdx) + ` OFFSET $` + strconv.Itoa(argIdx+1)
	args = append(args, pg.Limit, pg.Offset())

	rows, err := h.pool.Query(ctx, query, args...)
	if err != nil {
		slog.Error("failed to list documents", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list documents")
		return
	}
	defer rows.Close()

	docs := make([]model.DocumentListItem, 0)
	for rows.Next() {
		var doc model.DocumentListItem
		var dvID, dvLabel sql.NullString
		var dvEffective sql.NullTime
		var chunkCount, totalTokens int

		if err := rows.Scan(
			&doc.DocID, &doc.Title, &doc.SourceType, &doc.SourceURI, &doc.ContentHash, &doc.CreatedAt,
			&dvID, &dvLabel, &dvEffective,
			&chunkCount, &totalTokens,
		); err != nil {
			slog.Error("failed to scan document row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to read documents")
			return
		}

		if dvID.Valid {
			doc.ActiveVersion = &model.ActiveVersionSummary{
				DocVersionID: dvID.String,
				VersionLabel: dvLabel.String,
				EffectiveAt:  dvEffective.Time,
				ChunkCount:   chunkCount,
				TotalTokens:  totalTokens,
			}
		}

		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to read documents")
		return
	}

	writeJSON(w, http.StatusOK, model.DocumentListResponse{
		Documents: docs,
		Total:     total,
		Page:      pg.Page,
		Limit:     pg.Limit,
	})
}

// Get handles GET /v1/documents/:id?tenant_id=...
func (h *DocumentHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	docID := chi.URLParam(r, "id")

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	// Fetch document
	var doc model.DocumentDetailResponse
	err := h.pool.QueryRow(ctx,
		`SELECT doc_id, title, source_type, source_uri, content_hash, created_at
		 FROM documents WHERE doc_id = $1 AND tenant_id = $2`,
		docID, tenantID,
	).Scan(&doc.DocID, &doc.Title, &doc.SourceType, &doc.SourceURI, &doc.ContentHash, &doc.CreatedAt)

	if err != nil {
		if err.Error() == "no rows in result set" {
			writeError(w, http.StatusNotFound, "not_found", "document not found")
			return
		}
		slog.Error("failed to get document", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get document")
		return
	}

	// Fetch versions with chunk stats
	rows, err := h.pool.Query(ctx,
		`SELECT
			dv.doc_version_id, dv.version_label, dv.is_active, dv.effective_at, dv.created_at,
			COALESCE(cs.chunk_count, 0), COALESCE(cs.total_tokens, 0)
		FROM document_versions dv
		LEFT JOIN LATERAL (
			SELECT COUNT(*) AS chunk_count, COALESCE(SUM(token_count), 0) AS total_tokens
			FROM chunks c
			WHERE c.doc_version_id = dv.doc_version_id AND c.tenant_id = dv.tenant_id
		) cs ON true
		WHERE dv.doc_id = $1 AND dv.tenant_id = $2
		ORDER BY dv.created_at DESC`,
		docID, tenantID,
	)
	if err != nil {
		slog.Error("failed to get document versions", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get document versions")
		return
	}
	defer rows.Close()

	doc.Versions = make([]model.VersionDetail, 0)
	for rows.Next() {
		var v model.VersionDetail
		if err := rows.Scan(
			&v.DocVersionID, &v.VersionLabel, &v.IsActive, &v.EffectiveAt, &v.CreatedAt,
			&v.ChunkCount, &v.TotalTokens,
		); err != nil {
			slog.Error("failed to scan version row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to read versions")
			return
		}
		doc.Versions = append(doc.Versions, v)
	}

	writeJSON(w, http.StatusOK, doc)
}

// ListChunks handles GET /v1/documents/:id/chunks?tenant_id=...&page=1&limit=50&version_id=...
func (h *DocumentHandler) ListChunks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	docID := chi.URLParam(r, "id")

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	pg := model.DefaultPagination(page, limit)
	versionID := r.URL.Query().Get("version_id")

	// Determine which version to use
	var targetVersionID string
	if versionID != "" {
		// Verify the version belongs to this document and tenant
		err := h.pool.QueryRow(ctx,
			`SELECT doc_version_id FROM document_versions
			 WHERE doc_version_id = $1 AND doc_id = $2 AND tenant_id = $3`,
			versionID, docID, tenantID,
		).Scan(&targetVersionID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "version not found for this document")
			return
		}
	} else {
		// Use active version
		err := h.pool.QueryRow(ctx,
			`SELECT doc_version_id FROM document_versions
			 WHERE doc_id = $1 AND tenant_id = $2 AND is_active = true`,
			docID, tenantID,
		).Scan(&targetVersionID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "no active version found for this document")
			return
		}
	}

	// Count total chunks
	var total int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM chunks WHERE doc_version_id = $1 AND tenant_id = $2`,
		targetVersionID, tenantID,
	).Scan(&total); err != nil {
		slog.Error("failed to count chunks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to count chunks")
		return
	}

	// Fetch chunks
	rows, err := h.pool.Query(ctx,
		`SELECT chunk_id, ordinal, heading_path, chunk_type, text, token_count, metadata
		 FROM chunks
		 WHERE doc_version_id = $1 AND tenant_id = $2
		 ORDER BY ordinal ASC
		 LIMIT $3 OFFSET $4`,
		targetVersionID, tenantID, pg.Limit, pg.Offset(),
	)
	if err != nil {
		slog.Error("failed to list chunks", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list chunks")
		return
	}
	defer rows.Close()

	chunks := make([]model.ChunkListItem, 0)
	for rows.Next() {
		var c model.ChunkListItem
		if err := rows.Scan(
			&c.ChunkID, &c.Ordinal, &c.HeadingPath, &c.ChunkType, &c.Text, &c.TokenCount, &c.Metadata,
		); err != nil {
			slog.Error("failed to scan chunk row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to read chunks")
			return
		}
		chunks = append(chunks, c)
	}

	writeJSON(w, http.StatusOK, model.ChunkListResponse{
		Chunks: chunks,
		Total:  total,
		Page:   pg.Page,
		Limit:  pg.Limit,
	})
}

// Deactivate handles POST /v1/documents/:id/deactivate?tenant_id=...
func (h *DocumentHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	docID := chi.URLParam(r, "id")

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	// Find the active version
	var docVersionID string
	err := h.pool.QueryRow(ctx,
		`SELECT doc_version_id FROM document_versions
		 WHERE doc_id = $1 AND tenant_id = $2 AND is_active = true`,
		docID, tenantID,
	).Scan(&docVersionID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			writeError(w, http.StatusNotFound, "not_found", "no active version found for this document")
			return
		}
		slog.Error("failed to find active version", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to find active version")
		return
	}

	// Deactivate
	_, err = h.pool.Exec(ctx,
		`UPDATE document_versions SET is_active = false
		 WHERE doc_version_id = $1 AND tenant_id = $2`,
		docVersionID, tenantID,
	)
	if err != nil {
		slog.Error("failed to deactivate version", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to deactivate document")
		return
	}

	slog.Info("document deactivated", "doc_id", docID, "doc_version_id", docVersionID, "tenant_id", tenantID)

	writeJSON(w, http.StatusOK, model.DeactivateResponse{
		Status:       "deactivated",
		DocID:        docID,
		DocVersionID: docVersionID,
	})
}
