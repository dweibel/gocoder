package elicitation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PersonaType identifies which persona to use for an elicitation session.
type PersonaType string

const (
	PersonaSocraticBA PersonaType = "socratic_business_analyst"
	PersonaHostileSA  PersonaType = "hostile_systems_architect"
	PersonaTrustedAdv PersonaType = "trusted_advisor"
)

// ChatMessage represents a single turn in an elicitation conversation.
type ChatMessage struct {
	Role        string    `json:"role"`         // "user", "assistant", or "system"
	Content     string    `json:"content"`
	PersonaName string    `json:"persona_name"`
	SentAt      time.Time `json:"sent_at"`
}

// Session represents an active elicitation conversation.
type Session struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Persona   PersonaType   `json:"persona"`
	Messages  []ChatMessage `json:"messages"`
	CreatedAt time.Time     `json:"created_at"`
}

// SessionSummary is a lightweight view of a session for listing.
type SessionSummary struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Persona      PersonaType `json:"persona"`
	MessageCount int         `json:"message_count"`
	CreatedAt    time.Time   `json:"created_at"`
}

// SessionStore manages in-memory elicitation sessions.
type SessionStore interface {
	// Create initializes a new session with the given persona and returns it.
	Create(persona PersonaType) *Session

	// Get retrieves a session by ID. Returns nil, ErrSessionNotFound if missing.
	Get(id string) (*Session, error)

	// Put stores a session (e.g., restored from markdown). Overwrites if ID exists.
	Put(session *Session)

	// List returns all active session summaries.
	List() []SessionSummary

	// Delete removes a session by ID.
	Delete(id string) error
}

// ValidateSessionName checks that a name is valid for a session.
// Returns an error if the name is empty (after trimming) or exceeds 200 characters.
func ValidateSessionName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("name must be non-empty")
	}
	if len(trimmed) > 200 {
		return fmt.Errorf("name must not exceed 200 characters")
	}
	return nil
}

// inMemorySessionStore is a thread-safe in-memory implementation of SessionStore.
type inMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewInMemorySessionStore creates a new in-memory session store.
func NewInMemorySessionStore() SessionStore {
	return &inMemorySessionStore{
		sessions: make(map[string]*Session),
	}
}

// generateSessionID produces a cryptographically random hex-encoded ID.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// Create initializes a new session with the given persona and returns it.
func (s *inMemorySessionStore) Create(persona PersonaType) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess := &Session{
		ID:        generateSessionID(),
		Name:      "Untitled Session",
		Persona:   persona,
		Messages:  []ChatMessage{},
		CreatedAt: time.Now(),
	}
	s.sessions[sess.ID] = sess
	return sess
}

// Put stores a session (e.g., restored from markdown). Overwrites if ID exists.
func (s *inMemorySessionStore) Put(session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
}

// Get retrieves a session by ID. Returns ErrSessionNotFound if missing.
func (s *inMemorySessionStore) Get(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}

// List returns all active session summaries.
func (s *inMemorySessionStore) List() []SessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	summaries := make([]SessionSummary, 0, len(s.sessions))
	for _, sess := range s.sessions {
		summaries = append(summaries, SessionSummary{
			ID:           sess.ID,
			Name:         sess.Name,
			Persona:      sess.Persona,
			MessageCount: len(sess.Messages),
			CreatedAt:    sess.CreatedAt,
		})
	}
	return summaries
}

// Delete removes a session by ID. Returns ErrSessionNotFound if missing.
func (s *inMemorySessionStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[id]; !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, id)
	return nil
}
