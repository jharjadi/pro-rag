package service

import (
	"math"
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

func TestMergeRRF_BothEmpty(t *testing.T) {
	result := MergeRRF(nil, nil, 60)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestMergeRRF_VecOnly(t *testing.T) {
	vec := []model.ChunkResult{
		{ChunkID: "a", Text: "chunk a"},
		{ChunkID: "b", Text: "chunk b"},
	}
	result := MergeRRF(vec, nil, 60)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// First result should be "a" with RRF score 1/(60+1)
	if result[0].ChunkID != "a" {
		t.Errorf("expected first result to be 'a', got %q", result[0].ChunkID)
	}
	expectedScore := 1.0 / 61.0
	if math.Abs(result[0].RRFScore-expectedScore) > 1e-9 {
		t.Errorf("expected RRF score %f, got %f", expectedScore, result[0].RRFScore)
	}
}

func TestMergeRRF_FTSOnly(t *testing.T) {
	fts := []model.ChunkResult{
		{ChunkID: "x", Text: "chunk x"},
	}
	result := MergeRRF(nil, fts, 60)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ChunkID != "x" {
		t.Errorf("expected 'x', got %q", result[0].ChunkID)
	}
}

func TestMergeRRF_OverlappingChunks(t *testing.T) {
	// Chunk "a" appears in both vec (rank 1) and FTS (rank 1)
	vec := []model.ChunkResult{
		{ChunkID: "a", Text: "chunk a", VecScore: 0.9},
		{ChunkID: "b", Text: "chunk b", VecScore: 0.8},
	}
	fts := []model.ChunkResult{
		{ChunkID: "a", Text: "chunk a", FTSScore: 0.5},
		{ChunkID: "c", Text: "chunk c", FTSScore: 0.3},
	}

	result := MergeRRF(vec, fts, 60)

	if len(result) != 3 {
		t.Fatalf("expected 3 results (a, b, c), got %d", len(result))
	}

	// "a" should be first (appears in both lists)
	if result[0].ChunkID != "a" {
		t.Errorf("expected first result to be 'a', got %q", result[0].ChunkID)
	}

	// "a" RRF score = 1/(60+1) + 1/(60+1) = 2/61
	expectedA := 2.0 / 61.0
	if math.Abs(result[0].RRFScore-expectedA) > 1e-9 {
		t.Errorf("expected RRF score %f for 'a', got %f", expectedA, result[0].RRFScore)
	}

	// "a" should have both vec and FTS scores
	if result[0].VecScore != 0.9 {
		t.Errorf("expected VecScore 0.9, got %f", result[0].VecScore)
	}
	if result[0].FTSScore != 0.5 {
		t.Errorf("expected FTSScore 0.5, got %f", result[0].FTSScore)
	}
}

func TestMergeRRF_SortedByScore(t *testing.T) {
	// "a" in both lists, "b" only in vec, "c" only in FTS
	vec := []model.ChunkResult{
		{ChunkID: "b", Text: "b"},
		{ChunkID: "a", Text: "a"},
	}
	fts := []model.ChunkResult{
		{ChunkID: "c", Text: "c"},
		{ChunkID: "a", Text: "a"},
	}

	result := MergeRRF(vec, fts, 60)

	// "a" appears in both (rank 2 in each), so RRF = 1/62 + 1/62 = 2/62
	// "b" appears in vec rank 1: RRF = 1/61
	// "c" appears in FTS rank 1: RRF = 1/61
	// "b" and "c" tie at 1/61, "a" has 2/62 ≈ 0.0323 vs 1/61 ≈ 0.0164
	// Wait: 2/62 = 1/31 ≈ 0.0323, 1/61 ≈ 0.0164
	// So "a" > "b" = "c"

	if result[0].ChunkID != "a" {
		t.Errorf("expected 'a' first, got %q", result[0].ChunkID)
	}
}

func TestMergeRRF_DifferentK(t *testing.T) {
	vec := []model.ChunkResult{
		{ChunkID: "a", Text: "a"},
	}

	// With k=0, RRF score = 1/(0+1) = 1.0
	result := MergeRRF(vec, nil, 0)
	if math.Abs(result[0].RRFScore-1.0) > 1e-9 {
		t.Errorf("expected RRF score 1.0 with k=0, got %f", result[0].RRFScore)
	}
}
