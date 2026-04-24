package agent

import (
	"encoding/json"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
)

// ToolHandler is a function that executes a tool call and returns the result.
type ToolHandler func(input json.RawMessage) (string, error)

// ToolDefinition describes a tool the LLM can invoke during generation.
type ToolDefinition struct {
	Name        string          // Tool name
	Description string          // Human-readable description
	InputSchema json.RawMessage // JSON Schema for the tool's input (raw JSON)
	Handler     ToolHandler     // Function to execute when the tool is called
}

// toolsToParams converts a slice of ToolDefinition to Anthropic SDK ToolUnionParam entries.
func toolsToParams(tools []ToolDefinition) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		var schema anthropic.ToolInputSchemaParam
		if len(t.InputSchema) > 0 {
			_ = json.Unmarshal(t.InputSchema, &schema)
		}
		params[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: schema,
			},
		}
	}
	return params
}

// findTool looks up a tool handler by name. Returns an error if the tool is not registered.
func findTool(tools []ToolDefinition, name string) (ToolHandler, error) {
	for _, t := range tools {
		if t.Name == name {
			return t.Handler, nil
		}
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}
