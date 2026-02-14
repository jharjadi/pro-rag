package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_MissingDatabaseURL(t *testing.T) {
	// Ensure DATABASE_URL is not set
	os.Unsetenv("DATABASE_URL")

	_, err := Load()
	if err == nil {
		t.Error("expected error when DATABASE_URL is missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	defer os.Unsetenv("DATABASE_URL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.APIHost != "0.0.0.0" {
		t.Errorf("expected APIHost '0.0.0.0', got %q", cfg.APIHost)
	}
	if cfg.APIPort != "8000" {
		t.Errorf("expected APIPort '8000', got %q", cfg.APIPort)
	}
	if cfg.KVec != 50 {
		t.Errorf("expected KVec 50, got %d", cfg.KVec)
	}
	if cfg.KFTS != 50 {
		t.Errorf("expected KFTS 50, got %d", cfg.KFTS)
	}
	if cfg.RRFK != 60 {
		t.Errorf("expected RRFK 60, got %d", cfg.RRFK)
	}
	if cfg.AbstainRerankThreshold != 0.15 {
		t.Errorf("expected AbstainRerankThreshold 0.15, got %f", cfg.AbstainRerankThreshold)
	}
	if cfg.AbstainRRFThreshold != 0.030 {
		t.Errorf("expected AbstainRRFThreshold 0.030, got %f", cfg.AbstainRRFThreshold)
	}
	if cfg.MaxContextTokens != 6000 {
		t.Errorf("expected MaxContextTokens 6000, got %d", cfg.MaxContextTokens)
	}
	if cfg.MaxContextChunks != 12 {
		t.Errorf("expected MaxContextChunks 12, got %d", cfg.MaxContextChunks)
	}
	if cfg.RerankFailOpen != true {
		t.Error("expected RerankFailOpen true")
	}
	if cfg.LLMProvider != "anthropic" {
		t.Errorf("expected LLMProvider 'anthropic', got %q", cfg.LLMProvider)
	}
}

func TestLoad_CustomValues(t *testing.T) {
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("K_VEC", "100")
	os.Setenv("ABSTAIN_RERANK_THRESHOLD", "0.25")
	os.Setenv("RERANK_FAIL_OPEN", "false")
	defer func() {
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("K_VEC")
		os.Unsetenv("ABSTAIN_RERANK_THRESHOLD")
		os.Unsetenv("RERANK_FAIL_OPEN")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.KVec != 100 {
		t.Errorf("expected KVec 100, got %d", cfg.KVec)
	}
	if cfg.AbstainRerankThreshold != 0.25 {
		t.Errorf("expected AbstainRerankThreshold 0.25, got %f", cfg.AbstainRerankThreshold)
	}
	if cfg.RerankFailOpen != false {
		t.Error("expected RerankFailOpen false")
	}
}

func TestAddr(t *testing.T) {
	cfg := &Config{APIHost: "0.0.0.0", APIPort: "8000"}
	if cfg.Addr() != "0.0.0.0:8000" {
		t.Errorf("expected '0.0.0.0:8000', got %q", cfg.Addr())
	}
}

func TestRerankTimeout(t *testing.T) {
	cfg := &Config{RerankTimeoutMS: 3000}
	if cfg.RerankTimeout() != 3*time.Second {
		t.Errorf("expected 3s, got %v", cfg.RerankTimeout())
	}
}
