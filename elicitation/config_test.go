package elicitation

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/ardp/coding-agent/agent"
	"pgregory.net/rapid"
)

// ============================================================================
// Task 2.1: Unit tests for LoadElicitationConfig
// ============================================================================

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ELICITATION_API_KEY", "ELICITATION_BASE_URL", "ELICITATION_MODEL",
		"ELICITATION_MAX_TOKENS", "ELICITATION_TIMEOUT",
		"OPENROUTER_API_KEY", "OPENROUTER_BASE_URL", "OPENROUTER_MODEL",
		"OPENROUTER_MAX_TOKENS", "OPENROUTER_TIMEOUT",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestLoadElicitationConfig_ElicitationPrefixTakesPriority(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("ELICITATION_API_KEY", "elicit-key")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")
	t.Setenv("ELICITATION_BASE_URL", "https://elicit.example.com")
	t.Setenv("OPENROUTER_BASE_URL", "https://openrouter.example.com")
	t.Setenv("ELICITATION_MODEL", "elicit-model")
	t.Setenv("OPENROUTER_MODEL", "openrouter-model")
	t.Setenv("ELICITATION_MAX_TOKENS", "8192")
	t.Setenv("OPENROUTER_MAX_TOKENS", "2048")
	t.Setenv("ELICITATION_TIMEOUT", "600")
	t.Setenv("OPENROUTER_TIMEOUT", "120")

	cfg := LoadElicitationConfig()

	if cfg.APIKey != "elicit-key" {
		t.Fatalf("expected APIKey %q, got %q", "elicit-key", cfg.APIKey)
	}
	if cfg.BaseURL != "https://elicit.example.com" {
		t.Fatalf("expected BaseURL %q, got %q", "https://elicit.example.com", cfg.BaseURL)
	}
	if cfg.Model != "elicit-model" {
		t.Fatalf("expected Model %q, got %q", "elicit-model", cfg.Model)
	}
	if cfg.MaxTokens != 8192 {
		t.Fatalf("expected MaxTokens %d, got %d", 8192, cfg.MaxTokens)
	}
	if cfg.Timeout.Seconds() != 600 {
		t.Fatalf("expected Timeout 600s, got %v", cfg.Timeout)
	}
}

func TestLoadElicitationConfig_FallbackToOpenRouter(t *testing.T) {
	clearConfigEnv(t)

	t.Setenv("OPENROUTER_API_KEY", "or-key")
	t.Setenv("OPENROUTER_BASE_URL", "https://or.example.com")
	t.Setenv("OPENROUTER_MODEL", "or-model")
	t.Setenv("OPENROUTER_MAX_TOKENS", "2048")
	t.Setenv("OPENROUTER_TIMEOUT", "120")

	cfg := LoadElicitationConfig()

	if cfg.APIKey != "or-key" {
		t.Fatalf("expected APIKey %q, got %q", "or-key", cfg.APIKey)
	}
	if cfg.BaseURL != "https://or.example.com" {
		t.Fatalf("expected BaseURL %q, got %q", "https://or.example.com", cfg.BaseURL)
	}
	if cfg.Model != "or-model" {
		t.Fatalf("expected Model %q, got %q", "or-model", cfg.Model)
	}
	if cfg.MaxTokens != 2048 {
		t.Fatalf("expected MaxTokens %d, got %d", 2048, cfg.MaxTokens)
	}
	if cfg.Timeout.Seconds() != 120 {
		t.Fatalf("expected Timeout 120s, got %v", cfg.Timeout)
	}
}

func TestLoadElicitationConfig_DefaultsWhenNeitherSet(t *testing.T) {
	clearConfigEnv(t)

	cfg := LoadElicitationConfig()

	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://openrouter.ai/api" {
		t.Fatalf("expected default BaseURL, got %q", cfg.BaseURL)
	}
	if cfg.Model != "anthropic/claude-sonnet-4" {
		t.Fatalf("expected default Model, got %q", cfg.Model)
	}
	if cfg.MaxTokens != 4096 {
		t.Fatalf("expected default MaxTokens 4096, got %d", cfg.MaxTokens)
	}
	if cfg.Timeout.Seconds() != 300 {
		t.Fatalf("expected default Timeout 300s, got %v", cfg.Timeout)
	}
}

func TestLoadElicitationConfig_PartialElicitationOverride(t *testing.T) {
	clearConfigEnv(t)

	// Only set ELICITATION_API_KEY and ELICITATION_MODEL; rest falls back to OPENROUTER_ or defaults
	t.Setenv("ELICITATION_API_KEY", "elicit-key")
	t.Setenv("ELICITATION_MODEL", "elicit-model")
	t.Setenv("OPENROUTER_BASE_URL", "https://or.example.com")
	t.Setenv("OPENROUTER_MAX_TOKENS", "2048")

	cfg := LoadElicitationConfig()

	if cfg.APIKey != "elicit-key" {
		t.Fatalf("expected APIKey %q, got %q", "elicit-key", cfg.APIKey)
	}
	if cfg.Model != "elicit-model" {
		t.Fatalf("expected Model %q, got %q", "elicit-model", cfg.Model)
	}
	if cfg.BaseURL != "https://or.example.com" {
		t.Fatalf("expected BaseURL %q (OPENROUTER fallback), got %q", "https://or.example.com", cfg.BaseURL)
	}
	if cfg.MaxTokens != 2048 {
		t.Fatalf("expected MaxTokens %d (OPENROUTER fallback), got %d", 2048, cfg.MaxTokens)
	}
	if cfg.Timeout.Seconds() != 300 {
		t.Fatalf("expected default Timeout 300s, got %v", cfg.Timeout)
	}
}

// ============================================================================
// Task 2.2: Property test — Config resolution with priority and fallback
// Feature: elicitation-engine, Property 1: Config resolution with priority and fallback
// Validates: Requirements 1.1, 1.2
// ============================================================================

func TestConfigResolutionPriority(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random non-empty values for both prefixes
		elicitKey := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "elicitKey")
		orKey := rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "orKey")
		elicitURL := rapid.StringMatching(`https://[a-z]{1,10}\.[a-z]{2,5}`).Draw(t, "elicitURL")
		orURL := rapid.StringMatching(`https://[a-z]{1,10}\.[a-z]{2,5}`).Draw(t, "orURL")
		elicitModel := rapid.StringMatching(`[a-z]{1,10}/[a-z]{1,10}`).Draw(t, "elicitModel")
		orModel := rapid.StringMatching(`[a-z]{1,10}/[a-z]{1,10}`).Draw(t, "orModel")
		elicitMaxTokens := rapid.Int64Range(1, 100000).Draw(t, "elicitMaxTokens")
		orMaxTokens := rapid.Int64Range(1, 100000).Draw(t, "orMaxTokens")
		elicitTimeout := rapid.IntRange(1, 3600).Draw(t, "elicitTimeout")
		orTimeout := rapid.IntRange(1, 3600).Draw(t, "orTimeout")

		// Randomly decide which ELICITATION_ vars are set
		setElicitKey := rapid.Bool().Draw(t, "setElicitKey")
		setElicitURL := rapid.Bool().Draw(t, "setElicitURL")
		setElicitModel := rapid.Bool().Draw(t, "setElicitModel")
		setElicitMaxTokens := rapid.Bool().Draw(t, "setElicitMaxTokens")
		setElicitTimeout := rapid.Bool().Draw(t, "setElicitTimeout")

		// Clear all env vars
		os.Unsetenv("ELICITATION_API_KEY")
		os.Unsetenv("ELICITATION_BASE_URL")
		os.Unsetenv("ELICITATION_MODEL")
		os.Unsetenv("ELICITATION_MAX_TOKENS")
		os.Unsetenv("ELICITATION_TIMEOUT")
		os.Unsetenv("OPENROUTER_API_KEY")
		os.Unsetenv("OPENROUTER_BASE_URL")
		os.Unsetenv("OPENROUTER_MODEL")
		os.Unsetenv("OPENROUTER_MAX_TOKENS")
		os.Unsetenv("OPENROUTER_TIMEOUT")

		// Always set OPENROUTER_ vars
		os.Setenv("OPENROUTER_API_KEY", orKey)
		os.Setenv("OPENROUTER_BASE_URL", orURL)
		os.Setenv("OPENROUTER_MODEL", orModel)
		os.Setenv("OPENROUTER_MAX_TOKENS", fmt.Sprintf("%d", orMaxTokens))
		os.Setenv("OPENROUTER_TIMEOUT", fmt.Sprintf("%d", orTimeout))

		// Conditionally set ELICITATION_ vars
		if setElicitKey {
			os.Setenv("ELICITATION_API_KEY", elicitKey)
		}
		if setElicitURL {
			os.Setenv("ELICITATION_BASE_URL", elicitURL)
		}
		if setElicitModel {
			os.Setenv("ELICITATION_MODEL", elicitModel)
		}
		if setElicitMaxTokens {
			os.Setenv("ELICITATION_MAX_TOKENS", fmt.Sprintf("%d", elicitMaxTokens))
		}
		if setElicitTimeout {
			os.Setenv("ELICITATION_TIMEOUT", fmt.Sprintf("%d", elicitTimeout))
		}

		cfg := LoadElicitationConfig()

		// Property: ELICITATION_ values always win when set; OPENROUTER_ used as fallback
		if setElicitKey {
			if cfg.APIKey != elicitKey {
				t.Fatalf("ELICITATION_API_KEY set: expected %q, got %q", elicitKey, cfg.APIKey)
			}
		} else {
			if cfg.APIKey != orKey {
				t.Fatalf("ELICITATION_API_KEY unset: expected OPENROUTER fallback %q, got %q", orKey, cfg.APIKey)
			}
		}

		if setElicitURL {
			if cfg.BaseURL != elicitURL {
				t.Fatalf("ELICITATION_BASE_URL set: expected %q, got %q", elicitURL, cfg.BaseURL)
			}
		} else {
			if cfg.BaseURL != orURL {
				t.Fatalf("ELICITATION_BASE_URL unset: expected OPENROUTER fallback %q, got %q", orURL, cfg.BaseURL)
			}
		}

		if setElicitModel {
			if cfg.Model != elicitModel {
				t.Fatalf("ELICITATION_MODEL set: expected %q, got %q", elicitModel, cfg.Model)
			}
		} else {
			if cfg.Model != orModel {
				t.Fatalf("ELICITATION_MODEL unset: expected OPENROUTER fallback %q, got %q", orModel, cfg.Model)
			}
		}

		if setElicitMaxTokens {
			if cfg.MaxTokens != elicitMaxTokens {
				t.Fatalf("ELICITATION_MAX_TOKENS set: expected %d, got %d", elicitMaxTokens, cfg.MaxTokens)
			}
		} else {
			if cfg.MaxTokens != orMaxTokens {
				t.Fatalf("ELICITATION_MAX_TOKENS unset: expected OPENROUTER fallback %d, got %d", orMaxTokens, cfg.MaxTokens)
			}
		}

		if setElicitTimeout {
			if int(cfg.Timeout.Seconds()) != elicitTimeout {
				t.Fatalf("ELICITATION_TIMEOUT set: expected %ds, got %v", elicitTimeout, cfg.Timeout)
			}
		} else {
			if int(cfg.Timeout.Seconds()) != orTimeout {
				t.Fatalf("ELICITATION_TIMEOUT unset: expected OPENROUTER fallback %ds, got %v", orTimeout, cfg.Timeout)
			}
		}

		// Cleanup
		os.Unsetenv("ELICITATION_API_KEY")
		os.Unsetenv("ELICITATION_BASE_URL")
		os.Unsetenv("ELICITATION_MODEL")
		os.Unsetenv("ELICITATION_MAX_TOKENS")
		os.Unsetenv("ELICITATION_TIMEOUT")
		os.Unsetenv("OPENROUTER_API_KEY")
		os.Unsetenv("OPENROUTER_BASE_URL")
		os.Unsetenv("OPENROUTER_MODEL")
		os.Unsetenv("OPENROUTER_MAX_TOKENS")
		os.Unsetenv("OPENROUTER_TIMEOUT")
	})
}

// ============================================================================
// Task 2.3: Property test — Config validation identifies invalid fields
// Feature: elicitation-engine, Property 2: Config validation identifies invalid fields
// Validates: Requirements 1.4
// ============================================================================

func TestConfigValidationErrors(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Start with a valid config
		cfg := agent.AgentConfig{
			APIKey:    rapid.StringMatching(`[a-zA-Z0-9]{1,30}`).Draw(t, "apiKey"),
			BaseURL:   rapid.StringMatching(`https://[a-z]{1,10}\.[a-z]{2,5}`).Draw(t, "baseURL"),
			Model:     rapid.StringMatching(`[a-z]{1,10}/[a-z]{1,10}`).Draw(t, "model"),
			MaxTokens: rapid.Int64Range(1, 100000).Draw(t, "maxTokens"),
		}

		// Choose which field(s) to invalidate
		invalidateAPIKey := rapid.Bool().Draw(t, "invalidateAPIKey")
		invalidateModel := rapid.Bool().Draw(t, "invalidateModel")
		invalidateMaxTokens := rapid.Bool().Draw(t, "invalidateMaxTokens")

		// Ensure at least one field is invalid
		if !invalidateAPIKey && !invalidateModel && !invalidateMaxTokens {
			invalidateAPIKey = true
		}

		if invalidateAPIKey {
			cfg.APIKey = ""
		}
		if invalidateModel {
			cfg.Model = ""
		}
		if invalidateMaxTokens {
			cfg.MaxTokens = rapid.Int64Range(-100000, 0).Draw(t, "badMaxTokens")
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatalf("expected validation error for config %+v, got nil", cfg)
		}

		errMsg := err.Error()

		// The error message should identify at least one invalid field
		// Validate() checks fields in order: APIKey, Model, MaxTokens
		// It returns on the first error found
		if invalidateAPIKey {
			if !strings.Contains(errMsg, "APIKey") {
				t.Fatalf("expected error to mention APIKey, got: %s", errMsg)
			}
		} else if invalidateModel {
			if !strings.Contains(errMsg, "Model") {
				t.Fatalf("expected error to mention Model, got: %s", errMsg)
			}
		} else if invalidateMaxTokens {
			if !strings.Contains(errMsg, "MaxTokens") {
				t.Fatalf("expected error to mention MaxTokens, got: %s", errMsg)
			}
		}
	})
}
