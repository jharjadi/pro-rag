package service

import (
	"log/slog"
	"regexp"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// citationRegex matches [chunk:<CHUNK_ID>] patterns in LLM response text.
// Chunk IDs are UUIDs: 8-4-4-4-12 hex characters.
var citationRegex = regexp.MustCompile(`\[chunk:([0-9a-fA-F-]{36})\]`)

// ParseCitations extracts and validates citations from the LLM response text.
// It checks each extracted chunk_id against the context chunks that were sent to the LLM.
// Hallucinated citations (chunk_id not in context) are dropped silently with a warning log.
func ParseCitations(responseText string, contextChunks []model.ChunkResult) []model.Citation {
	// Build lookup map: chunk_id -> ChunkResult
	chunkMap := make(map[string]model.ChunkResult, len(contextChunks))
	for _, c := range contextChunks {
		chunkMap[c.ChunkID] = c
	}

	// Extract all [chunk:<ID>] matches
	matches := citationRegex.FindAllStringSubmatch(responseText, -1)
	if len(matches) == 0 {
		return []model.Citation{}
	}

	// Deduplicate and validate
	seen := make(map[string]bool)
	var citations []model.Citation

	for _, match := range matches {
		chunkID := match[1]

		// Skip duplicates
		if seen[chunkID] {
			continue
		}
		seen[chunkID] = true

		// Validate against context chunks
		chunk, ok := chunkMap[chunkID]
		if !ok {
			// Hallucinated citation — drop silently, log warning
			slog.Warn("hallucinated citation dropped",
				"chunk_id", chunkID,
				"context_chunk_count", len(contextChunks),
			)
			continue
		}

		citations = append(citations, model.Citation{
			DocID:        chunk.DocID,
			DocVersionID: chunk.DocVersionID,
			ChunkID:      chunk.ChunkID,
			Title:        chunk.Title,
			VersionLabel: chunk.VersionLabel,
		})
	}

	if citations == nil {
		return []model.Citation{}
	}
	return citations
}
