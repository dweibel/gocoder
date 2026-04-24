package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardp/coding-agent/elicitation"
	"github.com/ardp/coding-agent/store"
	"pgregory.net/rapid"
)

// ============================================================================
// Mock infrastructure
// ============================================================================

// mockElicitor implements elicitation.Elicitor for testing.
type mockElicitor struct {
	chatResp      elicitation.ChatMessage
	chatErr       error
	synthResp     []elicitation.Artifact
	synthErr      error
}

func (m *mockElicitor) Chat(ctx context.Context, session *elicitation.Session, userMessage string) (elicitation.ChatMessage, error) {
	if m.chatErr != nil {
		return elicitation.ChatMessage{}, m.chatErr
	}
	resp := m.chatResp
	if resp.Role == "" {
		resp = elicitation.ChatMessage{
			Role:    "assistant",
			Content: "mock response to: " + userMessage,
			SentAt:  time.Now(),
		}
	}
	// Simulate appending messages to session (like the real engine does)
	session.Messages = append(session.Messages,
		elicitation.ChatMessage{Role: "user", Content: userMessage, SentAt: time.Now()},
		resp,
	)
	return resp, nil
}

func (m *mockElicitor) Synthesize(ctx context.Context, session *elicitation.Session) ([]elicitation.Artifact, error) {
	if m.synthErr != nil {
		return nil, m.synthErr
	}
	if m.synthResp != nil {
		return m.synthResp, nil
	}
	return []elicitation.Artifact{
		{Type: "BRD", Title: "BRD", Content: "Business requirements"},
		{Type: "SRS", Title: "SRS", Content: "Software requirements"},
	}, nil
}

// mockStore implements store.Store for testing.
type mockStore struct {
	projects  map[string]store.Project
	artifacts map[string][]store.ArtifactRecord
	tasks     map[string][]store.Task
}

func newMockStore() *mockStore {
	return &mockStore{
		projects:  make(map[string]store.Project),
		artifacts: make(map[string][]store.ArtifactRecord),
		tasks:     make(map[string][]store.Task),
	}
}

func (m *mockStore) InitSchema(ctx context.Context) error { return nil }
func (m *mockStore) Close() error                         { return nil }

func (m *mockStore) InsertProject(ctx context.Context, p store.Project) error {
	m.projects[p.ID] = p
	return nil
}

func (m *mockStore) GetProject(ctx context.Context, id string) (store.Project, error) {
	p, ok := m.projects[id]
	if !ok {
		return store.Project{}, fmt.Errorf("project %q not found", id)
	}
	return p, nil
}

func (m *mockStore) InsertArtifacts(ctx context.Context, projectID string, artifacts []store.ArtifactRecord) error {
	m.artifacts[projectID] = append(m.artifacts[projectID], artifacts...)
	return nil
}

func (m *mockStore) ListArtifacts(ctx context.Context, projectID string) ([]store.ArtifactRecord, error) {
	arts, ok := m.artifacts[projectID]
	if !ok {
		return []store.ArtifactRecord{}, nil
	}
	return arts, nil
}

func (m *mockStore) InsertTasks(ctx context.Context, projectID string, tasks []store.Task) error {
	m.tasks[projectID] = append(m.tasks[projectID], tasks...)
	return nil
}

func (m *mockStore) ListTasks(ctx context.Context, projectID string) ([]store.Task, error) {
	tks, ok := m.tasks[projectID]
	if !ok {
		return []store.Task{}, nil
	}
	return tks, nil
}

// mockMarkdownCodec implements elicitation.ConversationMarshaler for testing.
type mockMarkdownCodec struct {
	marshalData []byte
	marshalErr  error
	unmarshalSession *elicitation.Session
	unmarshalErr     error
}

func (m *mockMarkdownCodec) Marshal(session *elicitation.Session) ([]byte, error) {
	if m.marshalErr != nil {
		return nil, m.marshalErr
	}
	if m.marshalData != nil {
		return m.marshalData, nil
	}
	return []byte("# mock markdown for " + session.ID), nil
}

func (m *mockMarkdownCodec) Unmarshal(data []byte) (*elicitation.Session, error) {
	if m.unmarshalErr != nil {
		return nil, m.unmarshalErr
	}
	if m.unmarshalSession != nil {
		return m.unmarshalSession, nil
	}
	return &elicitation.Session{
		ID:        "restored-session",
		Persona:   elicitation.PersonaSocraticBA,
		Messages:  []elicitation.ChatMessage{},
		CreatedAt: time.Now(),
	}, nil
}

// newTestHandler creates a Handler with mock dependencies and returns it along with the session store.
func newTestHandler(t *testing.T) (*Handler, elicitation.SessionStore) {
	t.Helper()
	sessions := elicitation.NewInMemorySessionStore()
	savePath := t.TempDir()
	h := NewHandler(
		&mockElicitor{},
		sessions,
		newMockStore(),
		&mockMarkdownCodec{},
		savePath,
	)
	return h, sessions
}

// setupMux creates a ServeMux with all routes registered for the given handler.
func setupMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterRoutes(mux, h)
	return mux
}

// ============================================================================
// Task 12.1: Unit tests for all HTTP handlers using httptest
// Requirements: 12.1, 12.2, 12.3, 12.4, 12.5, 12.6
// ============================================================================

func TestHandleNewSession(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	body := `{"persona": "socratic_business_analyst"}`
	req := httptest.NewRequest("POST", "/api/sessions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	sessionID, ok := resp["session_id"]
	if !ok || sessionID == "" {
		t.Fatal("expected non-empty session_id in response")
	}
}

func TestHandleChat(t *testing.T) {
	h, sessions := newTestHandler(t)
	mux := setupMux(h)

	// Create a session first
	sess := sessions.Create(elicitation.PersonaSocraticBA)

	body := `{"content": "Hello, I want to build a feature"}`
	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["role"] != "assistant" {
		t.Fatalf("expected role 'assistant', got %v", resp["role"])
	}
	content, ok := resp["content"].(string)
	if !ok || content == "" {
		t.Fatal("expected non-empty content in response")
	}
	if _, ok := resp["sent_at"]; !ok {
		t.Fatal("expected sent_at in response")
	}
}

func TestHandleSynthesize(t *testing.T) {
	h, sessions := newTestHandler(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/synthesize", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	artifacts, ok := resp["artifacts"]
	if !ok {
		t.Fatal("expected 'artifacts' key in response")
	}
	artList, ok := artifacts.([]interface{})
	if !ok || len(artList) == 0 {
		t.Fatal("expected non-empty artifacts array")
	}
}

func TestHandleSaveSession(t *testing.T) {
	h, sessions := newTestHandler(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/save", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	path, ok := resp["path"]
	if !ok || path == "" {
		t.Fatal("expected non-empty path in response")
	}

	// Verify the file was actually written
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected file at %s to exist", path)
	}
}

func TestHandleLoadSession(t *testing.T) {
	sessions := elicitation.NewInMemorySessionStore()
	savePath := t.TempDir()

	restoredSession := &elicitation.Session{
		ID:        "loaded-sess-123",
		Persona:   elicitation.PersonaHostileSA,
		Messages:  []elicitation.ChatMessage{},
		CreatedAt: time.Now(),
	}

	h := NewHandler(
		&mockElicitor{},
		sessions,
		newMockStore(),
		&mockMarkdownCodec{unmarshalSession: restoredSession},
		savePath,
	)
	mux := setupMux(h)

	// Write a dummy markdown file to load
	mdPath := filepath.Join(savePath, "test-session.md")
	if err := os.WriteFile(mdPath, []byte("# test"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	body := fmt.Sprintf(`{"path": %q}`, mdPath)
	req := httptest.NewRequest("POST", "/api/sessions/load", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	sessionID, ok := resp["session_id"]
	if !ok || sessionID == "" {
		t.Fatal("expected non-empty session_id in response")
	}

	// Verify the session is accessible in the store
	_, err := sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("loaded session should be accessible: %v", err)
	}
}

func TestHandleListArtifacts(t *testing.T) {
	ms := newMockStore()
	projectID := "proj-1"
	ms.projects[projectID] = store.Project{ID: projectID, Name: "Test Project", CreatedAt: time.Now()}
	ms.artifacts[projectID] = []store.ArtifactRecord{
		{ID: "art-1", ProjectID: projectID, ArtifactType: "BRD", Content: "BRD content", CreatedAt: time.Now()},
		{ID: "art-2", ProjectID: projectID, ArtifactType: "SRS", Content: "SRS content", CreatedAt: time.Now()},
	}

	h := NewHandler(&mockElicitor{}, elicitation.NewInMemorySessionStore(), ms, &mockMarkdownCodec{}, t.TempDir())
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/api/projects/"+projectID+"/artifacts", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	artifacts, ok := resp["artifacts"].([]interface{})
	if !ok {
		t.Fatal("expected 'artifacts' array in response")
	}
	if len(artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
	}
}

func TestHandleExportArtifacts(t *testing.T) {
	ms := newMockStore()
	projectID := "proj-export"
	ms.projects[projectID] = store.Project{ID: projectID, Name: "Export Project", CreatedAt: time.Now()}
	ms.artifacts[projectID] = []store.ArtifactRecord{
		{ID: "art-1", ProjectID: projectID, ArtifactType: "BRD", Content: "BRD export content", CreatedAt: time.Now()},
	}

	h := NewHandler(&mockElicitor{}, elicitation.NewInMemorySessionStore(), ms, &mockMarkdownCodec{}, t.TempDir())
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/api/projects/"+projectID+"/export", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Export should return markdown content
	body := w.Body.String()
	if !strings.Contains(body, "BRD export content") {
		t.Fatalf("expected export to contain artifact content, got: %s", body)
	}
}

func TestMalformedJSONBody_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	// Test malformed JSON on HandleNewSession
	req := httptest.NewRequest("POST", "/api/sessions", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON error response, got: %s", w.Body.String())
	}
	if _, ok := resp["error"]; !ok {
		t.Fatal("expected 'error' field in JSON response")
	}
}

func TestNonexistentSession_Returns404(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	body := `{"content": "hello"}`
	req := httptest.NewRequest("POST", "/api/sessions/nonexistent-id/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON error response, got: %s", w.Body.String())
	}
	if _, ok := resp["error"]; !ok {
		t.Fatal("expected 'error' field in JSON response")
	}
}

func TestMalformedJSONOnChat_Returns400(t *testing.T) {
	h, sessions := newTestHandler(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/messages", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON error response, got: %s", w.Body.String())
	}
	if _, ok := resp["error"]; !ok {
		t.Fatal("expected 'error' field in JSON response")
	}
}

func TestMalformedJSONOnLoadSession_Returns400(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	req := httptest.NewRequest("POST", "/api/sessions/load", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON error response, got: %s", w.Body.String())
	}
	if _, ok := resp["error"]; !ok {
		t.Fatal("expected 'error' field in JSON response")
	}
}

// ============================================================================
// Task 12.2: Property test — Malformed HTTP request returns structured JSON error
// Feature: elicitation-engine, Property 14: Malformed HTTP request returns structured JSON error
// Validates: Requirements 12.6
// ============================================================================

func TestMalformedHTTPError(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	rapid.Check(t, func(t *rapid.T) {

		// Generate random invalid JSON payloads
		// Strategy: generate byte sequences that are NOT valid JSON
		strategy := rapid.IntRange(0, 3).Draw(t, "strategy")
		var payload string
		switch strategy {
		case 0:
			// Random bytes that aren't valid JSON
			payload = rapid.StringMatching(`[^{}\[\]"0-9a-zA-Z]{1,100}`).Draw(t, "randomBytes")
		case 1:
			// Truncated JSON object
			payload = "{" + rapid.StringMatching(`"[a-z]{1,20}":`).Draw(t, "truncatedJSON")
		case 2:
			// JSON with trailing garbage
			payload = `{"persona":"test"}` + rapid.StringMatching(`[^}\s]{1,20}`).Draw(t, "trailingGarbage")
			// This might actually parse, so use a definitely broken one
			payload = `{"persona":` + rapid.StringMatching(`[a-z]{1,20}`).Draw(t, "unquotedValue")
		case 3:
			// Empty or whitespace-only
			payload = rapid.StringMatching(`\s{0,10}`).Draw(t, "whitespace")
			if strings.TrimSpace(payload) == "" {
				payload = "" // ensure truly empty
			}
		}

		// Ensure the payload is actually invalid JSON
		var js json.RawMessage
		if json.Unmarshal([]byte(payload), &js) == nil {
			// If it happens to be valid JSON, make it invalid
			payload = payload + "{"
		}

		req := httptest.NewRequest("POST", "/api/sessions", bytes.NewReader([]byte(payload)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		// Property: handler returns 400
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid JSON %q, got %d: %s", payload, w.Code, w.Body.String())
		}

		// Property: response is valid JSON with "error" field
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("expected valid JSON error response for payload %q, got: %s", payload, w.Body.String())
		}
		if _, ok := resp["error"]; !ok {
			t.Fatalf("expected 'error' field in response for payload %q, got: %v", payload, resp)
		}
	})
}
