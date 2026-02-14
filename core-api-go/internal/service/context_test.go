package service

import (
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

func TestSelectContextChunks_FitsAll(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", TokenCount: 100},
		{ChunkID: "b", TokenCount: 200},
		{ChunkID: "c", TokenCount: 300},
	}

	selected, tokens := SelectContextChunks(chunks, 6000, 1000, 12)
	if len(selected) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(selected))
	}
	if tokens != 600 {
		t.Errorf("expected 600 tokens, got %d", tokens)
	}
}

func TestSelectContextChunks_TokenBudgetLimit(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", TokenCount: 400},
		{ChunkID: "b", TokenCount: 400},
		{ChunkID: "c", TokenCount: 400},
	}

	// Budget = 1000 - 200 = 800. Only 2 chunks fit (400+400=800)
	selected, tokens := SelectContextChunks(chunks, 1000, 200, 12)
	if len(selected) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(selected))
	}
	if tokens != 800 {
		t.Errorf("expected 800 tokens, got %d", tokens)
	}
}

func TestSelectContextChunks_MaxChunksLimit(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", TokenCount: 10},
		{ChunkID: "b", TokenCount: 10},
		{ChunkID: "c", TokenCount: 10},
		{ChunkID: "d", TokenCount: 10},
	}

	// Max 2 chunks even though budget allows all
	selected, tokens := SelectContextChunks(chunks, 6000, 1000, 2)
	if len(selected) != 2 {
		t.Errorf("expected 2 chunks (max), got %d", len(selected))
	}
	if tokens != 20 {
		t.Errorf("expected 20 tokens, got %d", tokens)
	}
}

func TestSelectContextChunks_ZeroBudget(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", TokenCount: 100},
	}

	// Overhead >= maxTokens → zero budget
	selected, tokens := SelectContextChunks(chunks, 1000, 1000, 12)
	if len(selected) != 0 {
		t.Errorf("expected 0 chunks with zero budget, got %d", len(selected))
	}
	if tokens != 0 {
		t.Errorf("expected 0 tokens, got %d", tokens)
	}
}

func TestSelectContextChunks_EmptyInput(t *testing.T) {
	selected, tokens := SelectContextChunks(nil, 6000, 1000, 12)
	if len(selected) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(selected))
	}
	if tokens != 0 {
		t.Errorf("expected 0 tokens, got %d", tokens)
	}
}

func TestSelectContextChunks_SingleLargeChunk(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", TokenCount: 5000},
		{ChunkID: "b", TokenCount: 100},
	}

	// Budget = 6000 - 1000 = 5000. First chunk exactly fits.
	selected, tokens := SelectContextChunks(chunks, 6000, 1000, 12)
	if len(selected) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(selected))
	}
	if tokens != 5000 {
		t.Errorf("expected 5000 tokens, got %d", tokens)
	}
}
