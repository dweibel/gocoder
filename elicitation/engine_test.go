package elicitation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ardp/coding-agent/agent"
	"pgregory.net/rapid"
)

// ============================================================================
// Mock infrastructure
// ============================================================================

// mockMessageCreator implements agent.MessageCreator for testing.
type mockMessageCreator struct {
	responses []*anthropic.Message
	errors    []error // if non-nil at index, return this error instead of response
	callIndex int
	calls     []anthropic.MessageNewParams
}

func (m *mockMessageCreator) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	m.calls = append(m.calls, params)
	idx := m.callIndex
	m.callIndex++

	if idx < len(m.errors) && m.errors[idx] != nil {
		return nil, m.errors[idx]
	}
	if idx >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses at index %d", idx)
	}
	return m.responses[idx], nil
}

// mustMarshalString marshals a string to a JSON string literal.
func mustMarshalString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// makeTextResponse creates a mock anthropic.Message with a single text content block.
func makeTextResponse(text string) *anthropic.Message {
	jsonStr := fmt.Sprintf(
		`{"id":"msg_test","content":[{"type":"text","text":%s}],"usage":{"input_tokens":10,"output_tokens":5},"stop_reason":"end_turn","model":"test-model","role":"assistant","type":"message"}`,
		mustMarshalString(text),
	)
	var msg anthropic.Message
	if err := json.Unmarshal([]byte(jsonStr), &msg); err != nil {
		panic(fmt.Sprintf("makeTextResponse unmarshal failed: %v", err))
	}
	return &msg
}

// mockPersonaLoader implements PersonaLoader for testing.
type mockPersonaLoader struct {
	prompts map[PersonaType]string
}

func (m *mockPersonaLoader) Load(persona PersonaType) (string, error) {
	p, ok := m.prompts[persona]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidPersona, persona)
	}
	return p, nil
}

// newTestPersonaLoader creates a mock PersonaLoader with prompts for all three personas.
func newTestPersonaLoader() *mockPersonaLoader {
	return &mockPersonaLoader{
		prompts: map[PersonaType]string{
			PersonaSocraticBA: "You are a Socratic Business Analyst.",
			PersonaHostileSA:  "You are a Hostile Systems Architect.",
			PersonaTrustedAdv: "You are a Trusted Advisor.",
		},
	}
}

// newTestConfig creates a minimal AgentConfig for testing.
func newTestConfig() agent.AgentConfig {
	return agent.AgentConfig{
		APIKey:    "test-key",
		BaseURL:   "https://test.example.com",
		Model:     "test/model",
		MaxTokens: 4096,
		Timeout:   30 * time.Second,
	}
}

// ============================================================================
// Task 6.1: Unit tests for Engine.Chat with mocked MessageCreator
// Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 2.3, 2.4
// ============================================================================

func TestEngineChat_SingleTurnAppendsMessages(t *testing.T) {
	mock := &mockMessageCreator{
		responses: []*anthropic.Message{makeTextResponse("Hello back!")},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

	session := &Session{
		ID:        "sess-1",
		Persona:   PersonaSocraticBA,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}

	resp, err := engine.Chat(context.Background(), session, "Hello!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Role != "assistant" {
		t.Fatalf("expected role 'assistant', got %q", resp.Role)
	}
	if resp.Content != "Hello back!" {
		t.Fatalf("expected content 'Hello back!', got %q", resp.Content)
	}

	// Session should now have 2 messages: user + assistant
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}
	if session.Messages[0].Role != "user" {
		t.Fatalf("expected first message role 'user', got %q", session.Messages[0].Role)
	}
	if session.Messages[0].Content != "Hello!" {
		t.Fatalf("expected first message content 'Hello!', got %q", session.Messages[0].Content)
	}
	if session.Messages[1].Role != "assistant" {
		t.Fatalf("expected second message role 'assistant', got %q", session.Messages[1].Role)
	}
	if session.Messages[1].Content != "Hello back!" {
		t.Fatalf("expected second message content 'Hello back!', got %q", session.Messages[1].Content)
	}
}

func TestEngineChat_MultiTurnPreservesOrderedHistory(t *testing.T) {
	mock := &mockMessageCreator{
		responses: []*anthropic.Message{
			makeTextResponse("Response 1"),
			makeTextResponse("Response 2"),
			makeTextResponse("Response 3"),
		},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

	session := &Session{
		ID:        "sess-2",
		Persona:   PersonaHostileSA,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}

	messages := []string{"Message 1", "Message 2", "Message 3"}
	for i, msg := range messages {
		_, err := engine.Chat(context.Background(), session, msg)
		if err != nil {
			t.Fatalf("turn %d: unexpected error: %v", i+1, err)
		}
	}

	// Should have 6 messages total (3 user + 3 assistant)
	if len(session.Messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(session.Messages))
	}

	// Verify alternating user/assistant order
	for i, m := range session.Messages {
		expectedRole := "user"
		if i%2 == 1 {
			expectedRole = "assistant"
		}
		if m.Role != expectedRole {
			t.Fatalf("message[%d]: expected role %q, got %q", i, expectedRole, m.Role)
		}
	}

	// Verify content
	if session.Messages[0].Content != "Message 1" {
		t.Fatalf("expected 'Message 1', got %q", session.Messages[0].Content)
	}
	if session.Messages[1].Content != "Response 1" {
		t.Fatalf("expected 'Response 1', got %q", session.Messages[1].Content)
	}
}

func TestEngineChat_PersonaSystemPromptIncluded(t *testing.T) {
	mock := &mockMessageCreator{
		responses: []*anthropic.Message{
			makeTextResponse("resp1"),
			makeTextResponse("resp2"),
		},
	}
	personas := newTestPersonaLoader()
	engine := NewEngine(mock, newTestConfig(), personas)

	session := &Session{
		ID:        "sess-3",
		Persona:   PersonaSocraticBA,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}

	// Two turns
	engine.Chat(context.Background(), session, "turn 1")
	engine.Chat(context.Background(), session, "turn 2")

	expectedPrompt := personas.prompts[PersonaSocraticBA]

	// Both LLM calls should include the persona system prompt
	for i, call := range mock.calls {
		if len(call.System) == 0 {
			t.Fatalf("call %d: expected system prompt, got none", i)
		}
		if call.System[0].Text != expectedPrompt {
			t.Fatalf("call %d: expected system prompt %q, got %q", i, expectedPrompt, call.System[0].Text)
		}
	}
}

func TestEngineChat_LLMFailureReturnsErrorWithoutMutatingSession(t *testing.T) {
	mock := &mockMessageCreator{
		responses: []*anthropic.Message{makeTextResponse("first response")},
		errors:    []error{nil, fmt.Errorf("LLM API error")},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

	session := &Session{
		ID:        "sess-4",
		Persona:   PersonaTrustedAdv,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}

	// First turn succeeds
	_, err := engine.Chat(context.Background(), session, "hello")
	if err != nil {
		t.Fatalf("first turn: unexpected error: %v", err)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages after first turn, got %d", len(session.Messages))
	}

	// Snapshot state before failed call
	msgCountBefore := len(session.Messages)
	msgsBefore := make([]ChatMessage, len(session.Messages))
	copy(msgsBefore, session.Messages)

	// Second turn fails
	_, err = engine.Chat(context.Background(), session, "this will fail")
	if err == nil {
		t.Fatal("expected error from failed LLM call, got nil")
	}

	// Session should be unchanged
	if len(session.Messages) != msgCountBefore {
		t.Fatalf("expected %d messages after failure, got %d", msgCountBefore, len(session.Messages))
	}
	for i, m := range session.Messages {
		if m.Role != msgsBefore[i].Role || m.Content != msgsBefore[i].Content {
			t.Fatalf("message[%d] mutated after failure", i)
		}
	}
}

func TestEngineChat_LLMTimeoutReturnsTimeoutError(t *testing.T) {
	// Create a context that's already past deadline
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond) // ensure deadline passes

	mock := &mockMessageCreator{
		errors: []error{context.DeadlineExceeded},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

	session := &Session{
		ID:        "sess-5",
		Persona:   PersonaSocraticBA,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}

	_, err := engine.Chat(ctx, session, "hello")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Should be identifiable as a timeout error
	if !errors.Is(err, ErrLLMTimeout) {
		t.Fatalf("expected ErrLLMTimeout, got: %v", err)
	}

	// Session should be unchanged
	if len(session.Messages) != 0 {
		t.Fatalf("expected 0 messages after timeout, got %d", len(session.Messages))
	}
}

// ============================================================================
// Task 6.2: Property test — Chat preserves full ordered history
// Feature: elicitation-engine, Property 4: Chat preserves full ordered history
// Validates: Requirements 3.1, 3.2, 3.3
// ============================================================================

func TestChatPreservesOrderedHistory(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "numTurns")

		// Generate N random user messages
		userMessages := make([]string, n)
		for i := 0; i < n; i++ {
			userMessages[i] = rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(t, fmt.Sprintf("userMsg_%d", i))
		}

		// Build N mock responses
		responses := make([]*anthropic.Message, n)
		assistantTexts := make([]string, n)
		for i := 0; i < n; i++ {
			assistantTexts[i] = fmt.Sprintf("response_%d", i)
			responses[i] = makeTextResponse(assistantTexts[i])
		}

		mock := &mockMessageCreator{responses: responses}
		engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		persona := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "personaIdx")]

		session := &Session{
			ID:        "prop-sess",
			Persona:   persona,
			Messages:  []ChatMessage{},
			CreatedAt: time.Now(),
		}

		// Execute all N chat turns
		for i := 0; i < n; i++ {
			_, err := engine.Chat(context.Background(), session, userMessages[i])
			if err != nil {
				t.Fatalf("turn %d: unexpected error: %v", i, err)
			}
		}

		// Property: session contains exactly 2N messages
		if len(session.Messages) != 2*n {
			t.Fatalf("expected %d messages, got %d", 2*n, len(session.Messages))
		}

		// Property: messages alternate user/assistant in correct order
		for i := 0; i < 2*n; i++ {
			expectedRole := "user"
			if i%2 == 1 {
				expectedRole = "assistant"
			}
			if session.Messages[i].Role != expectedRole {
				t.Fatalf("message[%d]: expected role %q, got %q", i, expectedRole, session.Messages[i].Role)
			}
		}

		// Property: user messages match input
		for i := 0; i < n; i++ {
			if session.Messages[2*i].Content != userMessages[i] {
				t.Fatalf("user message[%d]: expected %q, got %q", i, userMessages[i], session.Messages[2*i].Content)
			}
		}

		// Property: assistant messages match responses
		for i := 0; i < n; i++ {
			if session.Messages[2*i+1].Content != assistantTexts[i] {
				t.Fatalf("assistant message[%d]: expected %q, got %q", i, assistantTexts[i], session.Messages[2*i+1].Content)
			}
		}

		// Property: MessageNewParams for Kth turn contains all 2(K-1) prior messages plus the new user message
		if len(mock.calls) != n {
			t.Fatalf("expected %d LLM calls, got %d", n, len(mock.calls))
		}
		for k := 0; k < n; k++ {
			call := mock.calls[k]
			// The call should have 2*(k) + 1 messages: 2*k prior history + 1 new user message
			// Prior history: k user + k assistant = 2k messages
			// Plus the new user message = 2k + 1
			expectedMsgCount := 2*k + 1
			if len(call.Messages) != expectedMsgCount {
				t.Fatalf("call %d: expected %d messages in params, got %d", k, expectedMsgCount, len(call.Messages))
			}
		}
	})
}

// ============================================================================
// Task 6.3: Property test — LLM failure preserves session state
// Feature: elicitation-engine, Property 5: LLM failure preserves session state
// Validates: Requirements 3.4, 6.6
// ============================================================================

func TestLLMFailurePreservesState(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random number of successful turns first (0-10)
		successfulTurns := rapid.IntRange(0, 10).Draw(t, "successfulTurns")

		// Build responses for successful turns
		responses := make([]*anthropic.Message, successfulTurns)
		errs := make([]error, successfulTurns+1)
		for i := 0; i < successfulTurns; i++ {
			responses[i] = makeTextResponse(fmt.Sprintf("response_%d", i))
			errs[i] = nil
		}
		// Inject error at the failure point
		errs[successfulTurns] = fmt.Errorf("injected LLM error")

		mock := &mockMessageCreator{responses: responses, errors: errs}
		engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())

		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		persona := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "personaIdx")]

		session := &Session{
			ID:        "fail-sess",
			Persona:   persona,
			Messages:  []ChatMessage{},
			CreatedAt: time.Now(),
		}

		// Execute successful turns
		for i := 0; i < successfulTurns; i++ {
			_, err := engine.Chat(context.Background(), session, fmt.Sprintf("msg_%d", i))
			if err != nil {
				t.Fatalf("successful turn %d: unexpected error: %v", i, err)
			}
		}

		// Snapshot state before failure
		expectedMsgCount := 2 * successfulTurns
		if len(session.Messages) != expectedMsgCount {
			t.Fatalf("expected %d messages before failure, got %d", expectedMsgCount, len(session.Messages))
		}
		msgsBefore := make([]ChatMessage, len(session.Messages))
		copy(msgsBefore, session.Messages)

		// Execute the failing turn
		_, err := engine.Chat(context.Background(), session, "this will fail")
		if err == nil {
			t.Fatal("expected error from failing LLM call, got nil")
		}

		// Property: session message list remains unchanged
		if len(session.Messages) != expectedMsgCount {
			t.Fatalf("expected %d messages after failure, got %d", expectedMsgCount, len(session.Messages))
		}
		for i, m := range session.Messages {
			if m.Role != msgsBefore[i].Role || m.Content != msgsBefore[i].Content {
				t.Fatalf("message[%d] mutated after failure: was {%s, %s}, now {%s, %s}",
					i, msgsBefore[i].Role, msgsBefore[i].Content, m.Role, m.Content)
			}
		}
	})
}

// ============================================================================
// Task 6.4: Property test — Persona prompt consistency across turns
// Feature: elicitation-engine, Property 3: Persona prompt consistency across turns
// Validates: Requirements 2.3, 2.4, 10.4
// ============================================================================

func TestPersonaPromptConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		persona := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "personaIdx")]

		n := rapid.IntRange(1, 15).Draw(t, "numTurns")

		// Build N mock responses
		responses := make([]*anthropic.Message, n)
		for i := 0; i < n; i++ {
			responses[i] = makeTextResponse(fmt.Sprintf("response_%d", i))
		}

		mock := &mockMessageCreator{responses: responses}
		loader := newTestPersonaLoader()
		engine := NewEngine(mock, newTestConfig(), loader)

		expectedPrompt := loader.prompts[persona]

		session := &Session{
			ID:        "persona-sess",
			Persona:   persona,
			Messages:  []ChatMessage{},
			CreatedAt: time.Now(),
		}

		// Execute N chat turns
		for i := 0; i < n; i++ {
			msg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, fmt.Sprintf("msg_%d", i))
			_, err := engine.Chat(context.Background(), session, msg)
			if err != nil {
				t.Fatalf("turn %d: unexpected error: %v", i, err)
			}
		}

		// Property: every LLM call includes the selected persona's prompt as system prompt
		if len(mock.calls) != n {
			t.Fatalf("expected %d LLM calls, got %d", n, len(mock.calls))
		}
		for i, call := range mock.calls {
			if len(call.System) == 0 {
				t.Fatalf("call %d: no system prompt set", i)
			}
			if call.System[0].Text != expectedPrompt {
				t.Fatalf("call %d: expected system prompt %q, got %q", i, expectedPrompt, call.System[0].Text)
			}
		}
	})
}

// ============================================================================
// Task 7.5: Unit tests for Engine.Synthesize with mocked MessageCreator
// Requirements: 5.3, 6.1, 6.2, 6.3, 6.4, 6.5, 6.6
// ============================================================================

// setupTestPromptsDir creates a temp directory with the synthesis template for testing.
func setupTestPromptsDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tmpl := `You are a requirements synthesis engine.

## Conversation History

{{range .Messages}}### {{.Role}}
{{.Content}}

{{end}}

Produce structured artifacts using the delimited format.`
	if err := os.WriteFile(filepath.Join(dir, "synthesis.tmpl"), []byte(tmpl), 0644); err != nil {
		t.Fatalf("failed to write test synthesis template: %v", err)
	}
	return dir
}

func TestEngineSynthesize_BuildsPromptWithChatHistory(t *testing.T) {
	synthesisOutput := `===BRD===
Business requirements from conversation.
===END_BRD===

===SRS===
Software requirements from conversation.
===END_SRS===

===NFR===
Non-functional requirements from conversation.
===END_NFR===

===GHERKIN===
### Story: User Feature
Given a user
When they use the feature
Then it works correctly
===END_GHERKIN===`

	mock := &mockMessageCreator{
		responses: []*anthropic.Message{makeTextResponse(synthesisOutput)},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())
	engine.SetPromptsDir(setupTestPromptsDir(t))

	session := &Session{
		ID:      "synth-1",
		Persona: PersonaSocraticBA,
		Messages: []ChatMessage{
			{Role: "user", Content: "I want to build a login feature", SentAt: time.Now()},
			{Role: "assistant", Content: "Tell me about the users", SentAt: time.Now()},
			{Role: "user", Content: "Internal employees and external clients", SentAt: time.Now()},
			{Role: "assistant", Content: "What authentication methods?", SentAt: time.Now()},
		},
		CreatedAt: time.Now(),
	}

	artifacts, err := engine.Synthesize(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have called the LLM exactly once
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 LLM call, got %d", len(mock.calls))
	}

	// The prompt should contain the chat history
	call := mock.calls[0]
	if len(call.Messages) == 0 {
		t.Fatal("expected at least one message in LLM call")
	}

	// Marshal the call to JSON to inspect the prompt content
	callJSON, err := json.Marshal(call.Messages[0])
	if err != nil {
		t.Fatalf("failed to marshal call: %v", err)
	}
	promptText := string(callJSON)
	if !strings.Contains(promptText, "I want to build a login feature") {
		t.Error("synthesis prompt should contain chat history messages")
	}
	if !strings.Contains(promptText, "Tell me about the users") {
		t.Error("synthesis prompt should contain assistant messages from history")
	}

	// Should have produced artifacts
	if len(artifacts) != 4 {
		t.Fatalf("expected 4 artifacts (BRD, SRS, NFR, 1 Gherkin), got %d", len(artifacts))
	}
}

func TestEngineSynthesize_ParsesLLMResponseIntoTypedArtifacts(t *testing.T) {
	synthesisOutput := `===BRD===
BRD content here.
===END_BRD===

===SRS===
SRS content here.
===END_SRS===

===NFR===
NFR content here.
===END_NFR===

===GHERKIN===
### Story: First Story
Given precondition
When action
Then result

### Story: Second Story
Given another precondition
When another action
Then another result
===END_GHERKIN===`

	mock := &mockMessageCreator{
		responses: []*anthropic.Message{makeTextResponse(synthesisOutput)},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())
	engine.SetPromptsDir(setupTestPromptsDir(t))

	session := &Session{
		ID:      "synth-2",
		Persona: PersonaHostileSA,
		Messages: []ChatMessage{
			{Role: "user", Content: "Build a feature", SentAt: time.Now()},
			{Role: "assistant", Content: "Describe it", SentAt: time.Now()},
		},
		CreatedAt: time.Now(),
	}

	artifacts, err := engine.Synthesize(context.Background(), session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BRD + SRS + NFR + 2 Gherkin = 5
	if len(artifacts) != 5 {
		t.Fatalf("expected 5 artifacts, got %d", len(artifacts))
	}

	// Verify types
	expectedTypes := []string{"BRD", "SRS", "NFR", "GHERKIN", "GHERKIN"}
	for i, a := range artifacts {
		if a.Type != expectedTypes[i] {
			t.Errorf("artifact[%d]: expected type %q, got %q", i, expectedTypes[i], a.Type)
		}
	}

	// Verify Gherkin titles
	if artifacts[3].Title != "First Story" {
		t.Errorf("expected title 'First Story', got %q", artifacts[3].Title)
	}
	if artifacts[4].Title != "Second Story" {
		t.Errorf("expected title 'Second Story', got %q", artifacts[4].Title)
	}
}

func TestEngineSynthesize_LLMFailurePreservesSessionState(t *testing.T) {
	mock := &mockMessageCreator{
		errors: []error{fmt.Errorf("LLM synthesis failure")},
	}
	engine := NewEngine(mock, newTestConfig(), newTestPersonaLoader())
	engine.SetPromptsDir(setupTestPromptsDir(t))

	originalMessages := []ChatMessage{
		{Role: "user", Content: "Build a feature", SentAt: time.Now()},
		{Role: "assistant", Content: "Tell me more", SentAt: time.Now()},
		{Role: "user", Content: "It should do X", SentAt: time.Now()},
		{Role: "assistant", Content: "What about Y?", SentAt: time.Now()},
	}

	session := &Session{
		ID:        "synth-3",
		Persona:   PersonaTrustedAdv,
		Messages:  make([]ChatMessage, len(originalMessages)),
		CreatedAt: time.Now(),
	}
	copy(session.Messages, originalMessages)

	_, err := engine.Synthesize(context.Background(), session)
	if err == nil {
		t.Fatal("expected error from failed LLM call, got nil")
	}

	// Session state should be unchanged
	if len(session.Messages) != len(originalMessages) {
		t.Fatalf("expected %d messages after failure, got %d", len(originalMessages), len(session.Messages))
	}
	for i, m := range session.Messages {
		if m.Role != originalMessages[i].Role || m.Content != originalMessages[i].Content {
			t.Errorf("message[%d] mutated after synthesis failure", i)
		}
	}
}

// ============================================================================
// Task 9.2: Property test — Persona switch uses new system prompt
// Feature: chat-ui, Property 5: Persona switch uses new system prompt
// Validates: Requirements 5.5
// ============================================================================

func TestPersonaSwitchUsesNewPrompt(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}

		// Pick an initial persona and a different target persona
		initialIdx := rapid.IntRange(0, len(personas)-1).Draw(t, "initialPersonaIdx")
		initialPersona := personas[initialIdx]

		// Pick a different persona for the switch
		targetIdx := rapid.IntRange(0, len(personas)-2).Draw(t, "targetPersonaIdx")
		if targetIdx >= initialIdx {
			targetIdx++
		}
		targetPersona := personas[targetIdx]

		// Generate some pre-switch turns (0-10)
		preSwitchTurns := rapid.IntRange(0, 10).Draw(t, "preSwitchTurns")

		// Build mock responses: preSwitchTurns + 1 post-switch turn
		totalTurns := preSwitchTurns + 1
		responses := make([]*anthropic.Message, totalTurns)
		for i := 0; i < totalTurns; i++ {
			responses[i] = makeTextResponse(fmt.Sprintf("response_%d", i))
		}

		mock := &mockMessageCreator{responses: responses}
		loader := newTestPersonaLoader()
		engine := NewEngine(mock, newTestConfig(), loader)

		session := &Session{
			ID:        "switch-sess",
			Persona:   initialPersona,
			Messages:  []ChatMessage{},
			CreatedAt: time.Now(),
		}

		// Execute pre-switch turns with initial persona
		for i := 0; i < preSwitchTurns; i++ {
			msg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, fmt.Sprintf("preMsg_%d", i))
			_, err := engine.Chat(context.Background(), session, msg)
			if err != nil {
				t.Fatalf("pre-switch turn %d: unexpected error: %v", i, err)
			}
		}

		// Switch persona (simulating what HandleUpdatePersona does)
		session.Persona = targetPersona

		// Execute one post-switch turn
		postMsg := rapid.StringMatching(`[a-zA-Z0-9 ]{1,30}`).Draw(t, "postMsg")
		_, err := engine.Chat(context.Background(), session, postMsg)
		if err != nil {
			t.Fatalf("post-switch turn: unexpected error: %v", err)
		}

		// Property: the last LLM call (post-switch) must use the target persona's system prompt
		expectedPrompt := loader.prompts[targetPersona]
		lastCall := mock.calls[len(mock.calls)-1]

		if len(lastCall.System) == 0 {
			t.Fatal("post-switch call: no system prompt set")
		}
		if lastCall.System[0].Text != expectedPrompt {
			t.Fatalf("post-switch call: expected system prompt %q (for %s), got %q",
				expectedPrompt, targetPersona, lastCall.System[0].Text)
		}

		// Also verify pre-switch calls used the initial persona's prompt
		initialPrompt := loader.prompts[initialPersona]
		for i := 0; i < preSwitchTurns; i++ {
			call := mock.calls[i]
			if len(call.System) == 0 {
				t.Fatalf("pre-switch call %d: no system prompt set", i)
			}
			if call.System[0].Text != initialPrompt {
				t.Fatalf("pre-switch call %d: expected system prompt %q (for %s), got %q",
					i, initialPrompt, initialPersona, call.System[0].Text)
			}
		}
	})
}
