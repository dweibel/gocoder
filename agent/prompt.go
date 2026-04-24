package agent

import (
	anthropic "github.com/anthropics/anthropic-sdk-go"
)

const systemPreamble = `You are an expert code generator. Your role is to produce clean, well-structured, production-quality code based on the requirements provided.

Output Format:
- Generate complete, runnable code that follows best practices for the target language and framework.
- Include necessary imports, type definitions, and error handling.
- Use clear naming conventions and add concise comments where helpful.

Architectural Constraints:
- Respect all architectural constraints, non-functional requirements, and design decisions described in the SRS context below.
- Ensure generated code integrates cleanly with the existing system architecture.

SRS Context:
`

// buildSystemPrompt combines the static role preamble with the SRS context.
func buildSystemPrompt(srsContext string) string {
	return systemPreamble + srsContext
}

// buildUserMessage wraps the Gherkin story as the initial user message.
func buildUserMessage(gherkinStory string) anthropic.MessageParam {
	return anthropic.NewUserMessage(anthropic.NewTextBlock(gherkinStory))
}
