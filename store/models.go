package store

import "time"

// Project represents a software project in the database.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SRSContext  string    `json:"srs_context"`
	CreatedAt   time.Time `json:"created_at"`
}

// Task represents a unit of work within a project.
type Task struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"project_id"`
	Title          string    `json:"title"`
	GherkinStory   string    `json:"gherkin_story"`
	MitigatedRisks string    `json:"mitigated_risks"`
	Status         string    `json:"status"`
	PRLink         string    `json:"pr_link"`
	AgentLogs      string    `json:"agent_logs"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ArtifactRecord represents a persisted artifact in the database.
type ArtifactRecord struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	ArtifactType string    `json:"artifact_type"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"created_at"`
}
