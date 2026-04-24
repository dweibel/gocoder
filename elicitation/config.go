package elicitation

import (
	"os"
	"strconv"
	"time"

	"github.com/ardp/coding-agent/agent"
)

// LoadElicitationConfig reads LLM configuration from ELICITATION_-prefixed
// environment variables, falling back to OPENROUTER_-prefixed variables when
// the ELICITATION_ variant is absent. Defaults match agent.LoadConfig.
func LoadElicitationConfig() agent.AgentConfig {
	cfg := agent.AgentConfig{
		BaseURL:   "https://openrouter.ai/api",
		Model:     "anthropic/claude-sonnet-4",
		MaxTokens: 4096,
		Timeout:   300 * time.Second,
	}

	cfg.APIKey = envWithFallback("ELICITATION_API_KEY", "OPENROUTER_API_KEY")

	if v := envWithFallback("ELICITATION_BASE_URL", "OPENROUTER_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := envWithFallback("ELICITATION_MODEL", "OPENROUTER_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := envWithFallback("ELICITATION_MAX_TOKENS", "OPENROUTER_MAX_TOKENS"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxTokens = parsed
		}
	}
	if v := envWithFallback("ELICITATION_TIMEOUT", "OPENROUTER_TIMEOUT"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.Timeout = time.Duration(parsed) * time.Second
		}
	}

	return cfg
}

// envWithFallback returns the value of primary if set and non-empty,
// otherwise returns the value of fallback.
func envWithFallback(primary, fallback string) string {
	if v := os.Getenv(primary); v != "" {
		return v
	}
	return os.Getenv(fallback)
}
