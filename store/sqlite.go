package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const timeFormat = "2006-01-02T15:04:05Z"

const schemaDDL = `
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    srs_context TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    project_id      TEXT NOT NULL REFERENCES projects(id),
    title           TEXT NOT NULL,
    gherkin_story   TEXT NOT NULL DEFAULT '',
    mitigated_risks TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'TODO'
                    CHECK(status IN ('TODO','PENDING_AGENT','AGENT_RUNNING','PR_READY','FAILED')),
    pr_link         TEXT NOT NULL DEFAULT '',
    agent_logs      TEXT NOT NULL DEFAULT '',
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS artifacts (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL REFERENCES projects(id),
    artifact_type TEXT NOT NULL CHECK(artifact_type IN ('BRD','SRS','NFR')),
    content       TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_artifacts_project_id ON artifacts(project_id);
`

// Store defines the persistence interface for projects, tasks, and artifacts.
type Store interface {
	InitSchema(ctx context.Context) error
	InsertProject(ctx context.Context, p Project) error
	InsertArtifacts(ctx context.Context, projectID string, artifacts []ArtifactRecord) error
	InsertTasks(ctx context.Context, projectID string, tasks []Task) error
	GetProject(ctx context.Context, id string) (Project, error)
	ListArtifacts(ctx context.Context, projectID string) ([]ArtifactRecord, error)
	ListTasks(ctx context.Context, projectID string) ([]Task, error)
	Close() error
}

// SQLiteStore implements Store using modernc.org/sqlite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens a SQLite connection at the given DSN.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Enable foreign keys.
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// InitSchema creates tables if they don't exist.
func (s *SQLiteStore) InitSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schemaDDL)
	if err != nil {
		return fmt.Errorf("init schema: %w", err)
	}
	return nil
}

// InsertProject creates a new project record.
func (s *SQLiteStore) InsertProject(ctx context.Context, p Project) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (id, name, description, srs_context, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, p.SRSContext, p.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

// GetProject retrieves a project by ID.
func (s *SQLiteStore) GetProject(ctx context.Context, id string) (Project, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, srs_context, created_at FROM projects WHERE id = ?`, id)

	var p Project
	var createdAt string
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &p.SRSContext, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return Project{}, fmt.Errorf("project %q not found", id)
		}
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	t, err := time.Parse(timeFormat, createdAt)
	if err != nil {
		return Project{}, fmt.Errorf("parse created_at: %w", err)
	}
	p.CreatedAt = t
	return p, nil
}

// InsertArtifacts inserts artifacts within a transaction; rolls back on error.
func (s *SQLiteStore) InsertArtifacts(ctx context.Context, projectID string, artifacts []ArtifactRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO artifacts (id, project_id, artifact_type, content, created_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert artifact: %w", err)
	}
	defer stmt.Close()

	for _, a := range artifacts {
		_, err := stmt.ExecContext(ctx, a.ID, projectID, a.ArtifactType, a.Content, a.CreatedAt.UTC().Format(timeFormat))
		if err != nil {
			return fmt.Errorf("insert artifact %q: %w", a.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit artifacts: %w", err)
	}
	return nil
}

// ListArtifacts returns all artifacts for a project.
func (s *SQLiteStore) ListArtifacts(ctx context.Context, projectID string) ([]ArtifactRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, artifact_type, content, created_at FROM artifacts WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	var result []ArtifactRecord
	for rows.Next() {
		var a ArtifactRecord
		var createdAt string
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.ArtifactType, &a.Content, &createdAt); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		t, err := time.Parse(timeFormat, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse artifact created_at: %w", err)
		}
		a.CreatedAt = t
		result = append(result, a)
	}
	return result, rows.Err()
}

// InsertTasks inserts task records within a transaction; rolls back on error.
func (s *SQLiteStore) InsertTasks(ctx context.Context, projectID string, tasks []Task) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO tasks (id, project_id, title, gherkin_story, mitigated_risks, status, pr_link, agent_logs, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert task: %w", err)
	}
	defer stmt.Close()

	for _, tk := range tasks {
		_, err := stmt.ExecContext(ctx, tk.ID, projectID, tk.Title, tk.GherkinStory,
			tk.MitigatedRisks, tk.Status, tk.PRLink, tk.AgentLogs, tk.UpdatedAt.UTC().Format(timeFormat))
		if err != nil {
			return fmt.Errorf("insert task %q: %w", tk.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tasks: %w", err)
	}
	return nil
}

// ListTasks returns all tasks for a project.
func (s *SQLiteStore) ListTasks(ctx context.Context, projectID string) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, title, gherkin_story, mitigated_risks, status, pr_link, agent_logs, updated_at
		 FROM tasks WHERE project_id = ?`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var result []Task
	for rows.Next() {
		var tk Task
		var updatedAt string
		if err := rows.Scan(&tk.ID, &tk.ProjectID, &tk.Title, &tk.GherkinStory,
			&tk.MitigatedRisks, &tk.Status, &tk.PRLink, &tk.AgentLogs, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan task: %w", err)
		}
		t, err := time.Parse(timeFormat, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse task updated_at: %w", err)
		}
		tk.UpdatedAt = t
		result = append(result, tk)
	}
	return result, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
