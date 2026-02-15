// Package handler implements HTTP handlers for the query API.
package handler

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jharjadi/pro-rag/core-api-go/internal/config"
	authmw "github.com/jharjadi/pro-rag/core-api-go/internal/middleware"
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
	"github.com/jharjadi/pro-rag/core-api-go/internal/service"
)

// QueryHandler handles POST /v1/query requests.
type QueryHandler struct {
	cfg       *config.Config
	retrieval *service.RetrievalService
	reranker  *service.RerankerService
	llm       *service.LLMService
	embed     *service.EmbedService
}

// NewQueryHandler creates a new QueryHandler with all required services.
func NewQueryHandler(
	cfg *config.Config,
	retrieval *service.RetrievalService,
	reranker *service.RerankerService,
	llm *service.LLMService,
	embed *service.EmbedService,
) *QueryHandler {
	return &QueryHandler{
		cfg:       cfg,
		retrieval: retrieval,
		reranker:  reranker,
		llm:       llm,
		embed:     embed,
	}
}

// Handle processes a POST /v1/query request through the full pipeline:
// embed question → retrieve (vec+FTS parallel) → RRF merge → rerank → abstain check → context budget → LLM → citations → response
func (h *QueryHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	totalStart := time.Now()

	// Get request ID from chi middleware
	requestID := chimw.GetReqID(ctx)

	// tenant_id from auth middleware context (spec v2.3 §2.1)
	tenantID := authmw.TenantIDFromContext(ctx)
	if tenantID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "tenant_id is required")
		return
	}

	// Parse request
	var req model.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON: "+err.Error())
		return
	}

	// Override tenant_id from context (never trust client payload)
	req.TenantID = tenantID

	if req.Question == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "question is required")
		return
	}

	// Default top_k
	if req.TopK <= 0 {
		req.TopK = 10
	}

	// Initialize query log
	qlog := &model.QueryLog{
		Timestamp:    time.Now().UTC(),
		TenantID:     req.TenantID,
		RequestID:    requestID,
		QuestionHash: hashQuestion(req.Question),
		KVec:         h.cfg.KVec,
		KFTS:         h.cfg.KFTS,
		LLMProvider:  h.llm.Provider(),
		LLMModel:     h.llm.Model(),
	}

	// ── Stage 1: Embed question ──────────────────────────
	embedStart := time.Now()
	questionEmbedding, err := h.embed.Embed(ctx, req.Question)
	if err != nil {
		slog.Error("failed to embed question", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "internal", "failed to embed question")
		h.emitQueryLog(qlog, http.StatusInternalServerError, totalStart)
		return
	}
	_ = time.Since(embedStart) // embed latency (not in spec log, but tracked)

	// ── Stage 2: Retrieve (vector + FTS parallel) ────────
	vecStart := time.Now()
	result, err := h.retrieval.Retrieve(ctx, req.TenantID, req.Question, questionEmbedding, h.cfg.KVec, h.cfg.KFTS)
	if err != nil {
		slog.Error("retrieval failed", "error", err, "request_id", requestID)
		writeError(w, http.StatusInternalServerError, "internal", "retrieval failed")
		h.emitQueryLog(qlog, http.StatusInternalServerError, totalStart)
		return
	}
	qlog.LatencyMSVec = time.Since(vecStart).Milliseconds() // approximate; both run in parallel
	qlog.LatencyMSFTS = qlog.LatencyMSVec                   // same wall-clock since parallel
	qlog.NumVecCandidates = len(result.VecResults)
	qlog.NumFTSCandidates = len(result.FTSResults)

	// ── Stage 3: Zero candidates check ───────────────────
	abstainCheck := service.CheckAbstainZeroCandidates(len(result.VecResults), len(result.FTSResults))
	if abstainCheck.ShouldAbstain {
		slog.Info("abstaining: zero candidates", "request_id", requestID)
		resp := service.AbstainResponse()
		if req.Debug {
			resp.Debug = &model.DebugInfo{
				VecCandidates:    0,
				FTSCandidates:    0,
				MergedCandidates: 0,
			}
		}
		qlog.Abstained = true
		writeJSON(w, http.StatusOK, resp)
		h.emitQueryLog(qlog, http.StatusOK, totalStart)
		return
	}

	// ── Stage 4: RRF merge ───────────────────────────────
	mergeStart := time.Now()
	merged := service.MergeRRF(result.VecResults, result.FTSResults, h.cfg.RRFK)
	qlog.LatencyMSMerge = time.Since(mergeStart).Milliseconds()
	qlog.NumMergedCandidates = len(merged)

	// ── Stage 5: Rerank ──────────────────────────────────
	rerankStart := time.Now()
	rerankResult := h.reranker.Rerank(ctx, req.Question, merged)
	qlog.LatencyMSRerank = time.Since(rerankStart).Milliseconds()
	qlog.RerankerUsed = rerankResult.Used
	qlog.RerankerSkipped = rerankResult.Skipped
	qlog.RerankerLatencyMS = rerankResult.Latency.Milliseconds()
	qlog.KRerank = len(rerankResult.Chunks)

	// ── Stage 6: Abstain check (post-rerank or post-RRF) ─
	var postAbstain *service.AbstainResult
	if rerankResult.Used {
		postAbstain = service.CheckAbstainPostRerank(rerankResult.Chunks, h.cfg.AbstainRerankThreshold)
	} else {
		postAbstain = service.CheckAbstainPostRRF(rerankResult.Chunks, h.cfg.AbstainRRFThreshold)
	}

	if postAbstain.ShouldAbstain {
		slog.Info("abstaining", "reason", postAbstain.Reason, "request_id", requestID)
		resp := service.AbstainResponse()
		if req.Debug {
			topScores := extractTopScores(rerankResult.Chunks, rerankResult.Used, 5)
			resp.Debug = &model.DebugInfo{
				VecCandidates:    qlog.NumVecCandidates,
				FTSCandidates:    qlog.NumFTSCandidates,
				MergedCandidates: qlog.NumMergedCandidates,
				RerankerUsed:     rerankResult.Used,
				RerankerSkipped:  rerankResult.Skipped,
				RerankerError:    rerankResult.Error,
				TopScores:        topScores,
			}
		}
		qlog.Abstained = true
		writeJSON(w, http.StatusOK, resp)
		h.emitQueryLog(qlog, http.StatusOK, totalStart)
		return
	}

	// ── Stage 7: Context budgeting ───────────────────────
	contextChunks, contextTokens := service.SelectContextChunks(
		rerankResult.Chunks,
		h.cfg.MaxContextTokens,
		h.cfg.ContextOverheadTokens,
		h.cfg.MaxContextChunks,
	)
	qlog.NumContextChunks = len(contextChunks)
	qlog.ContextTokensEst = contextTokens

	// ── Stage 8: Build prompt + call LLM ─────────────────
	contextBlock := service.FormatContext(contextChunks)
	userMessage := service.BuildUserMessage(contextBlock, req.Question)

	llmStart := time.Now()
	llmResp, err := h.llm.Generate(ctx, service.SystemPrompt, userMessage)
	qlog.LatencyMSLLM = time.Since(llmStart).Milliseconds()

	if err != nil {
		slog.Error("LLM call failed", "error", err, "request_id", requestID)
		writeError(w, http.StatusBadGateway, "llm_unavailable", "LLM service unavailable")
		h.emitQueryLog(qlog, http.StatusBadGateway, totalStart)
		return
	}

	qlog.LLMPromptTokens = llmResp.PromptTokens
	qlog.LLMCompletionTokens = llmResp.CompletionTokens

	// ── Stage 9: Parse citations ─────────────────────────
	citations := service.ParseCitations(llmResp.Text, contextChunks)

	// ── Stage 10: Build response ─────────────────────────
	resp := &model.QueryResponse{
		Answer:    llmResp.Text,
		Citations: citations,
		Abstained: false,
	}

	if req.Debug {
		topScores := extractTopScores(rerankResult.Chunks, rerankResult.Used, 5)
		resp.Debug = &model.DebugInfo{
			VecCandidates:    qlog.NumVecCandidates,
			FTSCandidates:    qlog.NumFTSCandidates,
			MergedCandidates: qlog.NumMergedCandidates,
			RerankerUsed:     rerankResult.Used,
			RerankerSkipped:  rerankResult.Skipped,
			RerankerError:    rerankResult.Error,
			TopScores:        topScores,
			ContextChunks:    qlog.NumContextChunks,
			ContextTokensEst: qlog.ContextTokensEst,
		}
	}

	writeJSON(w, http.StatusOK, resp)
	h.emitQueryLog(qlog, http.StatusOK, totalStart)
}

// emitQueryLog writes the structured per-query log line (spec §Logging).
func (h *QueryHandler) emitQueryLog(qlog *model.QueryLog, httpStatus int, totalStart time.Time) {
	qlog.HTTPStatus = httpStatus
	qlog.LatencyMSTotal = time.Since(totalStart).Milliseconds()

	slog.Info("query",
		"ts", qlog.Timestamp.Format(time.RFC3339),
		"tenant_id", qlog.TenantID,
		"request_id", qlog.RequestID,
		"question_hash", qlog.QuestionHash,
		"k_vec", qlog.KVec,
		"k_fts", qlog.KFTS,
		"k_rerank", qlog.KRerank,
		"num_vec_candidates", qlog.NumVecCandidates,
		"num_fts_candidates", qlog.NumFTSCandidates,
		"num_merged_candidates", qlog.NumMergedCandidates,
		"reranker_used", qlog.RerankerUsed,
		"reranker_skipped", qlog.RerankerSkipped,
		"reranker_latency_ms", qlog.RerankerLatencyMS,
		"num_context_chunks", qlog.NumContextChunks,
		"context_tokens_est", qlog.ContextTokensEst,
		"abstained", qlog.Abstained,
		"latency_ms_total", qlog.LatencyMSTotal,
		"latency_ms_vec", qlog.LatencyMSVec,
		"latency_ms_fts", qlog.LatencyMSFTS,
		"latency_ms_merge", qlog.LatencyMSMerge,
		"latency_ms_rerank", qlog.LatencyMSRerank,
		"latency_ms_llm", qlog.LatencyMSLLM,
		"llm_provider", qlog.LLMProvider,
		"llm_model", qlog.LLMModel,
		"llm_prompt_tokens", qlog.LLMPromptTokens,
		"llm_completion_tokens", qlog.LLMCompletionTokens,
		"http_status", qlog.HTTPStatus,
	)
}

// hashQuestion returns SHA-256 hex of the normalized (lowercased, trimmed) question.
func hashQuestion(question string) string {
	h := sha256.Sum256([]byte(question))
	return fmt.Sprintf("%x", h)
}

// extractTopScores returns the top N scores from chunks (rerank or RRF).
func extractTopScores(chunks []model.ChunkResult, useRerank bool, n int) []float64 {
	if len(chunks) == 0 {
		return nil
	}
	if n > len(chunks) {
		n = len(chunks)
	}
	scores := make([]float64, n)
	for i := 0; i < n; i++ {
		if useRerank {
			scores[i] = chunks[i].RerankScore
		} else {
			scores[i] = chunks[i].RRFScore
		}
	}
	return scores
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a standard error response.
func writeError(w http.ResponseWriter, status int, errCode, message string) {
	writeJSON(w, status, model.ErrorResponse{
		Error:   errCode,
		Message: message,
	})
}
