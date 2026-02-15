package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// IngestionHandler handles ingestion run endpoints.
type IngestionHandler struct {
	pool *pgxpool.Pool
}

// NewIngestionHandler creates a new IngestionHandler.
func NewIngestionHandler(pool *pgxpool.Pool) *IngestionHandler {
	return &IngestionHandler{pool: pool}
}

// List handles GET /v1/ingestion-runs?tenant_id=...&page=1&limit=20
func (h *IngestionHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	pg := model.DefaultPagination(page, limit)

	// Count total runs
	var total int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ingestion_runs WHERE tenant_id = $1`,
		tenantID,
	).Scan(&total); err != nil {
		slog.Error("failed to count ingestion runs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to count ingestion runs")
		return
	}

	// Fetch runs
	rows, err := h.pool.Query(ctx,
		`SELECT run_id, status, started_at, finished_at, config, stats, error
		 FROM ingestion_runs
		 WHERE tenant_id = $1
		 ORDER BY started_at DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, pg.Limit, pg.Offset(),
	)
	if err != nil {
		slog.Error("failed to list ingestion runs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to list ingestion runs")
		return
	}
	defer rows.Close()

	runs := make([]model.IngestionRunItem, 0)
	for rows.Next() {
		var run model.IngestionRunItem
		if err := rows.Scan(
			&run.RunID, &run.Status, &run.StartedAt, &run.FinishedAt,
			&run.Config, &run.Stats, &run.Error,
		); err != nil {
			slog.Error("failed to scan ingestion run row", "error", err)
			writeError(w, http.StatusInternalServerError, "internal", "failed to read ingestion runs")
			return
		}

		// Compute duration if finished
		if run.FinishedAt != nil {
			durationMS := run.FinishedAt.Sub(run.StartedAt).Milliseconds()
			run.DurationMS = &durationMS
		}

		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		slog.Error("row iteration error", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to read ingestion runs")
		return
	}

	writeJSON(w, http.StatusOK, model.IngestionRunListResponse{
		Runs:  runs,
		Total: total,
		Page:  pg.Page,
		Limit: pg.Limit,
	})
}

// Get handles GET /v1/ingestion-runs/:id?tenant_id=...
func (h *IngestionHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	runID := chi.URLParam(r, "id")

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id query parameter is required")
		return
	}

	var run model.IngestionRunItem
	err := h.pool.QueryRow(ctx,
		`SELECT run_id, status, started_at, finished_at, config, stats, error
		 FROM ingestion_runs
		 WHERE run_id = $1 AND tenant_id = $2`,
		runID, tenantID,
	).Scan(
		&run.RunID, &run.Status, &run.StartedAt, &run.FinishedAt,
		&run.Config, &run.Stats, &run.Error,
	)
	if err != nil {
		if err.Error() == "no rows in result set" {
			writeError(w, http.StatusNotFound, "not_found", "ingestion run not found")
			return
		}
		slog.Error("failed to get ingestion run", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to get ingestion run")
		return
	}

	// Compute duration if finished
	if run.FinishedAt != nil {
		durationMS := run.FinishedAt.Sub(run.StartedAt).Milliseconds()
		run.DurationMS = &durationMS
	}

	writeJSON(w, http.StatusOK, run)
}
