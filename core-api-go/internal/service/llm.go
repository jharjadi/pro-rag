package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const anthropicMessagesURL = "https://api.anthropic.com/v1/messages"
const anthropicAPIVersion = "2023-06-01"

// LLMResponse holds the LLM's response text and token usage.
type LLMResponse struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
}

// LLMService handles communication with the LLM provider.
type LLMService struct {
	provider  string
	model     string
	apiKey    string
	maxTokens int
	client    *http.Client
}

// NewLLMService creates a new LLMService.
func NewLLMService(provider, model, apiKey string, maxTokens int) *LLMService {
	return &LLMService{
		provider:  provider,
		model:     model,
		apiKey:    apiKey,
		maxTokens: maxTokens,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Generate sends the system prompt and user message to the LLM and returns the response.
func (s *LLMService) Generate(ctx context.Context, systemPrompt, userMessage string) (*LLMResponse, error) {
	start := time.Now()

	switch s.provider {
	case "anthropic":
		return s.generateAnthropic(ctx, systemPrompt, userMessage, start)
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", s.provider)
	}
}

// generateAnthropic calls the Anthropic Messages API.
func (s *LLMService) generateAnthropic(ctx context.Context, systemPrompt, userMessage string, start time.Time) (*LLMResponse, error) {
	reqBody := anthropicRequest{
		Model:     s.model,
		MaxTokens: s.maxTokens,
		System:    systemPrompt,
		Messages: []anthropicMessage{
			{Role: "user", Content: userMessage},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicMessagesURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Anthropic API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Extract text from content blocks
	var text string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return &LLMResponse{
		Text:             text,
		PromptTokens:     anthropicResp.Usage.InputTokens,
		CompletionTokens: anthropicResp.Usage.OutputTokens,
		Latency:          time.Since(start),
	}, nil
}

// Provider returns the configured LLM provider name.
func (s *LLMService) Provider() string {
	return s.provider
}

// Model returns the configured LLM model name.
func (s *LLMService) Model() string {
	return s.model
}

// Anthropic API types

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string                  `json:"id"`
	Type    string                  `json:"type"`
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
	Usage   anthropicUsage          `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
