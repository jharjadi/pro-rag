package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// IngestHandler proxies ingestion requests to the internal ingest-api service.
// Go acts as the single API gateway — all external traffic routes through here.
type IngestHandler struct {
	ingestAPIURL string
	client       *http.Client
}

// NewIngestHandler creates a new IngestHandler.
func NewIngestHandler(ingestAPIURL string) *IngestHandler {
	return &IngestHandler{
		ingestAPIURL: ingestAPIURL,
		client: &http.Client{
			Timeout: 120 * time.Second, // generous timeout for file upload + initial processing
		},
	}
}

// Ingest handles POST /v1/ingest — proxies multipart form to internal ingest-api.
// The request is forwarded as-is (multipart/form-data) to ingest-api:8002/ingest.
func (h *IngestHandler) Ingest(w http.ResponseWriter, r *http.Request) {
	// Build proxy request to internal ingest-api
	targetURL := fmt.Sprintf("%s/ingest", h.ingestAPIURL)

	proxyReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, r.Body)
	if err != nil {
		slog.Error("failed to create proxy request", "error", err)
		writeError(w, http.StatusInternalServerError, "internal", "failed to create ingest request")
		return
	}

	// Forward content-type (multipart boundary is in there)
	proxyReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	if cl := r.Header.Get("Content-Length"); cl != "" {
		proxyReq.Header.Set("Content-Length", cl)
	}

	resp, err := h.client.Do(proxyReq)
	if err != nil {
		slog.Error("ingest-api request failed", "error", err, "url", targetURL)
		writeError(w, http.StatusBadGateway, "ingest_unavailable", "ingestion service is unavailable")
		return
	}
	defer resp.Body.Close()

	// Forward response headers and status
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Error("failed to copy ingest response", "error", err)
	}
}
