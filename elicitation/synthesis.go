package elicitation

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// defaultPromptsDir is the default directory for prompt templates.
const defaultPromptsDir = "prompts"

// Synthesize generates artifacts from the session's chat history.
// It loads the synthesis prompt template, renders it with the session's messages,
// calls the LLM, and parses the response into typed artifacts.
// On failure, the session state is not mutated.
func (e *Engine) Synthesize(ctx context.Context, session *Session) ([]Artifact, error) {
	// Build the synthesis prompt from the template
	prompt, err := e.buildSynthesisPrompt(session)
	if err != nil {
		return nil, fmt.Errorf("failed to build synthesis prompt: %w", err)
	}

	// Build LLM params — synthesis uses a single user message with the rendered prompt
	params := anthropic.MessageNewParams{
		Model:     e.config.Model,
		MaxTokens: e.config.MaxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	// Call the LLM
	resp, err := e.messages.New(ctx, params)
	if err != nil {
		return nil, wrapLLMError(err)
	}

	// Extract text response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText = block.Text
			break
		}
	}

	// Parse the response into artifacts
	codec := NewArtifactCodec()
	artifacts, err := codec.Parse(responseText)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSynthesisParse, err)
	}

	return artifacts, nil
}

// buildSynthesisPrompt loads and renders the synthesis prompt template
// with the session's chat history.
func (e *Engine) buildSynthesisPrompt(session *Session) (string, error) {
	tmplPath := filepath.Join(e.promptsDir, "synthesis.tmpl")
	tmplData, err := os.ReadFile(tmplPath)
	if err != nil {
		return "", fmt.Errorf("failed to read synthesis template: %w", err)
	}

	tmpl, err := template.New("synthesis").Parse(string(tmplData))
	if err != nil {
		return "", fmt.Errorf("failed to parse synthesis template: %w", err)
	}

	data := struct {
		Messages []ChatMessage
	}{
		Messages: session.Messages,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute synthesis template: %w", err)
	}

	return buf.String(), nil
}
