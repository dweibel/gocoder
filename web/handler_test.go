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
	"sort"
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

// ============================================================================
// Task 5.4: Property test — Session list sorted descending by creation time
// Feature: chat-ui, Property 3: Session list sorted descending by creation time
// Validates: Requirements 2.4
// ============================================================================

func TestSessionListSortedDescending(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		sessions := elicitation.NewInMemorySessionStore()
		savePath := t.TempDir()
		h := NewHandler(
			&mockElicitor{},
			sessions,
			newMockStore(),
			&mockMarkdownCodec{},
			savePath,
		)

		// Generate 1–15 sessions with random CreatedAt timestamps
		n := rapid.IntRange(1, 15).Draw(rt, "numSessions")
		type sessionInfo struct {
			id        string
			createdAt time.Time
		}
		created := make([]sessionInfo, 0, n)

		baseTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		for i := 0; i < n; i++ {
			sess := sessions.Create(elicitation.PersonaTrustedAdv)
			// Assign a random CreatedAt by adding random seconds
			offsetSec := rapid.Int64Range(0, 5*365*24*3600).Draw(rt, fmt.Sprintf("offset_%d", i))
			sess.CreatedAt = baseTime.Add(time.Duration(offsetSec) * time.Second)
			created = append(created, sessionInfo{id: sess.ID, createdAt: sess.CreatedAt})
		}

		// Call HandleListSessions (JSON API) to check sort order
		req := httptest.NewRequest("GET", "/api/sessions", nil)
		w := httptest.NewRecorder()
		mux := setupMux(h)
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			rt.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var summaries []elicitation.SessionSummary
		if err := json.NewDecoder(w.Body).Decode(&summaries); err != nil {
			rt.Fatalf("failed to decode response: %v", err)
		}

		// All sessions must appear
		if len(summaries) != n {
			rt.Fatalf("expected %d sessions, got %d", n, len(summaries))
		}

		// Now test the landing page handler which sorts descending
		tmpls, err := ParseTemplates("templates")
		if err != nil {
			rt.Fatalf("failed to parse templates: %v", err)
		}
		h.SetTemplates(tmpls)

		req2 := httptest.NewRequest("GET", "/", nil)
		w2 := httptest.NewRecorder()
		mux2 := setupMux(h)
		mux2.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			rt.Fatalf("expected 200 for landing page, got %d: %s", w2.Code, w2.Body.String())
		}

		body := w2.Body.String()

		// Verify all session IDs appear in the HTML
		for _, s := range created {
			if !strings.Contains(body, s.id) {
				rt.Fatalf("session %s missing from landing page", s.id)
			}
		}

		// Verify descending order: extract session ID positions in the HTML
		// Each session appears as /chat/{id}, so find their positions
		positions := make([]int, 0, n)
		idByPos := make(map[int]string)
		for _, s := range created {
			pos := strings.Index(body, "/chat/"+s.id)
			if pos < 0 {
				rt.Fatalf("session link /chat/%s not found in landing page", s.id)
			}
			positions = append(positions, pos)
			idByPos[pos] = s.id
		}

		// Build a map from session ID to CreatedAt
		createdAtByID := make(map[string]time.Time)
		for _, s := range created {
			createdAtByID[s.id] = s.createdAt
		}

		// Sort positions to get the order they appear in HTML
		sort.Ints(positions)

		// Verify that sessions appear in descending CreatedAt order
		for i := 1; i < len(positions); i++ {
			prevID := idByPos[positions[i-1]]
			currID := idByPos[positions[i]]
			if createdAtByID[prevID].Before(createdAtByID[currID]) {
				rt.Fatalf("sessions not sorted descending: %s (created %v) appears before %s (created %v)",
					prevID, createdAtByID[prevID], currID, createdAtByID[currID])
			}
		}
	})
}

// ============================================================================
// Task 5.5: Property test — Persona switch preserves history and appends notification
// Feature: chat-ui, Property 4: Persona switch preserves history and appends notification
// Validates: Requirements 5.4, 5.6, 5.7
// ============================================================================

func TestPersonaSwitchPreservesHistoryAndNotifies(t *testing.T) {
	validPersonaTypes := []elicitation.PersonaType{
		elicitation.PersonaSocraticBA,
		elicitation.PersonaHostileSA,
		elicitation.PersonaTrustedAdv,
	}

	rapid.Check(t, func(rt *rapid.T) {
		sessions := elicitation.NewInMemorySessionStore()
		savePath := t.TempDir()
		h := NewHandler(
			&mockElicitor{},
			sessions,
			newMockStore(),
			&mockMarkdownCodec{},
			savePath,
		)
		mux := setupMux(h)

		// Pick a random initial persona
		initialIdx := rapid.IntRange(0, len(validPersonaTypes)-1).Draw(rt, "initialPersona")
		initialPersona := validPersonaTypes[initialIdx]

		sess := sessions.Create(initialPersona)

		// Generate 0–20 random messages
		numMessages := rapid.IntRange(0, 20).Draw(rt, "numMessages")
		for i := 0; i < numMessages; i++ {
			role := rapid.SampledFrom([]string{"user", "assistant"}).Draw(rt, fmt.Sprintf("role_%d", i))
			content := rapid.StringMatching(`[a-zA-Z0-9 ]{1,50}`).Draw(rt, fmt.Sprintf("content_%d", i))
			sess.Messages = append(sess.Messages, elicitation.ChatMessage{
				Role:    role,
				Content: content,
				SentAt:  time.Now(),
			})
		}

		// Snapshot original messages
		originalMessages := make([]elicitation.ChatMessage, len(sess.Messages))
		copy(originalMessages, sess.Messages)

		// Pick a target persona different from the current one
		var targetPersona elicitation.PersonaType
		for {
			targetIdx := rapid.IntRange(0, len(validPersonaTypes)-1).Draw(rt, "targetPersona")
			targetPersona = validPersonaTypes[targetIdx]
			if targetPersona != initialPersona {
				break
			}
		}

		// Call HandleUpdatePersona
		body := fmt.Sprintf(`{"persona": %q}`, string(targetPersona))
		req := httptest.NewRequest("PUT", "/api/sessions/"+sess.ID+"/persona", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			rt.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify response contains persona and display_name
		var resp map[string]string
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			rt.Fatalf("failed to decode response: %v", err)
		}
		if resp["persona"] != string(targetPersona) {
			rt.Fatalf("expected persona %q, got %q", targetPersona, resp["persona"])
		}
		expectedDisplayName := targetPersona.DisplayName()
		if resp["display_name"] != expectedDisplayName {
			rt.Fatalf("expected display_name %q, got %q", expectedDisplayName, resp["display_name"])
		}

		// Property (a): session.Persona updated
		updatedSess, err := sessions.Get(sess.ID)
		if err != nil {
			rt.Fatalf("failed to get session: %v", err)
		}
		if updatedSess.Persona != targetPersona {
			rt.Fatalf("expected session persona %q, got %q", targetPersona, updatedSess.Persona)
		}

		// Property (b): first N messages unchanged
		if len(updatedSess.Messages) != numMessages+1 {
			rt.Fatalf("expected %d messages, got %d", numMessages+1, len(updatedSess.Messages))
		}
		for i := 0; i < numMessages; i++ {
			if updatedSess.Messages[i].Role != originalMessages[i].Role {
				rt.Fatalf("message %d role changed: %q -> %q", i, originalMessages[i].Role, updatedSess.Messages[i].Role)
			}
			if updatedSess.Messages[i].Content != originalMessages[i].Content {
				rt.Fatalf("message %d content changed", i)
			}
		}

		// Property (c): message N+1 is system notification with new persona display name
		notification := updatedSess.Messages[numMessages]
		if notification.Role != "system" {
			rt.Fatalf("expected notification role 'system', got %q", notification.Role)
		}
		if !strings.Contains(notification.Content, expectedDisplayName) {
			rt.Fatalf("expected notification to contain %q, got %q", expectedDisplayName, notification.Content)
		}
	})
}

// ============================================================================
// Task 5.6: Property test — Invalid persona identifier rejected
// Feature: chat-ui, Property 7: Invalid persona identifier rejected
// Validates: Requirements 8.3
// ============================================================================

func TestInvalidPersonaRejected(t *testing.T) {
	validPersonas := map[string]bool{
		"socratic_business_analyst": true,
		"hostile_systems_architect": true,
		"trusted_advisor":           true,
	}

	rapid.Check(t, func(rt *rapid.T) {
		sessions := elicitation.NewInMemorySessionStore()
		savePath := t.TempDir()
		h := NewHandler(
			&mockElicitor{},
			sessions,
			newMockStore(),
			&mockMarkdownCodec{},
			savePath,
		)
		mux := setupMux(h)

		sess := sessions.Create(elicitation.PersonaTrustedAdv)

		// Generate a random string that is NOT one of the three valid personas
		persona := rapid.StringMatching(`[a-zA-Z0-9_]{1,50}`).Draw(rt, "invalidPersona")
		if validPersonas[persona] {
			// If we accidentally generated a valid one, mutate it
			persona = persona + "_invalid"
		}

		body := fmt.Sprintf(`{"persona": %q}`, persona)
		req := httptest.NewRequest("PUT", "/api/sessions/"+sess.ID+"/persona", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		// Property: HTTP 400
		if w.Code != http.StatusBadRequest {
			rt.Fatalf("expected 400 for invalid persona %q, got %d: %s", persona, w.Code, w.Body.String())
		}

		// Property: JSON error body with "error" field
		var resp map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			rt.Fatalf("expected valid JSON error response for persona %q, got: %s", persona, w.Body.String())
		}
		if _, ok := resp["error"]; !ok {
			rt.Fatalf("expected 'error' field in response for persona %q, got: %v", persona, resp)
		}
	})
}

// ============================================================================
// Task 5.7: Unit tests for new handlers
// Requirements: 2.6, 2.7, 7.3, 8.4, 9.4, 10.1, 10.2
// ============================================================================

func TestLandingPageEmptyState(t *testing.T) {
	h, _ := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "No sessions yet") {
		t.Fatalf("expected empty-state message, got: %s", body)
	}
}

func TestLandingPageSessionLinks(t *testing.T) {
	h, sessions := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	s1 := sessions.Create(elicitation.PersonaTrustedAdv)
	s2 := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "/chat/"+s1.ID) {
		t.Fatalf("expected link to /chat/%s in landing page", s1.ID)
	}
	if !strings.Contains(body, "/chat/"+s2.ID) {
		t.Fatalf("expected link to /chat/%s in landing page", s2.ID)
	}
}

func TestPersonaSwitchNotFoundSession(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	body := `{"persona": "trusted_advisor"}`
	req := httptest.NewRequest("PUT", "/api/sessions/nonexistent-id/persona", strings.NewReader(body))
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

func TestNameUpdateNotFoundSession(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	body := `{"name": "New Name"}`
	req := httptest.NewRequest("PUT", "/api/sessions/nonexistent-id/name", strings.NewReader(body))
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

func TestEmptySessionListAPI(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON array, got: %s", w.Body.String())
	}
	if len(resp) != 0 {
		t.Fatalf("expected empty array, got %d elements", len(resp))
	}
}

func TestSessionSummaryIncludesAllFields(t *testing.T) {
	h, sessions := newTestHandler(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)
	sess.Name = "Test Session"

	req := httptest.NewRequest("GET", "/api/sessions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var summaries []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&summaries); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(summaries) != 1 {
		t.Fatalf("expected 1 session, got %d", len(summaries))
	}

	s := summaries[0]
	requiredFields := []string{"id", "name", "persona", "message_count", "created_at"}
	for _, field := range requiredFields {
		if _, ok := s[field]; !ok {
			t.Fatalf("expected field %q in session summary, got: %v", field, s)
		}
	}

	if s["id"] != sess.ID {
		t.Fatalf("expected id %q, got %q", sess.ID, s["id"])
	}
	if s["persona"] != string(elicitation.PersonaSocraticBA) {
		t.Fatalf("expected persona %q, got %q", elicitation.PersonaSocraticBA, s["persona"])
	}
}

func TestChatPageShowsSessionNameAndPersona(t *testing.T) {
	h, sessions := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)
	sess.Name = "My Feature Discussion"

	req := httptest.NewRequest("GET", "/chat/"+sess.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "My Feature Discussion") {
		t.Fatalf("expected session name in chat page HTML, got: %s", body)
	}
	if !strings.Contains(body, "Socratic Business Analyst") {
		t.Fatalf("expected persona display name in chat page HTML, got: %s", body)
	}
}
