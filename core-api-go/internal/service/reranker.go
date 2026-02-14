package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

const cohereRerankURL = "https://api.cohere.com/v2/rerank"

// RerankResult holds the outcome of a rerank attempt.
type RerankResult struct {
	Chunks  []model.ChunkResult
	Used    bool   // true if reranker was actually used
	Skipped bool   // true if reranker was skipped (fail-open)
	Error   string // error message if reranker failed
	Latency time.Duration
}

// RerankerService handles Cohere reranking with fail-open behavior.
type RerankerService struct {
	apiKey   string
	model    string
	timeout  time.Duration
	maxDocs  int
	failOpen bool
	client   *http.Client
}

// NewRerankerService creates a new RerankerService.
func NewRerankerService(apiKey, model string, timeout time.Duration, maxDocs int, failOpen bool) *RerankerService {
	return &RerankerService{
		apiKey:   apiKey,
		model:    model,
		timeout:  timeout,
		maxDocs:  maxDocs,
		failOpen: failOpen,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Enabled returns true if the reranker has an API key configured.
func (s *RerankerService) Enabled() bool {
	return s.apiKey != ""
}

// Rerank sends chunks to Cohere for reranking. If the reranker fails and
// failOpen is true, returns the original chunks with RRF scores unchanged.
func (s *RerankerService) Rerank(ctx context.Context, question string, chunks []model.ChunkResult) *RerankResult {
	start := time.Now()

	// If no API key, skip reranking
	if !s.Enabled() {
		return &RerankResult{
			Chunks:  chunks,
			Used:    false,
			Skipped: true,
			Error:   "no API key configured",
			Latency: time.Since(start),
		}
	}

	// Limit documents sent to reranker
	toRerank := chunks
	if len(toRerank) > s.maxDocs {
		toRerank = toRerank[:s.maxDocs]
	}

	// Build request
	docs := make([]string, len(toRerank))
	for i, c := range toRerank {
		docs[i] = c.Text
	}

	reqBody := cohereRerankRequest{
		Model:           s.model,
		Query:           question,
		Documents:       docs,
		TopN:            len(docs),
		ReturnDocuments: false,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return s.handleError(chunks, start, fmt.Sprintf("marshal request: %v", err))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cohereRerankURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return s.handleError(chunks, start, fmt.Sprintf("create request: %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return s.handleError(chunks, start, fmt.Sprintf("HTTP request: %v", err))
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.handleError(chunks, start, fmt.Sprintf("read response: %v", err))
	}

	if resp.StatusCode != http.StatusOK {
		return s.handleError(chunks, start, fmt.Sprintf("Cohere API returned %d: %s", resp.StatusCode, string(respBody)))
	}

	var rerankResp cohereRerankResponse
	if err := json.Unmarshal(respBody, &rerankResp); err != nil {
		return s.handleError(chunks, start, fmt.Sprintf("unmarshal response: %v", err))
	}

	// Apply rerank scores to chunks
	reranked := make([]model.ChunkResult, 0, len(rerankResp.Results))
	for _, r := range rerankResp.Results {
		if r.Index < 0 || r.Index >= len(toRerank) {
			slog.Warn("reranker returned invalid index", "index", r.Index, "total", len(toRerank))
			continue
		}
		chunk := toRerank[r.Index]
		chunk.RerankScore = r.RelevanceScore
		reranked = append(reranked, chunk)
	}

	return &RerankResult{
		Chunks:  reranked,
		Used:    true,
		Skipped: false,
		Latency: time.Since(start),
	}
}

// handleError implements fail-open: if failOpen is true, return original chunks.
func (s *RerankerService) handleError(originalChunks []model.ChunkResult, start time.Time, errMsg string) *RerankResult {
	slog.Warn("reranker failed", "error", errMsg, "fail_open", s.failOpen)

	if s.failOpen {
		return &RerankResult{
			Chunks:  originalChunks,
			Used:    false,
			Skipped: true,
			Error:   errMsg,
			Latency: time.Since(start),
		}
	}

	// If not fail-open, return empty results (caller should handle as error)
	return &RerankResult{
		Chunks:  nil,
		Used:    false,
		Skipped: false,
		Error:   errMsg,
		Latency: time.Since(start),
	}
}

// Cohere API types

type cohereRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n"`
	ReturnDocuments bool     `json:"return_documents"`
}

type cohereRerankResponse struct {
	ID      string               `json:"id"`
	Results []cohereRerankResult `json:"results"`
}

type cohereRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}
