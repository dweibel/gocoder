package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// openAIAdapter implements MessageCreator by calling the OpenAI-compatible
// /v1/chat/completions endpoint. This allows non-Anthropic models (e.g. Qwen)
// to be used via OpenRouter.
type openAIAdapter struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// openAIChatRequest is the request body for /v1/chat/completions.
type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int64           `json:"max_tokens,omitempty"`
}

// openAIMessage represents a single message in the OpenAI chat format.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the response from /v1/chat/completions.
type openAIChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

// New translates Anthropic MessageNewParams into an OpenAI chat completion
// request, calls the API, and translates the response back to an
// anthropic.Message so the rest of the codebase is unaffected.
func (a *openAIAdapter) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	var msgs []openAIMessage

	// System prompt from params.System.
	for _, block := range params.System {
		if block.Text != "" {
			msgs = append(msgs, openAIMessage{Role: "system", Content: block.Text})
		}
	}

	// Conversation messages — extract text using the same pattern as the test helpers.
	for _, mp := range params.Messages {
		role := string(mp.Role)
		var contentParts []string
		for _, block := range mp.Content {
			if block.OfText != nil {
				contentParts = append(contentParts, block.OfText.Text)
			}
		}
		if len(contentParts) > 0 {
			msgs = append(msgs, openAIMessage{Role: role, Content: strings.Join(contentParts, "\n")})
		}
	}

	reqBody := openAIChatRequest{
		Model:     params.Model,
		Messages:  msgs,
		MaxTokens: params.MaxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := strings.TrimRight(a.baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("POST %q: %d %s %s", url, resp.StatusCode, resp.Status, string(respBytes))
	}

	var chatResp openAIChatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if chatResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Translate to anthropic.Message via JSON unmarshal (same pattern as test helpers).
	content := chatResp.Choices[0].Message.Content
	msgJSON := fmt.Sprintf(
		`{"id":%q,"content":[{"type":"text","text":%s}],"usage":{"input_tokens":%d,"output_tokens":%d},"stop_reason":"end_turn","model":%q,"role":"assistant","type":"message"}`,
		chatResp.ID,
		mustMarshalJSON(content),
		chatResp.Usage.PromptTokens,
		chatResp.Usage.CompletionTokens,
		reqBody.Model,
	)
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		return nil, fmt.Errorf("failed to construct response message: %w", err)
	}

	return &msg, nil
}

// mustMarshalJSON marshals a string to a JSON string value (with escaping).
func mustMarshalJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// IsAnthropicModel returns true if the model string indicates an Anthropic
// model that should use the native Anthropic /v1/messages endpoint.
func IsAnthropicModel(model string) bool {
	return strings.HasPrefix(model, "anthropic/")
}

// NewMessageCreator creates the appropriate MessageCreator based on the model.
// Anthropic models use the native SDK; all others use the OpenAI-compatible adapter.
func NewMessageCreator(cfg AgentConfig) MessageCreator {
	if IsAnthropicModel(cfg.Model) {
		return NewOpenRouterClient(cfg)
	}
	return &openAIAdapter{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}
