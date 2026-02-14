// Package config loads all environment variables for the core-api-go service.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the query API service.
type Config struct {
	// Server
	APIHost string
	APIPort string

	// Database
	DatabaseURL string

	// Retrieval
	KVec int
	KFTS int
	RRFK int

	// Reranker (Cohere)
	CohereAPIKey        string
	CohereRerankerModel string
	RerankTimeoutMS     int
	RerankMaxDocs       int
	RerankFailOpen      bool

	// Abstain thresholds
	AbstainRerankThreshold float64
	AbstainRRFThreshold    float64

	// Context budgeting
	MaxContextTokens      int
	ContextOverheadTokens int
	MaxContextChunks      int

	// LLM
	LLMProvider     string
	LLMModel        string
	AnthropicAPIKey string
	LLMMaxTokens    int

	// Embedding sidecar
	EmbedEndpoint string

	// Timeouts
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// Load reads configuration from environment variables with sensible defaults.
func Load() (*Config, error) {
	cfg := &Config{
		APIHost: envOr("API_HOST", "0.0.0.0"),
		APIPort: envOr("API_PORT", "8000"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		KVec: envInt("K_VEC", 50),
		KFTS: envInt("K_FTS", 50),
		RRFK: envInt("RRF_K", 60),

		CohereAPIKey:        os.Getenv("COHERE_API_KEY"),
		CohereRerankerModel: envOr("COHERE_RERANK_MODEL", "rerank-v3.5"),
		RerankTimeoutMS:     envInt("RERANK_TIMEOUT_MS", 3000),
		RerankMaxDocs:       envInt("RERANK_MAX_DOCS", 50),
		RerankFailOpen:      envBool("RERANK_FAIL_OPEN", true),

		AbstainRerankThreshold: envFloat("ABSTAIN_RERANK_THRESHOLD", 0.15),
		AbstainRRFThreshold:    envFloat("ABSTAIN_RRF_THRESHOLD", 0.030),

		MaxContextTokens:      envInt("MAX_CONTEXT_TOKENS", 6000),
		ContextOverheadTokens: envInt("CONTEXT_OVERHEAD_TOKENS", 1000),
		MaxContextChunks:      envInt("MAX_CONTEXT_CHUNKS", 12),

		LLMProvider:     envOr("LLM_PROVIDER", "anthropic"),
		LLMModel:        envOr("LLM_MODEL", "claude-sonnet-4-20250514"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		LLMMaxTokens:    envInt("LLM_MAX_TOKENS", 1024),

		EmbedEndpoint: envOr("EMBED_ENDPOINT", "http://embed:8001/embed"),

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 60 * time.Second, // LLM calls can be slow
		IdleTimeout:  60 * time.Second,
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	return cfg, nil
}

// Addr returns the listen address as "host:port".
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%s", c.APIHost, c.APIPort)
}

// RerankTimeout returns the reranker timeout as a time.Duration.
func (c *Config) RerankTimeout() time.Duration {
	return time.Duration(c.RerankTimeoutMS) * time.Millisecond
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
