package elicitation

import (
	"errors"
	"testing"

	"pgregory.net/rapid"
)

// ============================================================================
// Task 3.1: Unit tests for SessionStore (Create, Get, List, Delete)
// Requirements: 13.1, 13.2, 13.3, 13.4, 13.5
// ============================================================================

func TestSessionStore_CreateReturnsUniqueIDAndCorrectPersona(t *testing.T) {
	store := NewInMemorySessionStore()

	personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
	for _, p := range personas {
		s := store.Create(p)
		if s == nil {
			t.Fatal("Create returned nil session")
		}
		if s.ID == "" {
			t.Fatal("Create returned session with empty ID")
		}
		if s.Persona != p {
			t.Fatalf("expected persona %q, got %q", p, s.Persona)
		}
		if len(s.Messages) != 0 {
			t.Fatalf("expected empty messages, got %d", len(s.Messages))
		}
		if s.CreatedAt.IsZero() {
			t.Fatal("expected non-zero CreatedAt")
		}
	}
}

func TestSessionStore_CreateIDsAreUnique(t *testing.T) {
	store := NewInMemorySessionStore()
	ids := make(map[string]bool)
	for i := 0; i < 50; i++ {
		s := store.Create(PersonaSocraticBA)
		if ids[s.ID] {
			t.Fatalf("duplicate session ID: %s", s.ID)
		}
		ids[s.ID] = true
	}
}

func TestSessionStore_GetReturnsStoredSession(t *testing.T) {
	store := NewInMemorySessionStore()
	created := store.Create(PersonaHostileSA)

	got, err := store.Get(created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected ID %q, got %q", created.ID, got.ID)
	}
	if got.Persona != created.Persona {
		t.Fatalf("expected persona %q, got %q", created.Persona, got.Persona)
	}
}

func TestSessionStore_GetUnknownIDReturnsErrSessionNotFound(t *testing.T) {
	store := NewInMemorySessionStore()

	_, err := store.Get("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for unknown ID, got nil")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got: %v", err)
	}
}

func TestSessionStore_ListReturnsAllSessions(t *testing.T) {
	store := NewInMemorySessionStore()

	s1 := store.Create(PersonaSocraticBA)
	s2 := store.Create(PersonaHostileSA)
	s3 := store.Create(PersonaTrustedAdv)

	summaries := store.List()
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}

	ids := make(map[string]bool)
	for _, s := range summaries {
		ids[s.ID] = true
	}
	for _, id := range []string{s1.ID, s2.ID, s3.ID} {
		if !ids[id] {
			t.Fatalf("session %q not found in list", id)
		}
	}
}

func TestSessionStore_ListReturnsCorrectMessageCount(t *testing.T) {
	store := NewInMemorySessionStore()
	s := store.Create(PersonaSocraticBA)

	// Manually append messages to the session to test message count
	sess, _ := store.Get(s.ID)
	sess.Messages = append(sess.Messages,
		ChatMessage{Role: "user", Content: "hello"},
		ChatMessage{Role: "assistant", Content: "hi"},
	)

	summaries := store.List()
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].MessageCount != 2 {
		t.Fatalf("expected message count 2, got %d", summaries[0].MessageCount)
	}
}

func TestSessionStore_DeleteRemovesSession(t *testing.T) {
	store := NewInMemorySessionStore()
	s := store.Create(PersonaSocraticBA)

	err := store.Delete(s.ID)
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}

	_, err = store.Get(s.ID)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound after delete, got: %v", err)
	}
}

func TestSessionStore_DeleteNonexistentReturnsError(t *testing.T) {
	store := NewInMemorySessionStore()

	err := store.Delete("nonexistent-id")
	if err == nil {
		t.Fatal("expected error deleting nonexistent session, got nil")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got: %v", err)
	}
}

// ============================================================================
// Task 3.2: Property test — Session ID uniqueness
// Feature: elicitation-engine, Property 11: Session ID uniqueness
// Validates: Requirements 13.1
// ============================================================================

func TestSessionIDUniqueness(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 100).Draw(t, "batchSize")
		store := NewInMemorySessionStore()

		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		ids := make(map[string]bool, n)

		for i := 0; i < n; i++ {
			p := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "personaIdx")]
			s := store.Create(p)
			if ids[s.ID] {
				t.Fatalf("duplicate session ID %q after creating %d sessions", s.ID, i+1)
			}
			ids[s.ID] = true
		}

		if len(ids) != n {
			t.Fatalf("expected %d unique IDs, got %d", n, len(ids))
		}
	})
}

// ============================================================================
// Task 3.3: Property test — Session store round-trip
// Feature: elicitation-engine, Property 12: Session store round-trip
// Validates: Requirements 13.3, 13.4
// ============================================================================

func TestSessionStoreRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		p := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "personaIdx")]

		store := NewInMemorySessionStore()
		created := store.Create(p)

		retrieved, err := store.Get(created.ID)
		if err != nil {
			t.Fatalf("Get returned error: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Fatalf("ID mismatch: created %q, retrieved %q", created.ID, retrieved.ID)
		}
		if retrieved.Persona != p {
			t.Fatalf("Persona mismatch: expected %q, got %q", p, retrieved.Persona)
		}
		if len(retrieved.Messages) != 0 {
			t.Fatalf("expected empty messages, got %d", len(retrieved.Messages))
		}
	})
}

// ============================================================================
// Task 3.4: Property test — Nonexistent session returns error
// Feature: elicitation-engine, Property 13: Nonexistent session returns error
// Validates: Requirements 13.5
// ============================================================================

func TestNonexistentSessionError(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random UUID-like string
		fakeID := rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).Draw(t, "fakeID")

		store := NewInMemorySessionStore()

		// Create a few sessions to ensure the store isn't empty
		numSessions := rapid.IntRange(0, 10).Draw(t, "numSessions")
		personas := []PersonaType{PersonaSocraticBA, PersonaHostileSA, PersonaTrustedAdv}
		activeIDs := make(map[string]bool)
		for i := 0; i < numSessions; i++ {
			p := personas[rapid.IntRange(0, len(personas)-1).Draw(t, "pIdx")]
			s := store.Create(p)
			activeIDs[s.ID] = true
		}

		// Skip if the random ID happens to collide with an active session
		if activeIDs[fakeID] {
			return
		}

		_, err := store.Get(fakeID)
		if err == nil {
			t.Fatalf("expected error for nonexistent ID %q, got nil", fakeID)
		}
		if !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("expected ErrSessionNotFound, got: %v", err)
		}
	})
}
