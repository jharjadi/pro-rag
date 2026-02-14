package service

import (
	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// SelectContextChunks selects the top chunks that fit within the token budget.
// maxTokens is the total context budget (e.g., 6000).
// overhead is reserved for system prompt + question (e.g., 1000).
// maxChunks is the maximum number of chunks to include (e.g., 12).
func SelectContextChunks(chunks []model.ChunkResult, maxTokens, overhead, maxChunks int) ([]model.ChunkResult, int) {
	budget := maxTokens - overhead
	if budget <= 0 {
		return nil, 0
	}

	var selected []model.ChunkResult
	totalTokens := 0

	for _, c := range chunks {
		if len(selected) >= maxChunks {
			break
		}
		if totalTokens+c.TokenCount > budget {
			break
		}
		selected = append(selected, c)
		totalTokens += c.TokenCount
	}

	return selected, totalTokens
}
