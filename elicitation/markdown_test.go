package elicitation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// ============================================================================
// Task 8.1: Unit tests for ConversationMarshaler (Marshal and Unmarshal)
// Requirements: 9.1, 9.2, 9.3, 9.4, 9.5
// ============================================================================

func TestMarshal_ProducesValidMarkdownWithYAMLFrontMatter(t *testing.T) {
	session := &Session{
		ID:        "test-session-123",
		Persona:   PersonaSocraticBA,
		CreatedAt: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Messages: []ChatMessage{
			{Role: "user", Content: "I want to build a PDF export feature.", SentAt: time.Date(2025, 1, 15, 10, 30, 5, 0, time.UTC)},
			{Role: "assistant", Content: "Tell me more about the audience.", SentAt: time.Date(2025, 1, 15, 10, 30, 12, 0, time.UTC)},
		},
	}

	codec := NewMarkdownCodec()
	data, err := codec.Marshal(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	md := string(data)

	// Must start with YAML front-matter delimiters
	if !strings.HasPrefix(md, "---\n") {
		t.Error("markdown should start with ---")
	}

	// Must contain front-matter fields (YAML may or may not quote strings)
	if !strings.Contains(md, "session_id:") || !strings.Contains(md, "test-session-123") {
		t.Error("missing session_id in front-matter")
	}
	if !strings.Contains(md, "persona:") || !strings.Contains(md, "socratic_business_analyst") {
		t.Error("missing persona in front-matter")
	}
	if !strings.Contains(md, "created_at:") || !strings.Contains(md, "2025-01-15T10:30:00Z") {
		t.Error("missing created_at in front-matter")
	}
	if !strings.Contains(md, "message_count: 2") {
		t.Error("missing message_count in front-matter")
	}

	// Must contain message headers
	if !strings.Contains(md, "## user — 2025-01-15T10:30:05Z") {
		t.Error("missing user message header")
	}
	if !strings.Contains(md, "## Socratic Business Analyst — 2025-01-15T10:30:12Z") {
		t.Error("missing assistant message header (expected persona display name)")
	}

	// Must contain message content
	if !strings.Contains(md, "I want to build a PDF export feature.") {
		t.Error("missing user message content")
	}
	if !strings.Contains(md, "Tell me more about the audience.") {
		t.Error("missing assistant message content")
	}
}

func TestUnmarshal_RestoresSessionCorrectly(t *testing.T) {
	md := `---
session_id: "abc123"
persona: "hostile_systems_architect"
created_at: "2025-01-15T10:30:00Z"
message_count: 2
---

## user — 2025-01-15T10:30:05Z

Hello, I have a feature idea.

## assistant — 2025-01-15T10:30:12Z

Let me challenge that idea.
`

	codec := NewMarkdownCodec()
	session, err := codec.Unmarshal([]byte(md))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.ID != "abc123" {
		t.Errorf("expected ID %q, got %q", "abc123", session.ID)
	}
	if session.Persona != PersonaHostileSA {
		t.Errorf("expected persona %q, got %q", PersonaHostileSA, session.Persona)
	}
	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}

	// Check message order and content
	if session.Messages[0].Role != "user" {
		t.Errorf("expected first message role %q, got %q", "user", session.Messages[0].Role)
	}
	if session.Messages[0].Content != "Hello, I have a feature idea." {
		t.Errorf("unexpected first message content: %q", session.Messages[0].Content)
	}
	if session.Messages[1].Role != "assistant" {
		t.Errorf("expected second message role %q, got %q", "assistant", session.Messages[1].Role)
	}
	if session.Messages[1].Content != "Let me challenge that idea." {
		t.Errorf("unexpected second message content: %q", session.Messages[1].Content)
	}

	// Check timestamps
	expectedTime := time.Date(2025, 1, 15, 10, 30, 5, 0, time.UTC)
	if !session.Messages[0].SentAt.Equal(expectedTime) {
		t.Errorf("expected first message SentAt %v, got %v", expectedTime, session.Messages[0].SentAt)
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	original := &Session{
		ID:        "roundtrip-id",
		Persona:   PersonaTrustedAdv,
		CreatedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Messages: []ChatMessage{
			{Role: "user", Content: "First message.", SentAt: time.Date(2025, 6, 1, 12, 0, 1, 0, time.UTC)},
			{Role: "assistant", Content: "First response.", SentAt: time.Date(2025, 6, 1, 12, 0, 2, 0, time.UTC)},
			{Role: "user", Content: "Second message.", SentAt: time.Date(2025, 6, 1, 12, 0, 3, 0, time.UTC)},
			{Role: "assistant", Content: "Second response.", SentAt: time.Date(2025, 6, 1, 12, 0, 4, 0, time.UTC)},
		},
	}

	codec := NewMarkdownCodec()
	data, err := codec.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	restored, err := codec.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", original.ID, restored.ID)
	}
	if restored.Persona != original.Persona {
		t.Errorf("Persona mismatch: %q vs %q", original.Persona, restored.Persona)
	}
	if len(restored.Messages) != len(original.Messages) {
		t.Fatalf("message count mismatch: %d vs %d", len(original.Messages), len(restored.Messages))
	}
	for i := range original.Messages {
		if restored.Messages[i].Role != original.Messages[i].Role {
			t.Errorf("message[%d] role mismatch: %q vs %q", i, original.Messages[i].Role, restored.Messages[i].Role)
		}
		if restored.Messages[i].Content != original.Messages[i].Content {
			t.Errorf("message[%d] content mismatch: %q vs %q", i, original.Messages[i].Content, restored.Messages[i].Content)
		}
		if !restored.Messages[i].SentAt.Truncate(time.Second).Equal(original.Messages[i].SentAt.Truncate(time.Second)) {
			t.Errorf("message[%d] SentAt mismatch: %v vs %v", i, original.Messages[i].SentAt, restored.Messages[i].SentAt)
		}
	}
}

func TestMarshalUnmarshal_EmptyMessages(t *testing.T) {
	original := &Session{
		ID:        "empty-session",
		Persona:   PersonaSocraticBA,
		CreatedAt: time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC),
		Messages:  []ChatMessage{},
	}

	codec := NewMarkdownCodec()
	data, err := codec.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	restored, err := codec.Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if restored.ID != original.ID {
		t.Errorf("ID mismatch: %q vs %q", original.ID, restored.ID)
	}
	if restored.Persona != original.Persona {
		t.Errorf("Persona mismatch: %q vs %q", original.Persona, restored.Persona)
	}
	if len(restored.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(restored.Messages))
	}
}

func TestUnmarshal_MalformedMarkdown(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "empty input",
			data: "",
		},
		{
			name: "no front-matter delimiters",
			data: "just some random text without any structure",
		},
		{
			name: "missing closing front-matter delimiter",
			data: "---\nsession_id: \"abc\"\npersona: \"socratic_business_analyst\"\n",
		},
		{
			name: "invalid YAML in front-matter",
			data: "---\n: : : invalid yaml\n---\n",
		},
		{
			name: "missing session_id",
			data: "---\npersona: \"socratic_business_analyst\"\ncreated_at: \"2025-01-15T10:30:00Z\"\nmessage_count: 0\n---\n",
		},
		{
			name: "missing persona",
			data: "---\nsession_id: \"abc\"\ncreated_at: \"2025-01-15T10:30:00Z\"\nmessage_count: 0\n---\n",
		},
		{
			name: "bad message header format",
			data: "---\nsession_id: \"abc\"\npersona: \"socratic_business_analyst\"\ncreated_at: \"2025-01-15T10:30:00Z\"\nmessage_count: 1\n---\n\n## badheader\n\nSome content\n",
		},
	}

	codec := NewMarkdownCodec()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := codec.Unmarshal([]byte(tc.data))
			if err == nil {
				t.Fatal("expected error for malformed markdown, got nil")
			}
			// Should not panic — reaching here means no panic occurred
		})
	}
}

// ============================================================================
// Task 8.2: Property test — Conversation markdown round-trip
// Feature: elicitation-engine, Property 8: Conversation markdown round-trip
// Validates: Requirements 9.1, 9.2, 9.3, 9.4
// ============================================================================

func TestConversationMarkdownRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		persona := personas[rapid.IntRange(0, 2).Draw(t, "personaIdx")]

		sessionID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "sessionID")

		// Generate a base time truncated to seconds (format uses second precision)
		baseUnix := rapid.Int64Range(1700000000, 1800000000).Draw(t, "baseUnix")
		createdAt := time.Unix(baseUnix, 0).UTC()

		// Generate 0-20 messages
		numMessages := rapid.IntRange(0, 20).Draw(t, "numMessages")
		messages := make([]ChatMessage, numMessages)
		for i := 0; i < numMessages; i++ {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			// Safe content: no "## " at line start, no "---" at line start
			content := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 .,;:!?]{0,99}`).Draw(t, fmt.Sprintf("content_%d", i))
			sentAt := createdAt.Add(time.Duration(i+1) * time.Second)
			messages[i] = ChatMessage{
				Role:    role,
				Content: content,
				SentAt:  sentAt,
			}
		}

		original := &Session{
			ID:        sessionID,
			Persona:   persona,
			CreatedAt: createdAt,
			Messages:  messages,
		}

		codec := NewMarkdownCodec()

		// Marshal
		data, err := codec.Marshal(original)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		// Unmarshal
		restored, err := codec.Unmarshal(data)
		if err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		// Property: persona matches
		if restored.Persona != original.Persona {
			t.Errorf("persona mismatch: %q vs %q", original.Persona, restored.Persona)
		}

		// Property: session ID matches
		if restored.ID != original.ID {
			t.Errorf("ID mismatch: %q vs %q", original.ID, restored.ID)
		}

		// Property: message count matches
		if len(restored.Messages) != len(original.Messages) {
			t.Fatalf("message count mismatch: %d vs %d", len(original.Messages), len(restored.Messages))
		}

		// Property: each message role and content matches
		for i := range original.Messages {
			if restored.Messages[i].Role != original.Messages[i].Role {
				t.Errorf("message[%d] role mismatch: %q vs %q", i, original.Messages[i].Role, restored.Messages[i].Role)
			}
			if restored.Messages[i].Content != original.Messages[i].Content {
				t.Errorf("message[%d] content mismatch: %q vs %q", i, original.Messages[i].Content, restored.Messages[i].Content)
			}
			// Compare timestamps truncated to seconds
			origT := original.Messages[i].SentAt.Truncate(time.Second)
			restT := restored.Messages[i].SentAt.Truncate(time.Second)
			if !restT.Equal(origT) {
				t.Errorf("message[%d] SentAt mismatch: %v vs %v", i, origT, restT)
			}
		}

		// Property: CreatedAt matches (truncated to seconds)
		origCreated := original.CreatedAt.Truncate(time.Second)
		restCreated := restored.CreatedAt.Truncate(time.Second)
		if !restCreated.Equal(origCreated) {
			t.Errorf("CreatedAt mismatch: %v vs %v", origCreated, restCreated)
		}
	})
}

// ============================================================================
// Task 8.3: Property test — Malformed markdown returns parse error
// Feature: elicitation-engine, Property 9: Malformed markdown returns parse error
// Validates: Requirements 9.5
// ============================================================================

func TestMalformedMarkdownError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Choose a corruption strategy
		strategy := rapid.IntRange(0, 3).Draw(t, "strategy")

		var input []byte
		switch strategy {
		case 0:
			// Random byte sequences
			length := rapid.IntRange(0, 500).Draw(t, "length")
			input = make([]byte, length)
			for i := range input {
				input[i] = byte(rapid.IntRange(0, 255).Draw(t, fmt.Sprintf("byte_%d", i)))
			}
		case 1:
			// Missing front-matter: valid-looking content but no --- delimiters
			content := rapid.StringMatching(`[a-zA-Z0-9 .,;:!?\n]{10,200}`).Draw(t, "content")
			input = []byte(content)
		case 2:
			// Bad front-matter: has opening --- but no closing ---
			yaml := rapid.StringMatching(`[a-zA-Z0-9: ]{5,50}`).Draw(t, "yaml")
			input = []byte("---\n" + yaml + "\n")
		case 3:
			// Truncated: valid front-matter but bad message headers
			badHeader := rapid.StringMatching(`[a-zA-Z0-9 ]{5,30}`).Draw(t, "badHeader")
			input = []byte(fmt.Sprintf("---\nsession_id: \"x\"\npersona: \"socratic_business_analyst\"\ncreated_at: \"2025-01-15T10:30:00Z\"\nmessage_count: 1\n---\n\n## %s\n\nSome content\n", badHeader))
		}

		codec := NewMarkdownCodec()

		// Property: Unmarshal returns non-nil error without panicking
		_, err := codec.Unmarshal(input)
		if err == nil {
			// For strategy 0 (random bytes), it's extremely unlikely to produce valid markdown
			// For strategies 1-3, it should always error
			if strategy != 0 {
				t.Fatalf("expected error for malformed markdown (strategy %d), got nil", strategy)
			}
			// For random bytes, if it somehow parses, that's acceptable but very unlikely
		}
	})
}

// ============================================================================
// Task 3.3: Property test — Markdown round-trip preserves name and persona names
// Feature: chat-ui, Property 6: Conversation markdown round-trip preserves name and persona names
// Validates: Requirements 6.2, 6.3, 6.4
// ============================================================================

func TestMarkdownRoundTripWithNameAndPersonaName(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		persona := personas[rapid.IntRange(0, 2).Draw(t, "personaIdx")]

		sessionID := rapid.StringMatching(`[a-z0-9]{8,32}`).Draw(t, "sessionID")

		// Generate a valid session name (1-200 chars, safe characters)
		name := rapid.StringMatching(`[a-zA-Z][a-zA-Z0-9 ]{0,99}`).Draw(t, "sessionName")

		// Generate a base time truncated to seconds
		baseUnix := rapid.Int64Range(1700000000, 1800000000).Draw(t, "baseUnix")
		createdAt := time.Unix(baseUnix, 0).UTC()

		// Generate 0-20 messages with alternating user/assistant roles and system messages
		numMessages := rapid.IntRange(0, 20).Draw(t, "numMessages")
		messages := make([]ChatMessage, numMessages)
		for i := 0; i < numMessages; i++ {
			roleChoice := rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("roleChoice_%d", i))
			var role, personaName string
			switch roleChoice {
			case 0:
				role = "user"
			case 1:
				role = "assistant"
				// Generate a persona display name for assistant messages
				personaName = PersonaDisplayNames[personas[rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("msgPersonaIdx_%d", i))]]
			case 2:
				role = "system"
			}

			content := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9 .,;:!?]{0,99}`).Draw(t, fmt.Sprintf("content_%d", i))
			sentAt := createdAt.Add(time.Duration(i+1) * time.Second)
			messages[i] = ChatMessage{
				Role:        role,
				PersonaName: personaName,
				Content:     content,
				SentAt:      sentAt,
			}
		}

		original := &Session{
			ID:        sessionID,
			Name:      name,
			Persona:   persona,
			CreatedAt: createdAt,
			Messages:  messages,
		}

		codec := NewMarkdownCodec()

		// Marshal
		data, err := codec.Marshal(original)
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		// Unmarshal
		restored, err := codec.Unmarshal(data)
		if err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		// Property: name matches
		if restored.Name != original.Name {
			t.Errorf("name mismatch: %q vs %q", original.Name, restored.Name)
		}

		// Property: persona matches
		if restored.Persona != original.Persona {
			t.Errorf("persona mismatch: %q vs %q", original.Persona, restored.Persona)
		}

		// Property: session ID matches
		if restored.ID != original.ID {
			t.Errorf("ID mismatch: %q vs %q", original.ID, restored.ID)
		}

		// Property: message count matches
		if len(restored.Messages) != len(original.Messages) {
			t.Fatalf("message count mismatch: %d vs %d", len(original.Messages), len(restored.Messages))
		}

		// Property: each message role, content, and PersonaName matches
		for i := range original.Messages {
			if restored.Messages[i].Role != original.Messages[i].Role {
				t.Errorf("message[%d] role mismatch: %q vs %q", i, original.Messages[i].Role, restored.Messages[i].Role)
			}
			if restored.Messages[i].Content != original.Messages[i].Content {
				t.Errorf("message[%d] content mismatch: %q vs %q", i, original.Messages[i].Content, restored.Messages[i].Content)
			}
			// Check PersonaName on assistant messages
			if original.Messages[i].Role == "assistant" {
				expectedPersonaName := original.Messages[i].PersonaName
				if expectedPersonaName == "" {
					// Fallback: marshal uses PersonaDisplayNames
					expectedPersonaName = PersonaDisplayNames[original.Persona]
				}
				if restored.Messages[i].PersonaName != expectedPersonaName {
					t.Errorf("message[%d] PersonaName mismatch: %q vs %q", i, expectedPersonaName, restored.Messages[i].PersonaName)
				}
			}
			// Compare timestamps truncated to seconds
			origT := original.Messages[i].SentAt.Truncate(time.Second)
			restT := restored.Messages[i].SentAt.Truncate(time.Second)
			if !restT.Equal(origT) {
				t.Errorf("message[%d] SentAt mismatch: %v vs %v", i, origT, restT)
			}
		}

		// Property: CreatedAt matches
		origCreated := original.CreatedAt.Truncate(time.Second)
		restCreated := restored.CreatedAt.Truncate(time.Second)
		if !restCreated.Equal(origCreated) {
			t.Errorf("CreatedAt mismatch: %v vs %v", origCreated, restCreated)
		}
	})
}

// ============================================================================
// Task 3.4: Unit test — Backward compatibility: no name in front-matter
// Requirements: 6.5
// ============================================================================

func TestMarkdownBackwardCompatNoName(t *testing.T) {
	md := `---
session_id: "legacy-session"
persona: "trusted_advisor"
created_at: "2025-01-15T10:30:00Z"
message_count: 1
---

## user — 2025-01-15T10:30:05Z

Hello there.
`

	codec := NewMarkdownCodec()
	session, err := codec.Unmarshal([]byte(md))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if session.Name != "Untitled Session" {
		t.Errorf("expected Name %q, got %q", "Untitled Session", session.Name)
	}
}

// ============================================================================
// Task 3.5: Unit test — Backward compatibility: assistant role headers
// Requirements: 6.5
// ============================================================================

func TestMarkdownBackwardCompatAssistantRole(t *testing.T) {
	md := `---
session_id: "legacy-session"
persona: "socratic_business_analyst"
created_at: "2025-01-15T10:30:00Z"
message_count: 2
---

## user — 2025-01-15T10:30:05Z

I have a question.

## assistant — 2025-01-15T10:30:12Z

Let me help you with that.
`

	codec := NewMarkdownCodec()
	session, err := codec.Unmarshal([]byte(md))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(session.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(session.Messages))
	}

	assistantMsg := session.Messages[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("expected Role %q, got %q", "assistant", assistantMsg.Role)
	}
	if assistantMsg.PersonaName != "assistant" {
		t.Errorf("expected PersonaName %q, got %q", "assistant", assistantMsg.PersonaName)
	}
}
