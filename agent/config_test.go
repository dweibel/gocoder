package agent

import (
	"fmt"
	"os"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ardp-coding-agent, Property 13: Config env var defaults
// Validates: Requirements 9.1, 9.2, 9.3
func TestConfigEnvVarDefaults(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate non-empty alphanumeric values for each env var
		// LoadConfig only overrides defaults when env var is non-empty
		baseURL := rapid.StringMatching(`[a-zA-Z0-9:/._-]{1,50}`).Draw(t, "baseURL")
		model := rapid.StringMatching(`[a-zA-Z0-9/_.-]{1,50}`).Draw(t, "model")
		maxTokensVal := rapid.Int64Range(1, 100000).Draw(t, "maxTokens")

		// Randomly decide whether to set each env var
		setBaseURL := rapid.Bool().Draw(t, "setBaseURL")
		setModel := rapid.Bool().Draw(t, "setModel")
		setMaxTokens := rapid.Bool().Draw(t, "setMaxTokens")

		// Clear all env vars first
		os.Unsetenv("OPENROUTER_BASE_URL")
		os.Unsetenv("OPENROUTER_MODEL")
		os.Unsetenv("OPENROUTER_MAX_TOKENS")
		os.Unsetenv("OPENROUTER_API_KEY")

		if setBaseURL {
			os.Setenv("OPENROUTER_BASE_URL", baseURL)
		}
		if setModel {
			os.Setenv("OPENROUTER_MODEL", model)
		}
		if setMaxTokens {
			os.Setenv("OPENROUTER_MAX_TOKENS", fmt.Sprintf("%d", maxTokensVal))
		}

		cfg := LoadConfig()

		// Verify: env var when set, default when not
		if setBaseURL {
			if cfg.BaseURL != baseURL {
				t.Fatalf("expected BaseURL %q, got %q", baseURL, cfg.BaseURL)
			}
		} else {
			if cfg.BaseURL != "https://openrouter.ai/api" {
				t.Fatalf("expected default BaseURL, got %q", cfg.BaseURL)
			}
		}

		if setModel {
			if cfg.Model != model {
				t.Fatalf("expected Model %q, got %q", model, cfg.Model)
			}
		} else {
			if cfg.Model != "anthropic/claude-sonnet-4" {
				t.Fatalf("expected default Model, got %q", cfg.Model)
			}
		}

		if setMaxTokens {
			if cfg.MaxTokens != maxTokensVal {
				t.Fatalf("expected MaxTokens %d, got %d", maxTokensVal, cfg.MaxTokens)
			}
		} else {
			if cfg.MaxTokens != 4096 {
				t.Fatalf("expected default MaxTokens 4096, got %d", cfg.MaxTokens)
			}
		}

		// Cleanup
		os.Unsetenv("OPENROUTER_BASE_URL")
		os.Unsetenv("OPENROUTER_MODEL")
		os.Unsetenv("OPENROUTER_MAX_TOKENS")
	})
}

// Feature: ardp-coding-agent, Property 15: Invalid max tokens rejected
// Validates: Requirements 9.5
func TestInvalidMaxTokens(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate non-positive int64 values (0 or negative)
		maxTokens := rapid.Int64Range(-1000000, 0).Draw(t, "maxTokens")

		cfg := AgentConfig{
			APIKey:    "test-api-key",
			BaseURL:   "https://openrouter.ai/api",
			Model:     "anthropic/claude-sonnet-4",
			MaxTokens: maxTokens,
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected validation error for MaxTokens=%d, got nil", maxTokens)
		}
	})
}

// Unit tests for config validation
func TestValidateReturnsErrorForMissingAPIKey(t *testing.T) {
	cfg := AgentConfig{
		APIKey:    "",
		BaseURL:   "https://openrouter.ai/api",
		Model:     "anthropic/claude-sonnet-4",
		MaxTokens: 4096,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
}

func TestValidatePassesForValidConfig(t *testing.T) {
	cfg := AgentConfig{
		APIKey:    "test-key",
		BaseURL:   "https://openrouter.ai/api",
		Model:     "anthropic/claude-sonnet-4",
		MaxTokens: 4096,
	}

	err := cfg.Validate()
	if err != nil {
		t.Fatalf("expected no error for valid config, got %v", err)
	}
}

func TestLoadConfigAppliesDefaults(t *testing.T) {
	// Ensure env vars are unset so defaults apply
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("OPENROUTER_BASE_URL", "")
	t.Setenv("OPENROUTER_MODEL", "")
	t.Setenv("OPENROUTER_MAX_TOKENS", "")

	cfg := LoadConfig()

	if cfg.BaseURL != "https://openrouter.ai/api" {
		t.Fatalf("expected default BaseURL, got %q", cfg.BaseURL)
	}
	if cfg.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("expected default Model, got %q", cfg.Model)
	}
	if cfg.MaxTokens != 4096 {
		t.Fatalf("expected default MaxTokens 4096, got %d", cfg.MaxTokens)
	}
	if cfg.MaxIterations != 10 {
		t.Fatalf("expected default MaxIterations 10, got %d", cfg.MaxIterations)
	}
}
