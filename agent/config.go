package agent

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// AgentConfig holds all configuration for a CodingAgent instance.
type AgentConfig struct {
	APIKey        string        // OpenRouter API key
	BaseURL       string        // OpenRouter base URL (default: https://openrouter.ai/api)
	Model         string        // OpenRouter model string (e.g., "anthropic/claude-sonnet-4")
	MaxTokens     int64         // Maximum token limit for LLM responses
	MaxIterations int           // Maximum conversation loop iterations
	Timeout       time.Duration // Context deadline for the entire execution
	HTTPReferer   string        // Optional: OpenRouter HTTP-Referer header
	AppTitle      string        // Optional: OpenRouter X-Title header
}

// LoadConfig reads configuration from environment variables and applies defaults.
func LoadConfig() AgentConfig {
	cfg := AgentConfig{
		APIKey:        os.Getenv("OPENROUTER_API_KEY"),
		BaseURL:       "https://openrouter.ai/api",
		Model:         "anthropic/claude-sonnet-4",
		MaxTokens:     4096,
		MaxIterations: 10,
		Timeout:       300 * time.Second,
	}

	if v := os.Getenv("OPENROUTER_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("OPENROUTER_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("OPENROUTER_MAX_TOKENS"); v != "" {
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.MaxTokens = parsed
		}
	}
	if v := os.Getenv("OPENROUTER_TIMEOUT"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.Timeout = time.Duration(parsed) * time.Second
		}
	}

	return cfg
}

// Validate checks that the AgentConfig has all required fields and valid values.
func (c AgentConfig) Validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("configuration error: APIKey must be non-empty")
	}
	if c.Model == "" {
		return fmt.Errorf("configuration error: Model must be non-empty")
	}
	if c.MaxTokens <= 0 {
		return fmt.Errorf("configuration error: MaxTokens must be a positive integer, got %d", c.MaxTokens)
	}
	return nil
}
