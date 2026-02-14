package service

import (
	"fmt"
	"strings"

	"github.com/jharjadi/pro-rag/core-api-go/internal/model"
)

// SystemPrompt is the hardcoded V1 system prompt from spec §8.
// Hardcoded as a named constant — prompt iteration is high-frequency;
// a constant makes it easy to find and change. Template files add
// indirection without benefit at this scale.
const SystemPrompt = `You are a careful assistant answering questions using ONLY the provided context.
Rules:
1) If the answer is not clearly supported by the context, say you don't know and ask a clarifying question.
2) Do NOT use outside knowledge. Do NOT guess.
3) Every factual claim must include citations like [chunk:<CHUNK_ID>].
4) If the user asks for something outside scope, explain what's missing.

Example of abstaining:
User: What is our parental leave policy in Germany?
Assistant: I don't have enough information in the current documents to answer this specifically for Germany. The available documents cover US policy only. Could you clarify which document should contain this, or check whether the Germany-specific policy has been uploaded?`

// FormatContext formats chunks into the context block for the LLM prompt.
// Each chunk follows the spec §8 format:
//
//	Title: <title>
//	Version: <version_label>
//	Heading: <heading_path>
//	ChunkID: <chunk_id>
//	Text: <chunk text>
func FormatContext(chunks []model.ChunkResult) string {
	var sb strings.Builder
	for i, c := range chunks {
		if i > 0 {
			sb.WriteString("\n---\n")
		}
		sb.WriteString(fmt.Sprintf("Title: %s\n", c.Title))
		sb.WriteString(fmt.Sprintf("Version: %s\n", c.VersionLabel))
		heading := strings.Join(c.HeadingPath, " > ")
		if heading == "" {
			heading = "(none)"
		}
		sb.WriteString(fmt.Sprintf("Heading: %s\n", heading))
		sb.WriteString(fmt.Sprintf("ChunkID: %s\n", c.ChunkID))
		sb.WriteString(fmt.Sprintf("Text: %s\n", c.Text))
	}
	return sb.String()
}

// BuildUserMessage builds the user message with context + question.
func BuildUserMessage(contextBlock, question string) string {
	return fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextBlock, question)
}
