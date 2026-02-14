package service

import (
	"strings"
	"testing"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

func TestFormatContext_SingleChunk(t *testing.T) {
	chunks := []model.ChunkResult{
		{
			ChunkID:      "abc-123",
			Title:        "Test Doc",
			VersionLabel: "v1",
			HeadingPath:  []string{"Section 1", "Subsection A"},
			Text:         "This is the chunk text.",
		},
	}

	result := FormatContext(chunks)

	if !strings.Contains(result, "Title: Test Doc") {
		t.Error("expected Title in context")
	}
	if !strings.Contains(result, "Version: v1") {
		t.Error("expected Version in context")
	}
	if !strings.Contains(result, "Heading: Section 1 > Subsection A") {
		t.Error("expected Heading path in context")
	}
	if !strings.Contains(result, "ChunkID: abc-123") {
		t.Error("expected ChunkID in context")
	}
	if !strings.Contains(result, "Text: This is the chunk text.") {
		t.Error("expected Text in context")
	}
}

func TestFormatContext_MultipleChunks(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", Title: "Doc A", VersionLabel: "v1", Text: "Text A"},
		{ChunkID: "b", Title: "Doc B", VersionLabel: "v2", Text: "Text B"},
	}

	result := FormatContext(chunks)

	// Should have separator between chunks
	if !strings.Contains(result, "---") {
		t.Error("expected separator between chunks")
	}
	if !strings.Contains(result, "Text A") {
		t.Error("expected Text A")
	}
	if !strings.Contains(result, "Text B") {
		t.Error("expected Text B")
	}
}

func TestFormatContext_EmptyHeadingPath(t *testing.T) {
	chunks := []model.ChunkResult{
		{ChunkID: "a", Title: "Doc", VersionLabel: "v1", HeadingPath: nil, Text: "text"},
	}

	result := FormatContext(chunks)
	if !strings.Contains(result, "Heading: (none)") {
		t.Error("expected '(none)' for empty heading path")
	}
}

func TestFormatContext_Empty(t *testing.T) {
	result := FormatContext(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildUserMessage(t *testing.T) {
	msg := BuildUserMessage("some context", "What is the policy?")
	if !strings.Contains(msg, "Context:") {
		t.Error("expected 'Context:' in message")
	}
	if !strings.Contains(msg, "some context") {
		t.Error("expected context block in message")
	}
	if !strings.Contains(msg, "Question: What is the policy?") {
		t.Error("expected question in message")
	}
}

func TestSystemPrompt_ContainsRequiredElements(t *testing.T) {
	if !strings.Contains(SystemPrompt, "ONLY the provided context") {
		t.Error("system prompt should mention using only provided context")
	}
	if !strings.Contains(SystemPrompt, "[chunk:<CHUNK_ID>]") {
		t.Error("system prompt should mention citation format")
	}
	if !strings.Contains(SystemPrompt, "parental leave") {
		t.Error("system prompt should contain abstain example")
	}
}
