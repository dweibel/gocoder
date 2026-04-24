package agent

import (
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"pgregory.net/rapid"
)

// Feature: ardp-coding-agent, Property 1: Prompt assembly preserves inputs
// Validates: Requirements 3.1, 3.2, 3.4
func TestPromptAssemblyPreservesInputs(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		srsContext := rapid.String().Draw(t, "srsContext")
		gherkinStory := rapid.String().Draw(t, "gherkinStory")

		// System prompt must contain the SRS context
		sysPrompt := buildSystemPrompt(srsContext)
		if !strings.Contains(sysPrompt, srsContext) {
			t.Fatalf("system prompt does not contain srsContext %q", srsContext)
		}

		// User message must contain the Gherkin story
		msg := buildUserMessage(gherkinStory)
		text := extractMessageText(t, msg)
		if !strings.Contains(text, gherkinStory) {
			t.Fatalf("user message does not contain gherkinStory %q", gherkinStory)
		}
	})
}

// Unit tests for prompt structure

func TestSystemPromptContainsRoleDefinition(t *testing.T) {
	prompt := buildSystemPrompt("some context")
	if !strings.Contains(strings.ToLower(prompt), "code generator") {
		t.Fatal("system prompt missing role definition ('code generator')")
	}
}

func TestSystemPromptContainsOutputFormatInstructions(t *testing.T) {
	prompt := buildSystemPrompt("some context")
	if !strings.Contains(prompt, "Output Format") {
		t.Fatal("system prompt missing output format instructions ('Output Format')")
	}
}

func TestSystemPromptContainsSRSContext(t *testing.T) {
	ctx := "My custom SRS architecture constraints"
	prompt := buildSystemPrompt(ctx)
	if !strings.Contains(prompt, ctx) {
		t.Fatalf("system prompt does not contain SRS context %q", ctx)
	}
}

func TestUserMessageWrapsGherkinStory(t *testing.T) {
	story := "Given a user\nWhen they login\nThen they see the dashboard"
	msg := buildUserMessage(story)

	if msg.Role != "user" {
		t.Fatalf("expected role 'user', got %q", msg.Role)
	}

	text := extractMessageText(t, msg)
	if !strings.Contains(text, story) {
		t.Fatalf("user message does not contain gherkin story")
	}
}

// fataler is satisfied by both *testing.T and *rapid.T.
type fataler interface {
	Fatalf(format string, args ...any)
}

// extractMessageText concatenates all text content from a MessageParam.
// It fails the test if no text content blocks exist at all.
func extractMessageText(t fataler, msg anthropic.MessageParam) string {
	var sb strings.Builder
	found := false
	for _, block := range msg.Content {
		if block.OfText != nil {
			sb.WriteString(block.OfText.Text)
			found = true
		}
	}
	if !found {
		t.Fatalf("message contains no text content blocks")
	}
	return sb.String()
}
