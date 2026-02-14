// Package model defines the domain types for the query API.
package model

import "time"

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
