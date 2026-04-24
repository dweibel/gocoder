package agent

import (
	"context"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// runConversationLoop executes the multi-turn tool use exchange.
// It iterates until the LLM produces a response with no tool calls,
// or the maximum iteration count is reached.
func (a *agent) runConversationLoop(
	ctx context.Context,
	params anthropic.MessageNewParams,
) (AgentResult, error) {
	var totalInput, totalOutput int64
	iteration := 0

	for {
		// Respect context cancellation at each iteration.
		select {
		case <-ctx.Done():
			return AgentResult{}, fmt.Errorf("conversation loop cancelled: %w", ctx.Err())
		default:
		}

		// Send the current params to the LLM.
		response, err := a.messages.New(ctx, params)
		if err != nil {
			return AgentResult{}, fmt.Errorf("api call failed: %w", err)
		}

		// Accumulate token usage.
		totalInput += response.Usage.InputTokens
		totalOutput += response.Usage.OutputTokens

		// Check if any content block is a tool_use block.
		hasToolUse := false
		for _, block := range response.Content {
			if block.Type == "tool_use" {
				hasToolUse = true
				break
			}
		}

		if !hasToolUse {
			// Text-only response — extract final text and return.
			var finalText string
			for _, block := range response.Content {
				if block.Type == "text" {
					finalText = block.Text
					break
				}
			}
			return AgentResult{
				Code:         StripCodeFences(finalText),
				Model:        a.config.Model,
				InputTokens:  totalInput,
				OutputTokens: totalOutput,
				Success:      true,
			}, nil
		}

		// Tool use response — dispatch each tool call.
		// First, append the assistant's response to the message history.
		params.Messages = append(params.Messages, response.ToParam())

		// Build tool result blocks for each tool_use content block.
		var toolResultBlocks []anthropic.ContentBlockParamUnion
		for _, block := range response.Content {
			if block.Type != "tool_use" {
				continue
			}

			handler, err := findTool(a.tools, block.Name)
			if err != nil {
				return AgentResult{}, fmt.Errorf("tool dispatch failed [iteration=%d]: %w", iteration, err)
			}

			result, err := handler(block.Input)
			if err != nil {
				return AgentResult{}, fmt.Errorf("tool execution failed [tool=%s, iteration=%d]: %w", block.Name, iteration, err)
			}

			toolResultBlocks = append(toolResultBlocks, anthropic.NewToolResultBlock(block.ID, result, false))
		}

		// Append tool results as a user message.
		params.Messages = append(params.Messages, anthropic.NewUserMessage(toolResultBlocks...))

		iteration++
		if iteration >= a.config.MaxIterations {
			return AgentResult{}, fmt.Errorf("conversation loop exceeded maximum iteration limit of %d", a.config.MaxIterations)
		}
	}
}
