package service

import (
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

const abstainMessage = "I don't have enough information in the current documents to answer that."

// AbstainResult holds the outcome of the abstain check.
type AbstainResult struct {
	ShouldAbstain bool
	Reason        string
}

// CheckAbstainZeroCandidates checks if both vector and FTS returned zero results.
// This is the first abstain check, before RRF/rerank.
func CheckAbstainZeroCandidates(vecCount, ftsCount int) *AbstainResult {
	if vecCount == 0 && ftsCount == 0 {
		return &AbstainResult{
			ShouldAbstain: true,
			Reason:        "zero candidates from both vector and FTS search",
		}
	}
	return &AbstainResult{ShouldAbstain: false}
}

// CheckAbstainPostRerank checks if the top reranked chunk score is below the threshold.
// Only called when the reranker was actually used (not skipped).
func CheckAbstainPostRerank(chunks []model.ChunkResult, threshold float64) *AbstainResult {
	if len(chunks) == 0 {
		return &AbstainResult{
			ShouldAbstain: true,
			Reason:        "no chunks after reranking",
		}
	}

	topScore := chunks[0].RerankScore
	if topScore < threshold {
		return &AbstainResult{
			ShouldAbstain: true,
			Reason:        "top rerank score below threshold",
		}
	}

	return &AbstainResult{ShouldAbstain: false}
}

// CheckAbstainPostRRF checks if the top RRF score is below the threshold.
// Called when the reranker was skipped/failed (fail-open path).
func CheckAbstainPostRRF(chunks []model.ChunkResult, threshold float64) *AbstainResult {
	if len(chunks) == 0 {
		return &AbstainResult{
			ShouldAbstain: true,
			Reason:        "no chunks after RRF merge",
		}
	}

	topScore := chunks[0].RRFScore
	if topScore < threshold {
		return &AbstainResult{
			ShouldAbstain: true,
			Reason:        "top RRF score below threshold",
		}
	}

	return &AbstainResult{ShouldAbstain: false}
}

// AbstainResponse builds the standard abstain QueryResponse.
func AbstainResponse() *model.QueryResponse {
	return &model.QueryResponse{
		Answer:    abstainMessage,
		Citations: []model.Citation{},
		Abstained: true,
	}
}
