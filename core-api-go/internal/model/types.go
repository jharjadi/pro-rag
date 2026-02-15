// Package model defines the domain types for the query API.
package model

import (
	"encoding/json"
	"time"
)

// QueryRequest is the POST /v1/query request body.
type QueryRequest struct {
	TenantID string `json:"tenant_id"`
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
	Debug    bool   `json:"debug"`
}

// QueryResponse is the POST /v1/query response body.
type QueryResponse struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Abstained bool       `json:"abstained"`
	Debug     *DebugInfo `json:"debug"`
}

// Citation represents a single citation in the response.
type Citation struct {
	DocID        string `json:"doc_id"`
	DocVersionID string `json:"doc_version_id"`
	ChunkID      string `json:"chunk_id"`
	Title        string `json:"title"`
	VersionLabel string `json:"version_label"`
}

// ErrorResponse is the standard error response body.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// DebugInfo contains debug information when debug=true.
type DebugInfo struct {
	VecCandidates    int       `json:"vec_candidates"`
	FTSCandidates    int       `json:"fts_candidates"`
	MergedCandidates int       `json:"merged_candidates"`
	RerankerUsed     bool      `json:"reranker_used"`
	RerankerSkipped  bool      `json:"reranker_skipped"`
	RerankerError    string    `json:"reranker_error,omitempty"`
	TopScores        []float64 `json:"top_scores,omitempty"`
	ContextChunks    int       `json:"context_chunks"`
	ContextTokensEst int       `json:"context_tokens_est"`
}

// ChunkResult represents a chunk retrieved from the database with its scores.
type ChunkResult struct {
	ChunkID      string
	TenantID     string
	DocVersionID string
	DocID        string
	Title        string
	VersionLabel string
	HeadingPath  []string
	ChunkType    string
	Text         string
	TokenCount   int
	Ordinal      int
	CreatedAt    time.Time

	// Scores from different retrieval stages
	VecScore    float64 // cosine similarity (1 - distance)
	FTSScore    float64 // ts_rank_cd score
	RRFScore    float64 // reciprocal rank fusion score
	RerankScore float64 // reranker score (0 if not reranked)

	// Rank positions (1-based)
	VecRank int
	FTSRank int
}

// QueryLog holds all fields for the structured per-query log line.
type QueryLog struct {
	Timestamp           time.Time `json:"ts"`
	TenantID            string    `json:"tenant_id"`
	RequestID           string    `json:"request_id"`
	QuestionHash        string    `json:"question_hash"`
	KVec                int       `json:"k_vec"`
	KFTS                int       `json:"k_fts"`
	KRerank             int       `json:"k_rerank"`
	NumVecCandidates    int       `json:"num_vec_candidates"`
	NumFTSCandidates    int       `json:"num_fts_candidates"`
	NumMergedCandidates int       `json:"num_merged_candidates"`
	RerankerUsed        bool      `json:"reranker_used"`
	RerankerSkipped     bool      `json:"reranker_skipped"`
	RerankerLatencyMS   int64     `json:"reranker_latency_ms"`
	NumContextChunks    int       `json:"num_context_chunks"`
	ContextTokensEst    int       `json:"context_tokens_est"`
	Abstained           bool      `json:"abstained"`
	LatencyMSTotal      int64     `json:"latency_ms_total"`
	LatencyMSVec        int64     `json:"latency_ms_vec"`
	LatencyMSFTS        int64     `json:"latency_ms_fts"`
	LatencyMSMerge      int64     `json:"latency_ms_merge"`
	LatencyMSRerank     int64     `json:"latency_ms_rerank"`
	LatencyMSLLM        int64     `json:"latency_ms_llm"`
	LLMProvider         string    `json:"llm_provider"`
	LLMModel            string    `json:"llm_model"`
	LLMPromptTokens     int       `json:"llm_prompt_tokens"`
	LLMCompletionTokens int       `json:"llm_completion_tokens"`
	HTTPStatus          int       `json:"http_status"`
}

// ── Management API types ─────────────────────────────────

// PaginationParams holds common pagination parameters.
type PaginationParams struct {
	Page  int
	Limit int
}

// DefaultPagination returns pagination with defaults applied.
func DefaultPagination(page, limit int) PaginationParams {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return PaginationParams{Page: page, Limit: limit}
}

// Offset returns the SQL offset for the current page.
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.Limit
}

// DocumentListItem represents a document in the list response.
type DocumentListItem struct {
	DocID         string                `json:"doc_id"`
	Title         string                `json:"title"`
	SourceType    string                `json:"source_type"`
	SourceURI     string                `json:"source_uri"`
	ContentHash   string                `json:"content_hash"`
	CreatedAt     time.Time             `json:"created_at"`
	ActiveVersion *ActiveVersionSummary `json:"active_version"`
}

// ActiveVersionSummary is the active version info embedded in document list items.
type ActiveVersionSummary struct {
	DocVersionID string    `json:"doc_version_id"`
	VersionLabel string    `json:"version_label"`
	EffectiveAt  time.Time `json:"effective_at"`
	ChunkCount   int       `json:"chunk_count"`
	TotalTokens  int       `json:"total_tokens"`
}

// DocumentListResponse is the response for GET /v1/documents.
type DocumentListResponse struct {
	Documents []DocumentListItem `json:"documents"`
	Total     int                `json:"total"`
	Page      int                `json:"page"`
	Limit     int                `json:"limit"`
}

// VersionDetail represents a document version in the detail response.
type VersionDetail struct {
	DocVersionID string    `json:"doc_version_id"`
	VersionLabel string    `json:"version_label"`
	IsActive     bool      `json:"is_active"`
	EffectiveAt  time.Time `json:"effective_at"`
	ContentHash  string    `json:"content_hash"`
	ChunkCount   int       `json:"chunk_count"`
	TotalTokens  int       `json:"total_tokens"`
	CreatedAt    time.Time `json:"created_at"`
}

// DocumentDetailResponse is the response for GET /v1/documents/:id.
type DocumentDetailResponse struct {
	DocID       string          `json:"doc_id"`
	Title       string          `json:"title"`
	SourceType  string          `json:"source_type"`
	SourceURI   string          `json:"source_uri"`
	ContentHash string          `json:"content_hash"`
	CreatedAt   time.Time       `json:"created_at"`
	Versions    []VersionDetail `json:"versions"`
}

// ChunkListItem represents a chunk in the list response.
type ChunkListItem struct {
	ChunkID     string          `json:"chunk_id"`
	Ordinal     int             `json:"ordinal"`
	HeadingPath json.RawMessage `json:"heading_path"`
	ChunkType   string          `json:"chunk_type"`
	Text        string          `json:"text"`
	TokenCount  int             `json:"token_count"`
	Metadata    json.RawMessage `json:"metadata"`
}

// ChunkListResponse is the response for GET /v1/documents/:id/chunks.
type ChunkListResponse struct {
	Chunks []ChunkListItem `json:"chunks"`
	Total  int             `json:"total"`
	Page   int             `json:"page"`
	Limit  int             `json:"limit"`
}

// DeactivateResponse is the response for POST /v1/documents/:id/deactivate.
type DeactivateResponse struct {
	Status       string `json:"status"`
	DocID        string `json:"doc_id"`
	DocVersionID string `json:"doc_version_id"`
}

// IngestionRunItem represents an ingestion run in list/detail responses.
type IngestionRunItem struct {
	RunID      string          `json:"run_id"`
	DocID      *string         `json:"doc_id"`
	Status     string          `json:"status"`
	RunType    string          `json:"run_type"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	DurationMS *int64          `json:"duration_ms"`
	Config     json.RawMessage `json:"config"`
	Stats      json.RawMessage `json:"stats"`
	Error      *string         `json:"error"`
}

// IngestionRunListResponse is the response for GET /v1/ingestion-runs.
type IngestionRunListResponse struct {
	Runs  []IngestionRunItem `json:"runs"`
	Total int                `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}
