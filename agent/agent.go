package agent

import (
	"context"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// CodingAgent defines the contract for LLM-based code generation agents.
type CodingAgent interface {
	Execute(ctx context.Context, gherkinStory string, srsContext string) (AgentResult, error)
}

// MessageCreator abstracts the Anthropic SDK's Messages.New call for testability.
type MessageCreator interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

// agent is the unexported implementation of CodingAgent.
type agent struct {
	config   AgentConfig
	tools    []ToolDefinition
	messages MessageCreator
}

// Execute runs the coding agent with the given Gherkin story and SRS context.
func (a *agent) Execute(ctx context.Context, gherkinStory string, srsContext string) (AgentResult, error) {
	systemPrompt := buildSystemPrompt(srsContext)
	userMessage := buildUserMessage(gherkinStory)
	toolParams := toolsToParams(a.tools)

	params := anthropic.MessageNewParams{
		Model:     a.config.Model,
		MaxTokens: a.config.MaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{userMessage},
		Tools:    toolParams,
	}

	return a.runConversationLoop(ctx, params)
}

// NewOpenRouterClient creates an Anthropic SDK client configured for OpenRouter
// and returns its MessageService, which satisfies the MessageCreator interface.
func NewOpenRouterClient(cfg AgentConfig) MessageCreator {
	opts := []option.RequestOption{
		option.WithBaseURL(cfg.BaseURL),
		option.WithAPIKey(cfg.APIKey),
	}
	if cfg.HTTPReferer != "" {
		opts = append(opts, option.WithHeader("HTTP-Referer", cfg.HTTPReferer))
	}
	if cfg.AppTitle != "" {
		opts = append(opts, option.WithHeader("X-Title", cfg.AppTitle))
	}
	client := anthropic.NewClient(opts...)
	return &client.Messages
}

// NewCodingAgent creates a new CodingAgent with the given config and tools.
func NewCodingAgent(cfg AgentConfig, tools []ToolDefinition) CodingAgent {
	return &agent{
		config:   cfg,
		tools:    tools,
		messages: NewMessageCreator(cfg),
	}
}
