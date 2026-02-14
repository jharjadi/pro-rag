package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbedService handles question embedding for vector search.
// In V1, this calls a local embedding sidecar HTTP service
// that wraps sentence-transformers (BAAI/bge-base-en-v1.5).
type EmbedService struct {
	endpoint string // e.g., "http://embed:8001/embed"
	client   *http.Client
}

// NewEmbedService creates a new EmbedService.
// endpoint is the URL of the embedding HTTP service.
func NewEmbedService(endpoint string) *EmbedService {
	return &EmbedService{
		endpoint: endpoint,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// embedRequest is the request body for the embedding service.
type embedRequest struct {
	Texts []string `json:"texts"`
}

// embedResponse is the response body from the embedding service.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed generates an embedding vector for the given text.
func (s *EmbedService) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := embedRequest{
		Texts: []string{text},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read embed response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embed service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var embedResp embedResponse
	if err := json.Unmarshal(respBody, &embedResp); err != nil {
		return nil, fmt.Errorf("unmarshal embed response: %w", err)
	}

	if len(embedResp.Embeddings) == 0 {
		return nil, fmt.Errorf("embed service returned no embeddings")
	}

	return embedResp.Embeddings[0], nil
}
