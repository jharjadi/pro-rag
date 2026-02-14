package service

import (
	"sort"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// MergeRRF performs Reciprocal Rank Fusion on vector and FTS result sets.
// Formula: RRF(d) = sum over rank lists of 1/(k + rank(d))
// where k is the RRF constant (default 60) and rank is 1-based.
func MergeRRF(vecResults, ftsResults []model.ChunkResult, rrfK int) []model.ChunkResult {
	// Map chunk_id -> merged result
	merged := make(map[string]*model.ChunkResult)

	// Add vector results with RRF scores
	for i := range vecResults {
		cr := vecResults[i]
		rank := i + 1 // 1-based
		rrfScore := 1.0 / float64(rrfK+rank)

		if existing, ok := merged[cr.ChunkID]; ok {
			existing.RRFScore += rrfScore
			existing.VecScore = cr.VecScore
			existing.VecRank = rank
		} else {
			cr.RRFScore = rrfScore
			cr.VecRank = rank
			merged[cr.ChunkID] = &cr
		}
	}

	// Add FTS results with RRF scores
	for i := range ftsResults {
		cr := ftsResults[i]
		rank := i + 1 // 1-based
		rrfScore := 1.0 / float64(rrfK+rank)

		if existing, ok := merged[cr.ChunkID]; ok {
			existing.RRFScore += rrfScore
			existing.FTSScore = cr.FTSScore
			existing.FTSRank = rank
		} else {
			cr.RRFScore = rrfScore
			cr.FTSRank = rank
			merged[cr.ChunkID] = &cr
		}
	}

	// Convert map to sorted slice
	results := make([]model.ChunkResult, 0, len(merged))
	for _, cr := range merged {
		results = append(results, *cr)
	}

	// Sort by RRF score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].RRFScore > results[j].RRFScore
	})

	return results
}
