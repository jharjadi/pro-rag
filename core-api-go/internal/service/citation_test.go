package service

import (
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

func makeContextChunks() []model.ChunkResult {
	return []model.ChunkResult{
		{
			ChunkID:      "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			DocID:        "doc-1",
			DocVersionID: "ver-1",
			Title:        "Test Doc",
			VersionLabel: "v1",
		},
		{
			ChunkID:      "11111111-2222-3333-4444-555555555555",
			DocID:        "doc-2",
			DocVersionID: "ver-2",
			Title:        "Other Doc",
			VersionLabel: "v1",
		},
	}
}

func TestParseCitations_ValidCitation(t *testing.T) {
	ctx := makeContextChunks()
	text := "The policy states X [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee]."

	citations := ParseCitations(text, ctx)
	if len(citations) != 1 {
		t.Fatalf("expected 1 citation, got %d", len(citations))
	}
	if citations[0].ChunkID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("wrong chunk_id: %s", citations[0].ChunkID)
	}
	if citations[0].Title != "Test Doc" {
		t.Errorf("wrong title: %s", citations[0].Title)
	}
}

func TestParseCitations_MultipleCitations(t *testing.T) {
	ctx := makeContextChunks()
	text := "X [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee] and Y [chunk:11111111-2222-3333-4444-555555555555]."

	citations := ParseCitations(text, ctx)
	if len(citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(citations))
	}
}

func TestParseCitations_DuplicateCitation(t *testing.T) {
	ctx := makeContextChunks()
	text := "X [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee] and also [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee]."

	citations := ParseCitations(text, ctx)
	if len(citations) != 1 {
		t.Fatalf("expected 1 citation (deduped), got %d", len(citations))
	}
}

func TestParseCitations_HallucinatedCitation(t *testing.T) {
	ctx := makeContextChunks()
	// This chunk_id is not in the context
	text := "X [chunk:99999999-9999-9999-9999-999999999999]."

	citations := ParseCitations(text, ctx)
	if len(citations) != 0 {
		t.Fatalf("expected 0 citations (hallucinated dropped), got %d", len(citations))
	}
}

func TestParseCitations_NoCitations(t *testing.T) {
	ctx := makeContextChunks()
	text := "I don't have enough information to answer that."

	citations := ParseCitations(text, ctx)
	if len(citations) != 0 {
		t.Fatalf("expected 0 citations, got %d", len(citations))
	}
}

func TestParseCitations_MixedValidAndHallucinated(t *testing.T) {
	ctx := makeContextChunks()
	text := "X [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee] and Y [chunk:99999999-9999-9999-9999-999999999999]."

	citations := ParseCitations(text, ctx)
	if len(citations) != 1 {
		t.Fatalf("expected 1 citation (valid only), got %d", len(citations))
	}
	if citations[0].ChunkID != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("wrong chunk_id: %s", citations[0].ChunkID)
	}
}

func TestParseCitations_EmptyContext(t *testing.T) {
	text := "X [chunk:aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee]."
	citations := ParseCitations(text, nil)
	if len(citations) != 0 {
		t.Fatalf("expected 0 citations with empty context, got %d", len(citations))
	}
}
