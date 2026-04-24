package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// Feature: ardp-coding-agent, Property 10: AgentResult JSON round-trip
// Validates: Requirements 8.4
func TestAgentResultJSONRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := AgentResult{
			Code:         rapid.String().Draw(t, "code"),
			Model:        rapid.String().Draw(t, "model"),
			InputTokens:  rapid.Int64().Draw(t, "inputTokens"),
			OutputTokens: rapid.Int64().Draw(t, "outputTokens"),
			Success:      rapid.Bool().Draw(t, "success"),
		}

		data, err := SerializeResult(original)
		if err != nil {
			t.Fatalf("SerializeResult failed: %v", err)
		}

		var roundTripped AgentResult
		if err := json.Unmarshal(data, &roundTripped); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		if roundTripped.Code != original.Code {
			t.Fatalf("Code mismatch: got %q, want %q", roundTripped.Code, original.Code)
		}
		if roundTripped.Model != original.Model {
			t.Fatalf("Model mismatch: got %q, want %q", roundTripped.Model, original.Model)
		}
		if roundTripped.InputTokens != original.InputTokens {
			t.Fatalf("InputTokens mismatch: got %d, want %d", roundTripped.InputTokens, original.InputTokens)
		}
		if roundTripped.OutputTokens != original.OutputTokens {
			t.Fatalf("OutputTokens mismatch: got %d, want %d", roundTripped.OutputTokens, original.OutputTokens)
		}
		if roundTripped.Success != original.Success {
			t.Fatalf("Success mismatch: got %v, want %v", roundTripped.Success, original.Success)
		}
	})
}

// Feature: ardp-coding-agent, Property 11: AgentResult serialization contains required fields
// Validates: Requirements 8.1, 8.2
func TestAgentResultJSONFields(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		result := AgentResult{
			Code:         rapid.String().Draw(t, "code"),
			Model:        rapid.String().Draw(t, "model"),
			InputTokens:  rapid.Int64().Draw(t, "inputTokens"),
			OutputTokens: rapid.Int64().Draw(t, "outputTokens"),
			Success:      rapid.Bool().Draw(t, "success"),
		}

		data, err := SerializeResult(result)
		if err != nil {
			t.Fatalf("SerializeResult failed: %v", err)
		}

		jsonStr := string(data)
		requiredFields := []string{
			`"code"`,
			`"model"`,
			`"input_tokens"`,
			`"output_tokens"`,
			`"success"`,
		}
		for _, field := range requiredFields {
			if !strings.Contains(jsonStr, field) {
				t.Fatalf("serialized JSON missing required field %s: %s", field, jsonStr)
			}
		}
	})
}

// Feature: ardp-coding-agent, Property 12: Error JSON structure
// Validates: Requirements 8.3
func TestErrorJSONStructure(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		errMsg := rapid.String().Draw(t, "errorMessage")

		data, err := SerializeError(errMsg)
		if err != nil {
			t.Fatalf("SerializeError failed: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		errorVal, ok := parsed["error"]
		if !ok {
			t.Fatal("error JSON missing 'error' field")
		}
		if errorVal != errMsg {
			t.Fatalf("error field mismatch: got %q, want %q", errorVal, errMsg)
		}

		successVal, ok := parsed["success"]
		if !ok {
			t.Fatal("error JSON missing 'success' field")
		}
		if successVal != false {
			t.Fatalf("success field should be false, got %v", successVal)
		}
	})
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "go fenced block",
			input: "```go\npackage main\n\nfunc main() {}\n```",
			want:  "package main\n\nfunc main() {}",
		},
		{
			name:  "fenced block no language",
			input: "```\nsome code\n```",
			want:  "some code",
		},
		{
			name:  "no fences passthrough",
			input: "package main\n\nfunc main() {}",
			want:  "package main\n\nfunc main() {}",
		},
		{
			name:  "fences with surrounding whitespace",
			input: "  \n```go\nfmt.Println(\"hi\")\n```\n  ",
			want:  "fmt.Println(\"hi\")",
		},
		{
			name:  "fences with extra text after closing",
			input: "```go\ncode here\n```\n\nSome explanation text",
			want:  "```go\ncode here\n```\n\nSome explanation text",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only fences no content",
			input: "```go\n```",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripCodeFences(tt.input)
			if got != tt.want {
				t.Errorf("StripCodeFences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
