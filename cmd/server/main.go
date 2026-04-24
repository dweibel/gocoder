package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardp/coding-agent/agent"
	"github.com/ardp/coding-agent/elicitation"
	"github.com/ardp/coding-agent/store"
	"github.com/ardp/coding-agent/web"
)

func main() {
	// Load and validate config.
	cfg := elicitation.LoadElicitationConfig()
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %s\n", err)
		os.Exit(1)
	}

	// Database path (default: ./ardp.db).
	dbPath := os.Getenv("ARDP_DB_PATH")
	if dbPath == "" {
		dbPath = "./ardp.db"
	}

	// Initialize SQLite store and schema.
	sqlStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %s\n", err)
		os.Exit(1)
	}
	defer sqlStore.Close()

	ctx := context.Background()
	if err := sqlStore.InitSchema(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to init schema: %s\n", err)
		os.Exit(1)
	}

	// Prompts directory (default: ./prompts).
	promptsDir := os.Getenv("ARDP_PROMPTS_DIR")
	if promptsDir == "" {
		promptsDir = "prompts"
	}

	// Initialize dependencies.
	personaLoader := elicitation.NewFilePersonaLoader(promptsDir)
	messageCreator := agent.NewOpenRouterClient(cfg)
	engine := elicitation.NewEngine(messageCreator, cfg, personaLoader)
	engine.SetPromptsDir(promptsDir)

	sessions := elicitation.NewInMemorySessionStore()
	mdCodec := elicitation.NewMarkdownCodec()

	savePath := os.Getenv("ARDP_SAVE_PATH")
	if savePath == "" {
		savePath = "testdata/conversations"
	}

	handler := web.NewHandler(engine, sessions, sqlStore, mdCodec, savePath)

	// Parse and set templates.
	templatesDir := os.Getenv("ARDP_TEMPLATES_DIR")
	if templatesDir == "" {
		templatesDir = "web/templates"
	}
	tmpls, err := web.ParseTemplates(templatesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse templates: %s\n", err)
		os.Exit(1)
	}
	handler.SetTemplates(tmpls)

	// Register routes.
	mux := http.NewServeMux()
	web.RegisterRoutes(mux, handler)

	// Health check endpoint.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Server port (default: 8080).
	port := os.Getenv("ARDP_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Start server in a goroutine.
	go func() {
		log.Printf("server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %s", err)
		}
	}()

	// Wait for interrupt signal for graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("server shutdown error: %s", err)
	}

	log.Println("server stopped")
}
