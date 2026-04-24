package elicitation

import (
	"context"
	"errors"
	"fmt"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/ardp/coding-agent/agent"
)

// Elicitor defines the contract for the elicitation engine.
type Elicitor interface {
	// Chat sends a user message and returns the assistant response.
	Chat(ctx context.Context, session *Session, userMessage string) (ChatMessage, error)

	// Synthesize generates artifacts from the session's chat history.
	Synthesize(ctx context.Context, session *Session) ([]Artifact, error)
}

// Artifact represents a synthesized document produced from an elicitation session.
type Artifact struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// Engine orchestrates chat turns and synthesis for elicitation sessions.
type Engine struct {
	messages   agent.MessageCreator
	config     agent.AgentConfig
	personas   PersonaLoader
	promptsDir string
}

// NewEngine creates a new Engine with the given dependencies.
func NewEngine(messages agent.MessageCreator, config agent.AgentConfig, personas PersonaLoader) *Engine {
	return &Engine{
		messages:   messages,
		config:     config,
		personas:   personas,
		promptsDir: defaultPromptsDir,
	}
}

// SetPromptsDir overrides the default prompts directory.
func (e *Engine) SetPromptsDir(dir string) {
	e.promptsDir = dir
}

// Chat sends a user message within a session and returns the assistant response.
// It builds MessageNewParams from the persona prompt and full session history,
// calls the LLM, and appends both user and assistant messages atomically on success.
func (e *Engine) Chat(ctx context.Context, session *Session, userMessage string) (ChatMessage, error) {
	// Load the persona system prompt.
	systemPrompt, err := e.personas.Load(session.Persona)
	if err != nil {
		return ChatMessage{}, fmt.Errorf("failed to load persona: %w", err)
	}

	// Build the message history from session state.
	messages := make([]anthropic.MessageParam, 0, len(session.Messages)+1)
	for _, m := range session.Messages {
		switch m.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(m.Content)))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(m.Content)))
		}
	}

	// Add the new user message.
	messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)))

	params := anthropic.MessageNewParams{
		Model:     e.config.Model,
		MaxTokens: e.config.MaxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: messages,
	}

	// Call the LLM.
	resp, err := e.messages.New(ctx, params)
	if err != nil {
		return ChatMessage{}, wrapLLMError(err)
	}

	// Extract the text response from the first text content block.
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	now := time.Now()

	// Append both messages atomically only on success.
	userMsg := ChatMessage{
		Role:    "user",
		Content: userMessage,
		SentAt:  now,
	}
	assistantMsg := ChatMessage{
		Role:    "assistant",
		Content: responseText,
		SentAt:  now,
	}
	session.Messages = append(session.Messages, userMsg, assistantMsg)

	return assistantMsg, nil
}

// wrapLLMError wraps an LLM error, detecting timeouts and API errors.
func wrapLLMError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %v", ErrLLMTimeout, err)
	}
	// Use the same wrapAPIError pattern from agent/errors.go
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("api call failed [status=%d]: %w", apiErr.StatusCode, err)
	}
	return fmt.Errorf("api call failed: %w", err)
}
