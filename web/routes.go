package web

import (
	"net/http"
	"os"
)

// RegisterRoutes registers all API routes, page routes, and static file
// serving on the given ServeMux. Uses Go 1.22+ method-based routing.
func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	// API endpoints
	mux.HandleFunc("POST /api/sessions", h.HandleNewSession)
	mux.HandleFunc("POST /api/sessions/{id}/messages", h.HandleChat)
	mux.HandleFunc("POST /api/sessions/{id}/synthesize", h.HandleSynthesize)
	mux.HandleFunc("POST /api/sessions/{id}/save", h.HandleSaveSession)
	mux.HandleFunc("POST /api/sessions/load", h.HandleLoadSession)
	mux.HandleFunc("GET /api/projects/{id}/artifacts", h.HandleListArtifacts)
	mux.HandleFunc("GET /api/projects/{id}/export", h.HandleExportArtifacts)
	mux.HandleFunc("GET /api/sessions", h.HandleListSessions)
	mux.HandleFunc("PUT /api/sessions/{id}/persona", h.HandleUpdatePersona)
	mux.HandleFunc("PUT /api/sessions/{id}/name", h.HandleUpdateName)

	// Page routes (templates will be implemented in task 13)
	mux.HandleFunc("GET /{$}", h.HandleLandingPage)
	mux.HandleFunc("GET /new", h.HandlePersonaSelect)
	mux.HandleFunc("GET /chat/{id}", h.HandleChatPage)
	mux.HandleFunc("GET /artifacts/{id}", h.HandleArtifactsPage)

	// Static assets — use ARDP_STATIC_DIR env var, fall back to web/static
	staticDir := os.Getenv("ARDP_STATIC_DIR")
	if staticDir == "" {
		staticDir = "web/static"
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))
}
