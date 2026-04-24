package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

// AgentResult is the structured output of a CodingAgent execution.
type AgentResult struct {
	Code         string `json:"code"`
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Success      bool   `json:"success"`
}

// SerializeResult serializes an AgentResult as indented JSON.
func SerializeResult(result AgentResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// codeBlockRe matches a fenced code block with an optional language tag.
var codeBlockRe = regexp.MustCompile("(?s)^```[a-zA-Z]*\\n?(.*?)\\n?```$")

// StripCodeFences removes surrounding markdown code fences from s.
// If s does not start with ``` and end with ```, it is returned unchanged.
func StripCodeFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if m := codeBlockRe.FindStringSubmatch(trimmed); m != nil {
		return strings.TrimSpace(m[1])
	}
	return s
}

// SerializeError serializes an error response as indented JSON with "error" and "success" fields.
func SerializeError(errMsg string) ([]byte, error) {
	return json.MarshalIndent(struct {
		Error   string `json:"error"`
		Success bool   `json:"success"`
	}{
		Error:   errMsg,
		Success: false,
	}, "", "  ")
}
