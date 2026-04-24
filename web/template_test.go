package web

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardp/coding-agent/elicitation"
	"github.com/ardp/coding-agent/store"
)

// ============================================================================
// Task 13.1: Tests for template rendering
// Requirements: 4.1, 4.2
// ============================================================================

// loadTestTemplates parses all page templates from web/templates/ for testing.
func loadTestTemplateMap(t *testing.T) map[string]*template.Template {
	t.Helper()
	tmpls, err := ParseTemplates("templates")
	if err != nil {
		t.Fatalf("failed to parse templates: %v", err)
	}
	return tmpls
}

func TestLayoutTemplateParses(t *testing.T) {
	tmpls := loadTestTemplateMap(t)
	// Every page template should contain the "layout" definition
	for name, tmpl := range tmpls {
		if tmpl.Lookup("layout") == nil {
			t.Fatalf("template %q missing 'layout' definition", name)
		}
	}
}

func TestChatTemplateRendersWithData(t *testing.T) {
	tmpls := loadTestTemplateMap(t)

	data := ChatPageData{
		SessionID: "sess-abc",
		Persona:   "socratic_business_analyst",
		Messages: []elicitation.ChatMessage{
			{Role: "user", Content: "Hello", SentAt: time.Now()},
			{Role: "assistant", Content: "Hi there", SentAt: time.Now()},
		},
	}

	var buf bytes.Buffer
	if err := tmpls["chat.html"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("failed to render chat template: %v", err)
	}

	html := buf.String()

	checks := []string{
		"<!DOCTYPE html>",
		"<html",
		"<nav",
		"sess-abc",
		"socratic_business_analyst",
		"<chat-input",
		"<loading-indicator",
		"<chat-message",
		"Synthesize Artifacts",
		"chat-input.js",
		"chat-message.js",
		"loading-indicator.js",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("chat template output missing %q", want)
		}
	}
}

func TestPersonaSelectTemplateRendersWithData(t *testing.T) {
	tmpls := loadTestTemplateMap(t)

	data := PersonaSelectData{
		Personas: defaultPersonas,
	}

	var buf bytes.Buffer
	if err := tmpls["persona_select.html"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("failed to render persona_select template: %v", err)
	}

	html := buf.String()

	checks := []string{
		"<!DOCTYPE html>",
		"<html",
		"Socratic Business Analyst",
		"Hostile Systems Architect",
		"Trusted Advisor",
		"persona-card",
		"btn btn-primary",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("persona_select template output missing %q", want)
		}
	}
}

func TestArtifactsTemplateRendersWithData(t *testing.T) {
	tmpls := loadTestTemplateMap(t)

	data := ArtifactsPageData{
		ProjectID: "proj-123",
		Artifacts: []store.ArtifactRecord{
			{ID: "a1", ProjectID: "proj-123", ArtifactType: "BRD", Content: "Business reqs", CreatedAt: time.Now()},
			{ID: "a2", ProjectID: "proj-123", ArtifactType: "SRS", Content: "Software reqs", CreatedAt: time.Now()},
		},
	}

	var buf bytes.Buffer
	if err := tmpls["artifacts.html"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("failed to render artifacts template: %v", err)
	}

	html := buf.String()

	checks := []string{
		"<!DOCTYPE html>",
		"<html",
		"proj-123",
		"Export All",
		"BRD",
		"SRS",
		"Business reqs",
		"Software reqs",
		"artifact-card",
	}
	for _, want := range checks {
		if !strings.Contains(html, want) {
			t.Errorf("artifacts template output missing %q", want)
		}
	}
}

func TestArtifactsTemplateRendersEmptyState(t *testing.T) {
	tmpls := loadTestTemplateMap(t)

	data := ArtifactsPageData{
		ProjectID: "proj-empty",
		Artifacts: []store.ArtifactRecord{},
	}

	var buf bytes.Buffer
	if err := tmpls["artifacts.html"].ExecuteTemplate(&buf, "layout", data); err != nil {
		t.Fatalf("failed to render artifacts template with empty data: %v", err)
	}

	html := buf.String()
	if !strings.Contains(html, "No artifacts have been synthesized yet") {
		t.Error("expected empty state message in artifacts template")
	}
}

// ============================================================================
// Integration: page handlers render templates via HTTP
// ============================================================================

func newTestHandlerWithTemplates(t *testing.T) (*Handler, elicitation.SessionStore) {
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
	tmpls := loadTestTemplateMap(t)
	h.SetTemplates(tmpls)
	return h, sessions
}

func TestHandleChatPageRendersHTML(t *testing.T) {
	h, sessions := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("GET", "/chat/"+sess.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, sess.ID) {
		t.Error("expected session ID in rendered chat page")
	}
	if !strings.Contains(body, "<chat-input") {
		t.Error("expected <chat-input> component in rendered chat page")
	}
}

func TestHandlePersonaSelectRendersHTML(t *testing.T) {
	h, _ := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/new", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Socratic Business Analyst") {
		t.Error("expected persona name in rendered page")
	}
}

func TestHandleArtifactsPageRendersHTML(t *testing.T) {
	ms := newMockStore()
	projectID := "proj-tmpl"
	ms.artifacts[projectID] = []store.ArtifactRecord{
		{ID: "a1", ProjectID: projectID, ArtifactType: "BRD", Content: "Test BRD", CreatedAt: time.Now()},
	}

	sessions := elicitation.NewInMemorySessionStore()
	h := NewHandler(&mockElicitor{}, sessions, ms, &mockMarkdownCodec{}, t.TempDir())
	tmpls := loadTestTemplateMap(t)
	h.SetTemplates(tmpls)
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/artifacts/"+projectID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Fatalf("expected text/html content type, got %s", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Test BRD") {
		t.Error("expected artifact content in rendered page")
	}
}

func TestHandleChatPageNotFound(t *testing.T) {
	h, _ := newTestHandlerWithTemplates(t)
	mux := setupMux(h)

	req := httptest.NewRequest("GET", "/chat/nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleChatPageNoTemplates(t *testing.T) {
	h, sessions := newTestHandler(t) // no templates set
	mux := setupMux(h)

	sess := sessions.Create(elicitation.PersonaSocraticBA)

	req := httptest.NewRequest("GET", "/chat/"+sess.ID, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when templates not configured, got %d", w.Code)
	}
}
