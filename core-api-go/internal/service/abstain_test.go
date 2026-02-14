package service

import (
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

func TestCheckAbstainZeroCandidates_BothZero(t *testing.T) {
	result := CheckAbstainZeroCandidates(0, 0)
	if !result.ShouldAbstain {
		t.Error("expected abstain when both vec and FTS are zero")
	}
}

func TestCheckAbstainZeroCandidates_VecOnly(t *testing.T) {
	result := CheckAbstainZeroCandidates(5, 0)
	if result.ShouldAbstain {
		t.Error("should not abstain when vec has results")
	}
}

func TestCheckAbstainZeroCandidates_FTSOnly(t *testing.T) {
	result := CheckAbstainZeroCandidates(0, 3)
	if result.ShouldAbstain {
		t.Error("should not abstain when FTS has results")
	}
}

func TestCheckAbstainZeroCandidates_BothNonZero(t *testing.T) {
	result := CheckAbstainZeroCandidates(10, 10)
	if result.ShouldAbstain {
		t.Error("should not abstain when both have results")
	}
}

func TestCheckAbstainPostRerank_EmptyChunks(t *testing.T) {
	result := CheckAbstainPostRerank(nil, 0.15)
	if !result.ShouldAbstain {
		t.Error("expected abstain with empty chunks")
	}
}

func TestCheckAbstainPostRerank_BelowThreshold(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", RerankScore: 0.10},
		{ChunkID: "b", RerankScore: 0.05},
	}
	result := CheckAbstainPostRerank(chunks, 0.15)
	if !result.ShouldAbstain {
		t.Error("expected abstain when top score (0.10) < threshold (0.15)")
	}
}

func TestCheckAbstainPostRerank_AboveThreshold(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", RerankScore: 0.20},
		{ChunkID: "b", RerankScore: 0.10},
	}
	result := CheckAbstainPostRerank(chunks, 0.15)
	if result.ShouldAbstain {
		t.Error("should not abstain when top score (0.20) >= threshold (0.15)")
	}
}

func TestCheckAbstainPostRerank_ExactThreshold(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", RerankScore: 0.15},
	}
	result := CheckAbstainPostRerank(chunks, 0.15)
	if result.ShouldAbstain {
		t.Error("should not abstain when top score equals threshold")
	}
}

func TestCheckAbstainPostRRF_EmptyChunks(t *testing.T) {
	result := CheckAbstainPostRRF(nil, 0.030)
	if !result.ShouldAbstain {
		t.Error("expected abstain with empty chunks")
	}
}

func TestCheckAbstainPostRRF_BelowThreshold(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", RRFScore: 0.020},
	}
	result := CheckAbstainPostRRF(chunks, 0.030)
	if !result.ShouldAbstain {
		t.Error("expected abstain when top RRF score (0.020) < threshold (0.030)")
	}
}

func TestCheckAbstainPostRRF_AboveThreshold(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", RRFScore: 0.035},
	}
	result := CheckAbstainPostRRF(chunks, 0.030)
	if result.ShouldAbstain {
		t.Error("should not abstain when top RRF score (0.035) >= threshold (0.030)")
	}
}

func TestAbstainResponse(t *testing.T) {
	resp := AbstainResponse()
	if !resp.Abstained {
		t.Error("expected Abstained=true")
	}
	if resp.Answer == "" {
		t.Error("expected non-empty answer")
	}
	if len(resp.Citations) != 0 {
		t.Error("expected empty citations")
	}
}
