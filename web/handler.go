package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ardp/coding-agent/elicitation"
	"github.com/ardp/coding-agent/store"
)

// Handler provides HTTP handlers for the elicitation engine API.
type Handler struct {
	engine   elicitation.Elicitor
	sessions elicitation.SessionStore
	store    store.Store
	mdCodec  elicitation.ConversationMarshaler
	tmpls    map[string]*template.Template
	savePath string // Base directory for conversation markdown files
}

// ChatPageData holds data for rendering the chat template.
type ChatPageData struct {
	SessionID   string
	SessionName string
	Persona     string
	PersonaName string
	Messages    []elicitation.ChatMessage
	AllPersonas []PersonaOption
}

// LandingPageData holds data for rendering the landing page template.
type LandingPageData struct {
	Sessions []elicitation.SessionSummary
}

// PersonaOption describes a persona choice for the selection UI.
type PersonaOption struct {
	ID          string
	Name        string
	Description string
}

// PersonaSelectData holds data for rendering the persona selection template.
type PersonaSelectData struct {
	Personas []PersonaOption
}

// ArtifactsPageData holds data for rendering the artifacts template.
type ArtifactsPageData struct {
	ProjectID string
	Artifacts []store.ArtifactRecord
}

// defaultPersonas defines the three persona options shown on the selection page.
var defaultPersonas = []PersonaOption{
	{
		ID:          string(elicitation.PersonaSocraticBA),
		Name:        "Socratic Business Analyst",
		Description: "Uses guided questioning to uncover hidden assumptions, stakeholder needs, and edge cases through collaborative dialogue.",
	},
	{
		ID:          string(elicitation.PersonaHostileSA),
		Name:        "Hostile Systems Architect",
		Description: "Aggressively challenges proposals by probing for architectural weaknesses, failure modes, and scalability gaps.",
	},
	{
		ID:          string(elicitation.PersonaTrustedAdv),
		Name:        "Trusted Advisor",
		Description: "Gently fills in ambiguities and questions decisions in a supportive, non-confrontational way.",
	},
}

// NewHandler creates a new Handler with the given dependencies.
// The tmpl field can be nil (templates will be added via SetTemplates).
func NewHandler(engine elicitation.Elicitor, sessions elicitation.SessionStore, st store.Store, mdCodec elicitation.ConversationMarshaler, savePath string) *Handler {
	return &Handler{
		engine:   engine,
		sessions: sessions,
		store:    st,
		mdCodec:  mdCodec,
		savePath: savePath,
	}
}

// SetTemplates sets the parsed HTML templates for page rendering.
// Each key is a page name (e.g. "chat.html") mapped to a template
// that includes both the layout and the page-specific definitions.
func (h *Handler) SetTemplates(tmpls map[string]*template.Template) {
	h.tmpls = tmpls
}

// ParseTemplates parses the layout and each page template from the given
// directory, returning a map suitable for SetTemplates.
func ParseTemplates(dir string) (map[string]*template.Template, error) {
	layoutFile := filepath.Join(dir, "layout.html")
	pages := []string{"chat.html", "persona_select.html", "artifacts.html", "landing.html"}

	tmpls := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		t, err := template.ParseFiles(layoutFile, filepath.Join(dir, page))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", page, err)
		}
		tmpls[page] = t
	}
	return tmpls, nil
}

// jsonError writes a structured JSON error response.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// jsonOK writes a JSON success response.
func jsonOK(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// HandleNewSession creates a new elicitation session.
// POST /api/sessions
// Accepts JSON {"persona": "..."} or form-encoded persona=...
// On success from a browser form, redirects to the chat page.
// On success from JSON API, returns {"session_id": "..."}.
func (h *Handler) HandleNewSession(w http.ResponseWriter, r *http.Request) {
	var persona string

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		var req struct {
			Persona string `json:"persona"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		persona = req.Persona
	} else {
		// Form-encoded (default HTML form submission)
		if err := r.ParseForm(); err != nil {
			jsonError(w, "invalid form data: "+err.Error(), http.StatusBadRequest)
			return
		}
		persona = r.FormValue("persona")
	}

	if persona == "" {
		persona = string(elicitation.PersonaTrustedAdv)
	}

	sess := h.sessions.Create(elicitation.PersonaType(persona))

	// If the request came from a browser form, redirect to the chat page
	if !strings.HasPrefix(ct, "application/json") {
		http.Redirect(w, r, "/chat/"+sess.ID, http.StatusSeeOther)
		return
	}

	jsonOK(w, map[string]string{"session_id": sess.ID})
}

// HandleChat sends a user message and returns the assistant response.
// POST /api/sessions/{id}/messages
// Request: {"content": "user message"}
// Response: {"role": "assistant", "content": "...", "sent_at": "..."}
func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		jsonError(w, "missing required field: content", http.StatusBadRequest)
		return
	}

	resp, err := h.engine.Chat(r.Context(), sess, req.Content)
	if err != nil {
		if errors.Is(err, elicitation.ErrLLMTimeout) {
			jsonError(w, "LLM request timed out", http.StatusGatewayTimeout)
			return
		}
		jsonError(w, "chat failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, resp)
}

// HandleSynthesize generates artifacts from the session's chat history.
// POST /api/sessions/{id}/synthesize
// Response: {"artifacts": [...]}
func (h *Handler) HandleSynthesize(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	artifacts, err := h.engine.Synthesize(r.Context(), sess)
	if err != nil {
		if errors.Is(err, elicitation.ErrLLMTimeout) {
			jsonError(w, "LLM request timed out", http.StatusGatewayTimeout)
			return
		}
		if errors.Is(err, elicitation.ErrSynthesisParse) {
			jsonError(w, "synthesis parse failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		jsonError(w, "synthesis failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{"artifacts": artifacts})
}

// HandleSaveSession saves the session to a markdown file.
// POST /api/sessions/{id}/save
// Response: {"path": "..."}
func (h *Handler) HandleSaveSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data, err := h.mdCodec.Marshal(sess)
	if err != nil {
		jsonError(w, "failed to marshal session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s.md", sessionID)
	filePath := filepath.Join(h.savePath, filename)

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		jsonError(w, "failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		jsonError(w, "failed to write file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{"path": filePath})
}

// HandleLoadSession loads a session from a markdown file.
// POST /api/sessions/load
// Request: {"path": "path/to/file.md"}
// Response: {"session_id": "..."}
func (h *Handler) HandleLoadSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		jsonError(w, "missing required field: path", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(req.Path)
	if err != nil {
		jsonError(w, "failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}

	sess, err := h.mdCodec.Unmarshal(data)
	if err != nil {
		jsonError(w, "failed to parse markdown: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.sessions.Put(sess)

	jsonOK(w, map[string]string{"session_id": sess.ID})
}

// HandleListArtifacts returns all artifacts for a project.
// GET /api/projects/{id}/artifacts
// Response: {"artifacts": [...]}
func (h *Handler) HandleListArtifacts(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	artifacts, err := h.store.ListArtifacts(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to list artifacts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]interface{}{"artifacts": artifacts})
}

// HandleExportArtifacts exports all artifacts for a project as markdown.
// GET /api/projects/{id}/export
// Response: markdown text content
func (h *Handler) HandleExportArtifacts(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")

	artifacts, err := h.store.ListArtifacts(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to list artifacts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Project %s — Artifacts Export\n\n", projectID))

	for _, a := range artifacts {
		sb.WriteString(fmt.Sprintf("## %s\n\n", a.ArtifactType))
		sb.WriteString(a.Content)
		sb.WriteString("\n\n")
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(sb.String()))
}

// HandleChatPage renders the chat page for a given session.
// GET /chat/{id}
func (h *Handler) HandleChatPage(w http.ResponseWriter, r *http.Request) {
	if h.tmpls == nil {
		jsonError(w, "templates not configured", http.StatusInternalServerError)
		return
	}

	sessionID := r.PathValue("id")
	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := ChatPageData{
		SessionID:   sess.ID,
		SessionName: sess.Name,
		Persona:     string(sess.Persona),
		PersonaName: sess.Persona.DisplayName(),
		Messages:    sess.Messages,
		AllPersonas: defaultPersonas,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpls["chat.html"].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// HandlePersonaSelect renders the persona selection page.
// GET /new
func (h *Handler) HandlePersonaSelect(w http.ResponseWriter, r *http.Request) {
	if h.tmpls == nil {
		jsonError(w, "templates not configured", http.StatusInternalServerError)
		return
	}

	data := PersonaSelectData{
		Personas: defaultPersonas,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpls["persona_select.html"].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// HandleArtifactsPage renders the artifacts page for a given project.
// GET /artifacts/{projectID}
func (h *Handler) HandleArtifactsPage(w http.ResponseWriter, r *http.Request) {
	if h.tmpls == nil {
		jsonError(w, "templates not configured", http.StatusInternalServerError)
		return
	}

	projectID := r.PathValue("id")

	artifacts, err := h.store.ListArtifacts(r.Context(), projectID)
	if err != nil {
		jsonError(w, "failed to list artifacts: "+err.Error(), http.StatusInternalServerError)
		return
	}

	data := ArtifactsPageData{
		ProjectID: projectID,
		Artifacts: artifacts,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpls["artifacts.html"].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}


// validPersonas is the set of accepted persona identifiers.
var validPersonas = map[elicitation.PersonaType]bool{
	elicitation.PersonaSocraticBA: true,
	elicitation.PersonaHostileSA:  true,
	elicitation.PersonaTrustedAdv: true,
}

// HandleLandingPage renders the landing page with session list.
// GET /
func (h *Handler) HandleLandingPage(w http.ResponseWriter, r *http.Request) {
	if h.tmpls == nil {
		jsonError(w, "templates not configured", http.StatusInternalServerError)
		return
	}

	sessions := h.sessions.List()
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})

	data := LandingPageData{
		Sessions: sessions,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpls["landing.html"].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// HandleListSessions returns all sessions as JSON.
// GET /api/sessions
func (h *Handler) HandleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := h.sessions.List()
	if sessions == nil {
		sessions = []elicitation.SessionSummary{}
	}
	jsonOK(w, sessions)
}

// HandleUpdatePersona changes the active persona for a session.
// PUT /api/sessions/{id}/persona
// Request: {"persona": "hostile_systems_architect"}
// Response: {"persona": "hostile_systems_architect", "display_name": "Hostile Systems Architect"}
func (h *Handler) HandleUpdatePersona(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Persona string `json:"persona"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	newPersona := elicitation.PersonaType(req.Persona)
	if !validPersonas[newPersona] {
		jsonError(w, "invalid persona: "+req.Persona, http.StatusBadRequest)
		return
	}

	sess.Persona = newPersona
	sess.Messages = append(sess.Messages, elicitation.ChatMessage{
		Role:    "system",
		Content: "Switched to " + newPersona.DisplayName(),
		SentAt:  time.Now(),
	})

	jsonOK(w, map[string]string{
		"persona":      req.Persona,
		"display_name": newPersona.DisplayName(),
	})
}

// HandleUpdateName changes the name of a session.
// PUT /api/sessions/{id}/name
// Request: {"name": "My Feature Discussion"}
// Response: {"name": "My Feature Discussion"}
func (h *Handler) HandleUpdateName(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		if errors.Is(err, elicitation.ErrSessionNotFound) {
			jsonError(w, "session not found: "+sessionID, http.StatusNotFound)
			return
		}
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := elicitation.ValidateSessionName(req.Name); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	sess.Name = strings.TrimSpace(req.Name)

	jsonOK(w, map[string]string{"name": sess.Name})
}
