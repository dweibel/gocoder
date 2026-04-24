package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"pgregory.net/rapid"
)

// --- Mock infrastructure ---

// mockMessageCreator implements the MessageCreator interface for testing.
type mockMessageCreator struct {
	responses []*anthropic.Message
	callIndex int
	calls     []anthropic.MessageNewParams
}

func (m *mockMessageCreator) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	m.calls = append(m.calls, params)
	if m.callIndex >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

// mustMarshalString marshals a string to a JSON string literal.
func mustMarshalString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// makeTextResponse creates a mock anthropic.Message with a single text content block.
func makeTextResponse(text string, inputTokens, outputTokens int64) *anthropic.Message {
	jsonStr := fmt.Sprintf(`{"id":"msg_test","content":[{"type":"text","text":%s}],"usage":{"input_tokens":%d,"output_tokens":%d},"stop_reason":"end_turn","model":"test-model","role":"assistant","type":"message"}`,
		mustMarshalString(text), inputTokens, outputTokens)
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		panic(fmt.Sprintf("makeTextResponse unmarshal failed: %v", err))
	}
	return &msg
}

// makeToolUseResponse creates a mock anthropic.Message with a single tool_use content block.
func makeToolUseResponse(toolName, toolID string, input json.RawMessage, inputTokens, outputTokens int64) *anthropic.Message {
	jsonStr := fmt.Sprintf(`{"id":"msg_test","content":[{"type":"tool_use","id":%s,"name":%s,"input":%s}],"usage":{"input_tokens":%d,"output_tokens":%d},"stop_reason":"tool_use","model":"test-model","role":"assistant","type":"message"}`,
		mustMarshalString(toolID), mustMarshalString(toolName), string(input), inputTokens, outputTokens)
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		panic(fmt.Sprintf("makeToolUseResponse unmarshal failed: %v", err))
	}
	return &msg
}

// newTestAgent creates an agent with the given mock and tools for testing.
func newTestAgent(mock *mockMessageCreator, tools []ToolDefinition, maxIterations int) *agent {
	if maxIterations <= 0 {
		maxIterations = 10
	}
	return &agent{
		config: AgentConfig{
			Model:         "test/model",
			MaxTokens:     4096,
			MaxIterations: maxIterations,
		},
		tools:    tools,
		messages: mock,
	}
}

// --- Task 8.2: Property 2 ---

// Feature: ardp-coding-agent, Property 2: AgentResult populated from LLM response
// Validates: Requirements 4.2, 4.3
func TestAgentResultFromResponse(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := rapid.String().Draw(t, "text")
		inputTokens := rapid.Int64Range(0, 100000).Draw(t, "inputTokens")
		outputTokens := rapid.Int64Range(0, 100000).Draw(t, "outputTokens")

		mock := &mockMessageCreator{
			responses: []*anthropic.Message{
				makeTextResponse(text, inputTokens, outputTokens),
			},
		}
		a := newTestAgent(mock, nil, 10)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		result, err := a.runConversationLoop(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Code != text {
			t.Fatalf("Code mismatch: got %q, want %q", result.Code, text)
		}
		if result.InputTokens != inputTokens {
			t.Fatalf("InputTokens mismatch: got %d, want %d", result.InputTokens, inputTokens)
		}
		if result.OutputTokens != outputTokens {
			t.Fatalf("OutputTokens mismatch: got %d, want %d", result.OutputTokens, outputTokens)
		}
		if !result.Success {
			t.Fatal("expected Success to be true")
		}
	})
}

// --- Task 8.3: Property 4 ---

// Feature: ardp-coding-agent, Property 4: Tool calls dispatched and results appended
// Validates: Requirements 5.3, 5.4
func TestToolDispatchAndHistory(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		toolInput := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "toolInput")
		toolResult := rapid.StringMatching(`[a-zA-Z0-9]{1,20}`).Draw(t, "toolResult")
		finalText := rapid.String().Draw(t, "finalText")

		var handlerCalled bool
		var receivedInput json.RawMessage

		tools := []ToolDefinition{
			{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(input json.RawMessage) (string, error) {
					handlerCalled = true
					receivedInput = input
					return toolResult, nil
				},
			},
		}

		inputJSON, _ := json.Marshal(map[string]string{"arg": toolInput})

		mock := &mockMessageCreator{
			responses: []*anthropic.Message{
				makeToolUseResponse("test_tool", "tool_123", inputJSON, 100, 50),
				makeTextResponse(finalText, 100, 50),
			},
		}
		a := newTestAgent(mock, tools, 10)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		result, err := a.runConversationLoop(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !handlerCalled {
			t.Fatal("expected tool handler to be called")
		}

		// Verify the handler received the correct input
		var gotInput map[string]string
		if err := json.Unmarshal(receivedInput, &gotInput); err != nil {
			t.Fatalf("failed to unmarshal received input: %v", err)
		}
		if gotInput["arg"] != toolInput {
			t.Fatalf("tool input mismatch: got %q, want %q", gotInput["arg"], toolInput)
		}

		// Verify mock was called twice (tool_use + final text)
		if len(mock.calls) != 2 {
			t.Fatalf("expected 2 API calls, got %d", len(mock.calls))
		}

		// Verify second call has history (messages grew)
		if len(mock.calls[1].Messages) < 2 {
			t.Fatalf("expected message history to grow, got %d messages", len(mock.calls[1].Messages))
		}

		if result.Code != finalText {
			t.Fatalf("final text mismatch: got %q, want %q", result.Code, finalText)
		}
	})
}

// --- Task 8.4: Property 5 ---

// Feature: ardp-coding-agent, Property 5: Text-only response terminates loop
// Validates: Requirements 5.5, 6.2
func TestTextOnlyTerminatesLoop(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		text := rapid.String().Draw(t, "text")

		mock := &mockMessageCreator{
			responses: []*anthropic.Message{
				makeTextResponse(text, 10, 20),
			},
		}
		a := newTestAgent(mock, nil, 10)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		result, err := a.runConversationLoop(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Should return immediately with the text
		if result.Code != text {
			t.Fatalf("Code mismatch: got %q, want %q", result.Code, text)
		}

		// Mock should have been called exactly once
		if len(mock.calls) != 1 {
			t.Fatalf("expected exactly 1 API call, got %d", len(mock.calls))
		}
	})
}

// --- Task 8.5: Property 7 ---

// Feature: ardp-coding-agent, Property 7: Message history preserved across turns
// Validates: Requirements 6.1
func TestHistoryPreserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 5).Draw(t, "numToolTurns")

		tools := []ToolDefinition{
			{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(input json.RawMessage) (string, error) {
					return "ok", nil
				},
			},
		}

		// Build N tool-use responses followed by a final text response
		responses := make([]*anthropic.Message, 0, n+1)
		for i := 0; i < n; i++ {
			responses = append(responses, makeToolUseResponse("test_tool", fmt.Sprintf("tool_%d", i), json.RawMessage(`{"x":1}`), 10, 10))
		}
		responses = append(responses, makeTextResponse("done", 10, 10))

		mock := &mockMessageCreator{
			responses: responses,
		}
		a := newTestAgent(mock, tools, n+1)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		_, err := a.runConversationLoop(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify message history grows with each turn.
		// Call 0: initial messages (empty).
		// Call k (k>=1): should have 2*k additional messages (assistant + tool_result per turn).
		if len(mock.calls) != n+1 {
			t.Fatalf("expected %d API calls, got %d", n+1, len(mock.calls))
		}

		for i := 1; i <= n; i++ {
			expectedMsgs := 2 * i // each tool turn adds assistant msg + tool result msg
			actualMsgs := len(mock.calls[i].Messages)
			if actualMsgs != expectedMsgs {
				t.Fatalf("call %d: expected %d messages in history, got %d", i, expectedMsgs, actualMsgs)
			}
		}
	})
}

// --- Task 8.6: Property 8 ---

// Feature: ardp-coding-agent, Property 8: Maximum iteration limit enforced
// Validates: Requirements 6.4
func TestMaxIterationLimit(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		maxIter := rapid.IntRange(1, 10).Draw(t, "maxIterations")

		tools := []ToolDefinition{
			{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(input json.RawMessage) (string, error) {
					return "ok", nil
				},
			},
		}

		// Create enough tool-use responses to exceed the limit.
		// The loop: send → tool_use → iteration++ → check limit.
		// For MaxIterations=M, it processes M tool-use rounds then errors.
		// That's M API calls total.
		responses := make([]*anthropic.Message, 0, maxIter+1)
		for i := 0; i < maxIter+1; i++ {
			responses = append(responses, makeToolUseResponse("test_tool", fmt.Sprintf("tool_%d", i), json.RawMessage(`{"x":1}`), 10, 10))
		}

		mock := &mockMessageCreator{
			responses: responses,
		}
		a := newTestAgent(mock, tools, maxIter)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		_, err := a.runConversationLoop(context.Background(), params)
		if err == nil {
			t.Fatal("expected error for exceeding max iterations, got nil")
		}

		if !strings.Contains(err.Error(), fmt.Sprintf("%d", maxIter)) {
			t.Fatalf("error %q does not mention the limit %d", err.Error(), maxIter)
		}

		// Verify exactly M API calls were made
		if len(mock.calls) != maxIter {
			t.Fatalf("expected %d API calls, got %d", maxIter, len(mock.calls))
		}
	})
}

// --- Task 8.7: Property 9 ---

// Feature: ardp-coding-agent, Property 9: Token accumulation across iterations
// Validates: Requirements 6.5
func TestTokenAccumulation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 5).Draw(t, "numToolTurns")

		tools := []ToolDefinition{
			{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{}`),
				Handler: func(input json.RawMessage) (string, error) {
					return "ok", nil
				},
			},
		}

		var expectedInput, expectedOutput int64
		responses := make([]*anthropic.Message, 0, n+1)

		for i := 0; i < n; i++ {
			inTok := rapid.Int64Range(1, 1000).Draw(t, fmt.Sprintf("inTok_%d", i))
			outTok := rapid.Int64Range(1, 1000).Draw(t, fmt.Sprintf("outTok_%d", i))
			expectedInput += inTok
			expectedOutput += outTok
			responses = append(responses, makeToolUseResponse("test_tool", fmt.Sprintf("tool_%d", i), json.RawMessage(`{"x":1}`), inTok, outTok))
		}

		// Final text response with its own token counts
		finalIn := rapid.Int64Range(1, 1000).Draw(t, "finalIn")
		finalOut := rapid.Int64Range(1, 1000).Draw(t, "finalOut")
		expectedInput += finalIn
		expectedOutput += finalOut
		responses = append(responses, makeTextResponse("done", finalIn, finalOut))

		mock := &mockMessageCreator{
			responses: responses,
		}
		a := newTestAgent(mock, tools, n+1)

		params := anthropic.MessageNewParams{
			Model:     "test/model",
			MaxTokens: 4096,
			Messages:  []anthropic.MessageParam{},
		}

		result, err := a.runConversationLoop(context.Background(), params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.InputTokens != expectedInput {
			t.Fatalf("InputTokens mismatch: got %d, want %d", result.InputTokens, expectedInput)
		}
		if result.OutputTokens != expectedOutput {
			t.Fatalf("OutputTokens mismatch: got %d, want %d", result.OutputTokens, expectedOutput)
		}
	})
}

// --- Task 8.8: Unit tests for conversation loop edge cases ---

func TestConversationLoopContextCancellation(t *testing.T) {
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockMessageCreator{
		responses: []*anthropic.Message{
			makeTextResponse("should not reach", 10, 10),
		},
	}
	a := newTestAgent(mock, nil, 10)

	params := anthropic.MessageNewParams{
		Model:     "test/model",
		MaxTokens: 4096,
		Messages:  []anthropic.MessageParam{},
	}

	_, err := a.runConversationLoop(ctx, params)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
}

func TestConversationLoopSingleTurnNoToolCalls(t *testing.T) {
	text := "Hello, world!"
	mock := &mockMessageCreator{
		responses: []*anthropic.Message{
			makeTextResponse(text, 50, 25),
		},
	}
	a := newTestAgent(mock, nil, 10)

	params := anthropic.MessageNewParams{
		Model:     "test/model",
		MaxTokens: 4096,
		Messages:  []anthropic.MessageParam{},
	}

	result, err := a.runConversationLoop(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Code != text {
		t.Fatalf("Code mismatch: got %q, want %q", result.Code, text)
	}
	if result.InputTokens != 50 {
		t.Fatalf("InputTokens mismatch: got %d, want 50", result.InputTokens)
	}
	if result.OutputTokens != 25 {
		t.Fatalf("OutputTokens mismatch: got %d, want 25", result.OutputTokens)
	}
	if !result.Success {
		t.Fatal("expected Success to be true")
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(mock.calls))
	}
}

func TestConversationLoopMultiTurnMixed(t *testing.T) {
	var toolCallCount int
	tools := []ToolDefinition{
		{
			Name:        "write_file",
			Description: "Write a file",
			InputSchema: json.RawMessage(`{}`),
			Handler: func(input json.RawMessage) (string, error) {
				toolCallCount++
				return "file written", nil
			},
		},
	}

	mock := &mockMessageCreator{
		responses: []*anthropic.Message{
			makeToolUseResponse("write_file", "tool_1", json.RawMessage(`{"path":"main.go"}`), 100, 50),
			makeToolUseResponse("write_file", "tool_2", json.RawMessage(`{"path":"test.go"}`), 80, 40),
			makeTextResponse("All files written successfully", 60, 30),
		},
	}
	a := newTestAgent(mock, tools, 10)

	params := anthropic.MessageNewParams{
		Model:     "test/model",
		MaxTokens: 4096,
		Messages:  []anthropic.MessageParam{},
	}

	result, err := a.runConversationLoop(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if toolCallCount != 2 {
		t.Fatalf("expected 2 tool calls, got %d", toolCallCount)
	}
	if result.Code != "All files written successfully" {
		t.Fatalf("Code mismatch: got %q", result.Code)
	}
	if result.InputTokens != 240 {
		t.Fatalf("InputTokens mismatch: got %d, want 240", result.InputTokens)
	}
	if result.OutputTokens != 120 {
		t.Fatalf("OutputTokens mismatch: got %d, want 120", result.OutputTokens)
	}
	if len(mock.calls) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(mock.calls))
	}
}
