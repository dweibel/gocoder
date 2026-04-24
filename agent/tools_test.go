package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ardp-coding-agent, Property 3: Tool definitions forwarded as ToolParams
// Validates: Requirements 5.2
func TestToolDefinitionsForwarded(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 10).Draw(t, "numTools")
		tools := make([]ToolDefinition, n)
		for i := 0; i < n; i++ {
			tools[i] = ToolDefinition{
				Name:        rapid.StringMatching(`[a-zA-Z_][a-zA-Z0-9_]{0,20}`).Draw(t, fmt.Sprintf("name_%d", i)),
				Description: rapid.String().Draw(t, fmt.Sprintf("desc_%d", i)),
				InputSchema: json.RawMessage(`{}`),
				Handler:     nil,
			}
		}

		params := toolsToParams(tools)

		if len(params) != len(tools) {
			t.Fatalf("expected %d params, got %d", len(tools), len(params))
		}

		for i, tool := range tools {
			p := params[i]
			if p.OfTool == nil {
				t.Fatalf("params[%d].OfTool is nil", i)
			}
			if p.OfTool.Name != tool.Name {
				t.Fatalf("params[%d] name mismatch: got %q, want %q", i, p.OfTool.Name, tool.Name)
			}
			if p.OfTool.Description.Value != tool.Description {
				t.Fatalf("params[%d] description mismatch: got %q, want %q", i, p.OfTool.Description.Value, tool.Description)
			}
		}
	})
}

// Feature: ardp-coding-agent, Property 6: Unknown tool call returns error
// Validates: Requirements 5.6
func TestUnknownToolError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 10).Draw(t, "numTools")
		knownNames := make(map[string]bool, n)
		tools := make([]ToolDefinition, n)
		for i := 0; i < n; i++ {
			name := rapid.StringMatching(`[a-z]{3,15}`).Draw(t, fmt.Sprintf("known_%d", i))
			knownNames[name] = true
			tools[i] = ToolDefinition{
				Name:    name,
				Handler: func(input json.RawMessage) (string, error) { return "", nil },
			}
		}

		// Generate a name guaranteed not to be in the known set
		unknown := rapid.StringMatching(`[a-z]{3,15}`).Filter(func(s string) bool {
			return !knownNames[s]
		}).Draw(t, "unknownName")

		_, err := findTool(tools, unknown)
		if err == nil {
			t.Fatalf("expected error for unknown tool %q, got nil", unknown)
		}
		if !strings.Contains(err.Error(), unknown) {
			t.Fatalf("error %q does not contain unknown tool name %q", err.Error(), unknown)
		}
	})
}

// Feature: ardp-coding-agent, Property 18: Tool errors wrapped with tool name and iteration
// Validates: Requirements 10.4
func TestToolErrorWrapping(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolName := rapid.StringMatching(`[a-zA-Z_][a-zA-Z0-9_]{0,20}`).Draw(t, "toolName")
		iteration := rapid.IntRange(0, 1000).Draw(t, "iteration")
		errMsg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, "errMsg")

		wrapped := wrapToolError(toolName, iteration, errors.New(errMsg))

		errStr := wrapped.Error()
		if !strings.Contains(errStr, toolName) {
			t.Fatalf("wrapped error %q does not contain tool name %q", errStr, toolName)
		}
		iterStr := fmt.Sprintf("%d", iteration)
		if !strings.Contains(errStr, iterStr) {
			t.Fatalf("wrapped error %q does not contain iteration %q", errStr, iterStr)
		}
	})
}
