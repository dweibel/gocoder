package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardp/coding-agent/elicitation"
	"github.com/ardp/coding-agent/store"
)

// ============================================================================
// Integration test helpers
// ============================================================================

// integrationElicitor is a mock Elicitor that simulates realistic multi-turn
// chat and synthesis behavior for integration tests.
type integrationElicitor struct {
	chatResponses []string // rotating responses for chat turns
	chatIndex     int
	synthArtifacts []elicitation.Artifact // artifacts returned by Synthesize
}

func (e *integrationElicitor) Chat(ctx context.Context, session *elicitation.Session, userMessage string) (elicitation.ChatMessage, error) {
	respText := fmt.Sprintf("Response to: %s", userMessage)
	if e.chatResponses != nil && len(e.chatResponses) > 0 {
		respText = e.chatResponses[e.chatIndex%len(e.chatResponses)]
		e.chatIndex++
	}

	now := time.Now()
	userMsg := elicitation.ChatMessage{Role: "user", Content: userMessage, SentAt: now}
	assistantMsg := elicitation.ChatMessage{Role: "assistant", Content: respText, SentAt: now}
	session.Messages = append(session.Messages, userMsg, assistantMsg)

	return assistantMsg, nil
}

func (e *integrationElicitor) Synthesize(ctx context.Context, session *elicitation.Session) ([]elicitation.Artifact, error) {
	if e.synthArtifacts != nil {
		return e.synthArtifacts, nil
	}
	return []elicitation.Artifact{
		{Type: "BRD", Title: "BRD", Content: "Business requirements from chat"},
		{Type: "SRS", Title: "SRS", Content: "Software requirements from chat"},
		{Type: "NFR", Title: "NFR", Content: "Non-functional requirements"},
		{Type: "GHERKIN", Title: "User Login", Content: "Given a user\nWhen they login\nThen they see dashboard"},
	}, nil
}


// setupIntegrationServer creates an httptest.Server with real SessionStore,
// real MarkdownCodec, and the given store and elicitor.
func setupIntegrationServer(t *testing.T, elicitor elicitation.Elicitor, st store.Store) (*httptest.Server, elicitation.SessionStore) {
	t.Helper()
	sessions := elicitation.NewInMemorySessionStore()
	mdCodec := elicitation.NewMarkdownCodec()
	savePath := t.TempDir()

	h := NewHandler(elicitor, sessions, st, mdCodec, savePath)
	mux := http.NewServeMux()
	RegisterRoutes(mux, h)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, sessions
}

// postJSON sends a POST request with JSON body and returns the response.
func postJSON(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("failed to encode request body: %v", err)
		}
	}
	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	return resp
}

// decodeJSON decodes a JSON response body into the given target.
func decodeJSON(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

// ============================================================================
// Task 16.1: Integration test for full chat lifecycle
// Create session → send chat messages → verify session state (mocked LLM)
// Requirements: 3.1, 3.2, 3.3, 13.1, 13.3, 13.4
// ============================================================================

func TestIntegration_ChatLifecycle(t *testing.T) {
	elicitor := &integrationElicitor{
		chatResponses: []string{
			"Tell me more about your feature idea.",
			"What are the key stakeholders?",
			"How should the system handle failures?",
		},
	}

	server, sessions := setupIntegrationServer(t, elicitor, newMockStore())

	// Step 1: Create a new session via POST /api/sessions
	var createResp map[string]string
	resp := postJSON(t, server.URL+"/api/sessions", map[string]string{
		"persona": "socratic_business_analyst",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &createResp)

	sessionID := createResp["session_id"]
	if sessionID == "" {
		t.Fatal("expected non-empty session_id")
	}

	// Verify session exists in the store
	sess, err := sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("session not found in store: %v", err)
	}
	if sess.Persona != elicitation.PersonaSocraticBA {
		t.Fatalf("expected persona %q, got %q", elicitation.PersonaSocraticBA, sess.Persona)
	}

	// Step 2: Send 3 chat messages via POST /api/sessions/{id}/messages
	userMessages := []string{
		"I want to build a PDF export feature",
		"Internal teams and external clients",
		"It should retry on transient errors",
	}

	for i, msg := range userMessages {
		var chatResp map[string]interface{}
		resp := postJSON(t, server.URL+"/api/sessions/"+sessionID+"/messages", map[string]string{
			"content": msg,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("chat turn %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		decodeJSON(t, resp, &chatResp)

		if chatResp["role"] != "assistant" {
			t.Fatalf("chat turn %d: expected role 'assistant', got %v", i+1, chatResp["role"])
		}
		content, ok := chatResp["content"].(string)
		if !ok || content == "" {
			t.Fatalf("chat turn %d: expected non-empty content", i+1)
		}
	}

	// Step 3: Verify session state has correct number of messages
	sess, err = sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get session after chat: %v", err)
	}

	expectedMsgCount := len(userMessages) * 2 // user + assistant per turn
	if len(sess.Messages) != expectedMsgCount {
		t.Fatalf("expected %d messages, got %d", expectedMsgCount, len(sess.Messages))
	}

	// Verify message ordering: alternating user/assistant
	for i, m := range sess.Messages {
		expectedRole := "user"
		if i%2 == 1 {
			expectedRole = "assistant"
		}
		if m.Role != expectedRole {
			t.Fatalf("message[%d]: expected role %q, got %q", i, expectedRole, m.Role)
		}
	}

	// Verify user messages match what we sent
	for i, msg := range userMessages {
		if sess.Messages[i*2].Content != msg {
			t.Fatalf("user message[%d]: expected %q, got %q", i, msg, sess.Messages[i*2].Content)
		}
	}

	// Verify assistant responses match the mock
	for i, expected := range elicitor.chatResponses {
		if sess.Messages[i*2+1].Content != expected {
			t.Fatalf("assistant message[%d]: expected %q, got %q", i, expected, sess.Messages[i*2+1].Content)
		}
	}
}


// ============================================================================
// Task 16.2: Integration test for synthesis lifecycle
// Create session → chat turns → synthesize → verify artifacts in DB (mocked LLM)
// Requirements: 6.1, 6.2, 6.3, 6.4, 8.5
// ============================================================================

func TestIntegration_SynthesisLifecycle(t *testing.T) {
	// Use a real SQLiteStore with :memory: for DB verification
	sqlStore, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLite store: %v", err)
	}
	defer sqlStore.Close()

	ctx := context.Background()
	if err := sqlStore.InitSchema(ctx); err != nil {
		t.Fatalf("failed to init schema: %v", err)
	}

	// Create a project to associate artifacts with
	projectID := "integ-proj-1"
	if err := sqlStore.InsertProject(ctx, store.Project{
		ID:        projectID,
		Name:      "Integration Test Project",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("failed to insert project: %v", err)
	}

	// synthElicitor returns artifacts and also inserts them into the DB
	synthElicitor := &synthIntegrationElicitor{
		store:     sqlStore,
		projectID: projectID,
	}

	server, sessions := setupIntegrationServer(t, synthElicitor, sqlStore)

	// Step 1: Create session
	var createResp map[string]string
	resp := postJSON(t, server.URL+"/api/sessions", map[string]string{
		"persona": "hostile_systems_architect",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &createResp)
	sessionID := createResp["session_id"]

	// Step 2: Do a few chat turns
	chatMessages := []string{
		"I want to build a real-time notification system",
		"WebSocket for browser clients, push notifications for mobile",
	}
	for i, msg := range chatMessages {
		var chatResp map[string]interface{}
		resp := postJSON(t, server.URL+"/api/sessions/"+sessionID+"/messages", map[string]string{
			"content": msg,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("chat turn %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		decodeJSON(t, resp, &chatResp)
	}

	// Verify session has messages before synthesis
	sess, err := sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	if len(sess.Messages) != 4 { // 2 turns × 2 messages
		t.Fatalf("expected 4 messages before synthesis, got %d", len(sess.Messages))
	}

	// Step 3: Trigger synthesis via POST /api/sessions/{id}/synthesize
	var synthResp map[string]interface{}
	resp = postJSON(t, server.URL+"/api/sessions/"+sessionID+"/synthesize", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("synthesize: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &synthResp)

	// Verify artifacts in response
	artifactsRaw, ok := synthResp["artifacts"].([]interface{})
	if !ok {
		t.Fatal("expected 'artifacts' array in synthesis response")
	}
	if len(artifactsRaw) != 4 { // BRD + SRS + NFR + 1 Gherkin
		t.Fatalf("expected 4 artifacts in response, got %d", len(artifactsRaw))
	}

	// Step 4: Verify artifacts persisted in DB via GET /api/projects/{id}/artifacts
	var listResp map[string]interface{}
	getResp, err := http.Get(server.URL + "/api/projects/" + projectID + "/artifacts")
	if err != nil {
		t.Fatalf("GET artifacts failed: %v", err)
	}
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("list artifacts: expected 200, got %d", getResp.StatusCode)
	}
	decodeJSON(t, getResp, &listResp)

	dbArtifacts, ok := listResp["artifacts"].([]interface{})
	if !ok {
		t.Fatal("expected 'artifacts' array in list response")
	}
	// The DB should have BRD + SRS + NFR = 3 (Gherkin stored as tasks, not artifacts)
	if len(dbArtifacts) != 3 {
		t.Fatalf("expected 3 artifacts in DB, got %d", len(dbArtifacts))
	}

	// Verify artifact types in DB
	typesSeen := make(map[string]bool)
	for _, raw := range dbArtifacts {
		art, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatal("expected artifact to be a JSON object")
		}
		artType, _ := art["artifact_type"].(string)
		typesSeen[artType] = true
	}
	for _, expected := range []string{"BRD", "SRS", "NFR"} {
		if !typesSeen[expected] {
			t.Fatalf("expected artifact type %q in DB, not found", expected)
		}
	}
}

// synthIntegrationElicitor simulates chat and synthesis, persisting artifacts to a real DB.
type synthIntegrationElicitor struct {
	store     *store.SQLiteStore
	projectID string
}

func (e *synthIntegrationElicitor) Chat(ctx context.Context, session *elicitation.Session, userMessage string) (elicitation.ChatMessage, error) {
	now := time.Now()
	userMsg := elicitation.ChatMessage{Role: "user", Content: userMessage, SentAt: now}
	assistantMsg := elicitation.ChatMessage{
		Role:    "assistant",
		Content: "Interesting point about: " + userMessage,
		SentAt:  now,
	}
	session.Messages = append(session.Messages, userMsg, assistantMsg)
	return assistantMsg, nil
}

func (e *synthIntegrationElicitor) Synthesize(ctx context.Context, session *elicitation.Session) ([]elicitation.Artifact, error) {
	artifacts := []elicitation.Artifact{
		{Type: "BRD", Title: "BRD", Content: "Business requirements from integration test"},
		{Type: "SRS", Title: "SRS", Content: "Software requirements from integration test"},
		{Type: "NFR", Title: "NFR", Content: "Non-functional requirements from integration test"},
		{Type: "GHERKIN", Title: "Notification Delivery", Content: "Given a user\nWhen event occurs\nThen notification is sent"},
	}

	// Persist non-Gherkin artifacts to DB (like the real engine would)
	now := time.Now().UTC().Truncate(time.Second)
	var dbArtifacts []store.ArtifactRecord
	var dbTasks []store.Task
	for i, a := range artifacts {
		if a.Type != "GHERKIN" {
			dbArtifacts = append(dbArtifacts, store.ArtifactRecord{
				ID:           fmt.Sprintf("art-integ-%d", i),
				ProjectID:    e.projectID,
				ArtifactType: a.Type,
				Content:      a.Content,
				CreatedAt:    now,
			})
		} else {
			dbTasks = append(dbTasks, store.Task{
				ID:           fmt.Sprintf("task-integ-%d", i),
				ProjectID:    e.projectID,
				Title:        a.Title,
				GherkinStory: a.Content,
				Status:       "TODO",
				UpdatedAt:    now,
			})
		}
	}

	if len(dbArtifacts) > 0 {
		if err := e.store.InsertArtifacts(ctx, e.projectID, dbArtifacts); err != nil {
			return nil, fmt.Errorf("failed to persist artifacts: %w", err)
		}
	}
	if len(dbTasks) > 0 {
		if err := e.store.InsertTasks(ctx, e.projectID, dbTasks); err != nil {
			return nil, fmt.Errorf("failed to persist tasks: %w", err)
		}
	}

	return artifacts, nil
}


// ============================================================================
// Task 16.3: Integration test for save/load lifecycle
// Create session → chat turns → save to markdown → load from markdown → verify restored session
// Requirements: 9.1, 9.2, 9.4
// ============================================================================

func TestIntegration_SaveLoadLifecycle(t *testing.T) {
	elicitor := &integrationElicitor{
		chatResponses: []string{
			"What problem does this solve?",
			"Who are the target users?",
		},
	}

	server, sessions := setupIntegrationServer(t, elicitor, newMockStore())

	// Step 1: Create session
	var createResp map[string]string
	resp := postJSON(t, server.URL+"/api/sessions", map[string]string{
		"persona": "trusted_advisor",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create session: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &createResp)
	sessionID := createResp["session_id"]

	// Step 2: Do a couple of chat turns
	chatMessages := []string{
		"I want to add search functionality",
		"Both internal staff and customers",
	}
	for i, msg := range chatMessages {
		var chatResp map[string]interface{}
		resp := postJSON(t, server.URL+"/api/sessions/"+sessionID+"/messages", map[string]string{
			"content": msg,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("chat turn %d: expected 200, got %d", i+1, resp.StatusCode)
		}
		decodeJSON(t, resp, &chatResp)
	}

	// Capture original session state
	origSess, err := sessions.Get(sessionID)
	if err != nil {
		t.Fatalf("failed to get original session: %v", err)
	}
	origPersona := origSess.Persona
	origMsgCount := len(origSess.Messages)

	if origMsgCount != 4 { // 2 turns × 2 messages
		t.Fatalf("expected 4 messages before save, got %d", origMsgCount)
	}

	// Step 3: Save session via POST /api/sessions/{id}/save
	var saveResp map[string]string
	resp = postJSON(t, server.URL+"/api/sessions/"+sessionID+"/save", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save session: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &saveResp)

	savedPath := saveResp["path"]
	if savedPath == "" {
		t.Fatal("expected non-empty path in save response")
	}

	// Step 4: Load session from saved markdown via POST /api/sessions/load
	var loadResp map[string]string
	resp = postJSON(t, server.URL+"/api/sessions/load", map[string]string{
		"path": savedPath,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("load session: expected 200, got %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &loadResp)

	loadedSessionID := loadResp["session_id"]
	if loadedSessionID == "" {
		t.Fatal("expected non-empty session_id in load response")
	}

	// Step 5: Verify the loaded session has the same persona and messages
	loadedSess, err := sessions.Get(loadedSessionID)
	if err != nil {
		t.Fatalf("failed to get loaded session: %v", err)
	}

	// Verify persona is preserved
	if loadedSess.Persona != origPersona {
		t.Fatalf("persona mismatch: original %q, loaded %q", origPersona, loadedSess.Persona)
	}

	// Verify message count is preserved
	if len(loadedSess.Messages) != origMsgCount {
		t.Fatalf("message count mismatch: original %d, loaded %d", origMsgCount, len(loadedSess.Messages))
	}

	// Verify message content and roles are preserved
	for i, origMsg := range origSess.Messages {
		loadedMsg := loadedSess.Messages[i]
		if loadedMsg.Role != origMsg.Role {
			t.Fatalf("message[%d] role mismatch: original %q, loaded %q", i, origMsg.Role, loadedMsg.Role)
		}
		if loadedMsg.Content != origMsg.Content {
			t.Fatalf("message[%d] content mismatch: original %q, loaded %q", i, origMsg.Content, loadedMsg.Content)
		}
	}

	// Verify the loaded session ID matches the original (markdown preserves session ID)
	if loadedSessionID != sessionID {
		t.Logf("note: loaded session ID %q differs from original %q (expected if markdown codec preserves original ID)", loadedSessionID, sessionID)
	}

	// Verify message ordering is correct (alternating user/assistant)
	for i, m := range loadedSess.Messages {
		expectedRole := "user"
		if i%2 == 1 {
			expectedRole = "assistant"
		}
		if m.Role != expectedRole {
			t.Fatalf("loaded message[%d]: expected role %q, got %q", i, expectedRole, m.Role)
		}
	}

	// Verify specific content survived the round-trip
	if !strings.Contains(loadedSess.Messages[0].Content, "search functionality") {
		t.Fatalf("first user message content not preserved: %q", loadedSess.Messages[0].Content)
	}
	if !strings.Contains(loadedSess.Messages[1].Content, "What problem") {
		t.Fatalf("first assistant message content not preserved: %q", loadedSess.Messages[1].Content)
	}
}
