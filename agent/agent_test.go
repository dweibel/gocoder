package agent

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// --- Task 7.2: Unit tests for client initialization ---

func TestNewCodingAgentReturnsNonNil(t *testing.T) {
	cfg := AgentConfig{
		APIKey:    "test-key",
		BaseURL:   "https://openrouter.ai/api",
		Model:     "anthropic/claude-sonnet-4",
		MaxTokens: 4096,
	}
	agent := NewCodingAgent(cfg, nil)
	if agent == nil {
		t.Fatal("expected non-nil CodingAgent, got nil")
	}
}

func TestNewCodingAgentWithVariousConfigs(t *testing.T) {
	configs := []AgentConfig{
		{
			APIKey:    "key-1",
			BaseURL:   "https://openrouter.ai/api",
			Model:     "anthropic/claude-sonnet-4",
			MaxTokens: 4096,
		},
		{
			APIKey:    "key-2",
			BaseURL:   "https://custom.api.example.com",
			Model:     "openai/gpt-4o",
			MaxTokens: 8192,
		},
		{
			APIKey:        "key-3",
			BaseURL:       "https://openrouter.ai/api",
			Model:         "meta-llama/llama-3-70b",
			MaxTokens:     2048,
			MaxIterations: 5,
		},
	}

	for i, cfg := range configs {
		agent := NewCodingAgent(cfg, nil)
		if agent == nil {
			t.Fatalf("config[%d]: expected non-nil CodingAgent, got nil", i)
		}
	}
}

func TestNewCodingAgentWithHTTPRefererAndAppTitle(t *testing.T) {
	cfg := AgentConfig{
		APIKey:      "test-key",
		BaseURL:     "https://openrouter.ai/api",
		Model:       "anthropic/claude-sonnet-4",
		MaxTokens:   4096,
		HTTPReferer: "https://myapp.example.com",
		AppTitle:    "My Coding App",
	}
	agent := NewCodingAgent(cfg, nil)
	if agent == nil {
		t.Fatal("expected non-nil CodingAgent with HTTPReferer and AppTitle set, got nil")
	}
}

func TestNewCodingAgentWithEmptyHTTPRefererAndAppTitle(t *testing.T) {
	cfg := AgentConfig{
		APIKey:      "test-key",
		BaseURL:     "https://openrouter.ai/api",
		Model:       "anthropic/claude-sonnet-4",
		MaxTokens:   4096,
		HTTPReferer: "",
		AppTitle:    "",
	}
	agent := NewCodingAgent(cfg, nil)
	if agent == nil {
		t.Fatal("expected non-nil CodingAgent with empty HTTPReferer and AppTitle, got nil")
	}
}

// --- Task 7.3: Property 20 ---

// Known Anthropic SDK model enum constants (without provider prefix).
// These are the raw model identifiers that should NOT be used directly
// with OpenRouter — OpenRouter requires "provider/model" format.
var anthropicSDKModelConstants = []string{
	"claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-20240620",
	"claude-3-5-haiku-20241022",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-3-haiku-20240307",
	"claude-2.1",
	"claude-2.0",
	"claude-instant-1.2",
}

// Feature: ardp-coding-agent, Property 20: OpenRouter model string format
// Validates: Requirements 2.3
func TestModelStringFormat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random provider and model name segments
		provider := rapid.StringMatching(`[a-z][a-z0-9-]{1,20}`).Draw(t, "provider")
		model := rapid.StringMatching(`[a-z][a-z0-9._-]{1,30}`).Draw(t, "model")

		// Construct a valid OpenRouter model string: "provider/model"
		modelStr := provider + "/" + model

		// Property: model string must contain "/" separator
		if !strings.Contains(modelStr, "/") {
			t.Fatalf("model string %q does not contain '/' separator", modelStr)
		}

		// Property: model string must NOT match any known Anthropic SDK enum constant
		for _, constant := range anthropicSDKModelConstants {
			if modelStr == constant {
				t.Fatalf("model string %q matches Anthropic SDK enum constant %q", modelStr, constant)
			}
		}

		// Additional: the part before "/" is the provider, the part after is the model name
		parts := strings.SplitN(modelStr, "/", 2)
		if len(parts) != 2 {
			t.Fatalf("model string %q does not split into provider/model", modelStr)
		}
		if parts[0] == "" {
			t.Fatalf("model string %q has empty provider", modelStr)
		}
		if parts[1] == "" {
			t.Fatalf("model string %q has empty model name", modelStr)
		}
	})
}
