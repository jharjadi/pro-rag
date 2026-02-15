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

	// ── Ingestion orchestration (spec v2.3 §17) ──────────

	// IngestMode: "http" (V1) or "queue" (V2)
	IngestMode string

	// IngestWorkerURL is the internal HTTP URL for the ingest-worker service
	IngestWorkerURL string

	// IngestWorkerTimeoutMS is the HTTP timeout for delegation calls to the worker
	IngestWorkerTimeoutMS int

	// InternalAuthToken is the shared HMAC token for internal service auth
	InternalAuthToken string

	// UploadStorePath is the base path for raw uploaded files
	UploadStorePath string

	// UploadMaxSizeMB is the maximum upload file size in MB
	UploadMaxSizeMB int

	// AuthEnabled controls whether JWT auth is enforced
	AuthEnabled bool

	// JWTSecret is the HMAC-SHA256 signing key for JWT tokens
	JWTSecret string

	// JWTExpiryHours is the JWT token lifetime in hours (default 24)
	JWTExpiryHours int

	// CrashGuardQueuedTTLHours marks queued runs as failed after this many hours
	CrashGuardQueuedTTLHours int

	// CrashGuardRunningStaleMin marks running runs as failed if no heartbeat for this many minutes
	CrashGuardRunningStaleMin int

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

		// Ingestion orchestration
		IngestMode:            envOr("INGEST_MODE", "http"),
		IngestWorkerURL:       envOr("INGEST_WORKER_URL", "http://ingest-worker:8002"),
		IngestWorkerTimeoutMS: envInt("INGEST_WORKER_TIMEOUT_MS", 300000),
		InternalAuthToken:     os.Getenv("INTERNAL_AUTH_TOKEN"),
		UploadStorePath:       envOr("UPLOAD_STORE_PATH", "/data/uploads"),
		UploadMaxSizeMB:       envInt("UPLOAD_MAX_SIZE_MB", 50),
		AuthEnabled:           envBool("AUTH_ENABLED", false),
		JWTSecret:             os.Getenv("JWT_SECRET"),
		JWTExpiryHours:        envInt("JWT_EXPIRY_HOURS", 24),

		CrashGuardQueuedTTLHours:  envInt("CRASH_GUARD_QUEUED_TTL_HOURS", 1),
		CrashGuardRunningStaleMin: envInt("CRASH_GUARD_RUNNING_STALE_MIN", 15),

		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second, // Increased for file uploads
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

// IngestWorkerTimeout returns the ingest worker HTTP timeout as a time.Duration.
func (c *Config) IngestWorkerTimeout() time.Duration {
	return time.Duration(c.IngestWorkerTimeoutMS) * time.Millisecond
}

// UploadMaxSizeBytes returns the max upload size in bytes.
func (c *Config) UploadMaxSizeBytes() int64 {
	return int64(c.UploadMaxSizeMB) * 1024 * 1024
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
