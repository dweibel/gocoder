package store

import (
	"context"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// ============================================================================
// Task 11.1: Unit tests for SQLiteStore schema initialization and CRUD
// Requirements: 8.1, 8.2, 8.3, 8.4, 8.5, 8.6
// ============================================================================

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	ctx := context.Background()
	if err := s.InitSchema(ctx); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInitSchema_CreatesTables(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Query sqlite_master for the three expected tables.
	tables := map[string]bool{"projects": false, "tasks": false, "artifacts": false}
	rows, err := s.db.QueryContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name IN ('projects','tasks','artifacts')`)
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables[name] = true
	}
	for tbl, found := range tables {
		if !found {
			t.Errorf("table %q not created by InitSchema", tbl)
		}
	}
}

func TestInitSchema_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Calling InitSchema a second time should not error.
	if err := s.InitSchema(ctx); err != nil {
		t.Fatalf("second InitSchema call failed: %v", err)
	}
}

func TestInsertProject_GetProject_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{
		ID:          "proj-001",
		Name:        "Test Project",
		Description: "A test project description",
		SRSContext:  "Some SRS context",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}

	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	got, err := s.GetProject(ctx, "proj-001")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}

	if got.ID != p.ID {
		t.Errorf("ID: got %q, want %q", got.ID, p.ID)
	}
	if got.Name != p.Name {
		t.Errorf("Name: got %q, want %q", got.Name, p.Name)
	}
	if got.Description != p.Description {
		t.Errorf("Description: got %q, want %q", got.Description, p.Description)
	}
	if got.SRSContext != p.SRSContext {
		t.Errorf("SRSContext: got %q, want %q", got.SRSContext, p.SRSContext)
	}
	if !got.CreatedAt.Equal(p.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, p.CreatedAt)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetProject(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent project, got nil")
	}
}

func TestInsertArtifacts_ListArtifacts_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert a project first (foreign key).
	p := Project{ID: "proj-art", Name: "Art Project", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	artifacts := []ArtifactRecord{
		{ID: "art-1", ProjectID: "proj-art", ArtifactType: "BRD", Content: "BRD content", CreatedAt: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "art-2", ProjectID: "proj-art", ArtifactType: "SRS", Content: "SRS content", CreatedAt: time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)},
		{ID: "art-3", ProjectID: "proj-art", ArtifactType: "NFR", Content: "NFR content", CreatedAt: time.Date(2025, 2, 3, 0, 0, 0, 0, time.UTC)},
	}

	if err := s.InsertArtifacts(ctx, "proj-art", artifacts); err != nil {
		t.Fatalf("InsertArtifacts: %v", err)
	}

	got, err := s.ListArtifacts(ctx, "proj-art")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 artifacts, got %d", len(got))
	}

	// Build a map for easy lookup.
	byID := make(map[string]ArtifactRecord)
	for _, a := range got {
		byID[a.ID] = a
	}

	for _, want := range artifacts {
		a, ok := byID[want.ID]
		if !ok {
			t.Errorf("artifact %q not found", want.ID)
			continue
		}
		if a.ArtifactType != want.ArtifactType {
			t.Errorf("artifact %q: ArtifactType got %q, want %q", want.ID, a.ArtifactType, want.ArtifactType)
		}
		if a.Content != want.Content {
			t.Errorf("artifact %q: Content got %q, want %q", want.ID, a.Content, want.Content)
		}
		if !a.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("artifact %q: CreatedAt got %v, want %v", want.ID, a.CreatedAt, want.CreatedAt)
		}
	}
}

func TestInsertTasks_ListTasks_RoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{ID: "proj-task", Name: "Task Project", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	tasks := []Task{
		{
			ID: "task-1", ProjectID: "proj-task", Title: "First task",
			GherkinStory: "Given X When Y Then Z", MitigatedRisks: "risk-a",
			Status: "TODO", PRLink: "", AgentLogs: "",
			UpdatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID: "task-2", ProjectID: "proj-task", Title: "Second task",
			GherkinStory: "Given A When B Then C", MitigatedRisks: "",
			Status: "PENDING_AGENT", PRLink: "https://pr/1", AgentLogs: "log data",
			UpdatedAt: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := s.InsertTasks(ctx, "proj-task", tasks); err != nil {
		t.Fatalf("InsertTasks: %v", err)
	}

	got, err := s.ListTasks(ctx, "proj-task")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}

	byID := make(map[string]Task)
	for _, tk := range got {
		byID[tk.ID] = tk
	}

	for _, want := range tasks {
		tk, ok := byID[want.ID]
		if !ok {
			t.Errorf("task %q not found", want.ID)
			continue
		}
		if tk.Title != want.Title {
			t.Errorf("task %q: Title got %q, want %q", want.ID, tk.Title, want.Title)
		}
		if tk.GherkinStory != want.GherkinStory {
			t.Errorf("task %q: GherkinStory got %q, want %q", want.ID, tk.GherkinStory, want.GherkinStory)
		}
		if tk.MitigatedRisks != want.MitigatedRisks {
			t.Errorf("task %q: MitigatedRisks got %q, want %q", want.ID, tk.MitigatedRisks, want.MitigatedRisks)
		}
		if tk.Status != want.Status {
			t.Errorf("task %q: Status got %q, want %q", want.ID, tk.Status, want.Status)
		}
		if tk.PRLink != want.PRLink {
			t.Errorf("task %q: PRLink got %q, want %q", want.ID, tk.PRLink, want.PRLink)
		}
		if tk.AgentLogs != want.AgentLogs {
			t.Errorf("task %q: AgentLogs got %q, want %q", want.ID, tk.AgentLogs, want.AgentLogs)
		}
		if !tk.UpdatedAt.Equal(want.UpdatedAt) {
			t.Errorf("task %q: UpdatedAt got %v, want %v", want.ID, tk.UpdatedAt, want.UpdatedAt)
		}
	}
}

func TestTransactionRollback_OnInjectedFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{ID: "proj-rollback", Name: "Rollback Project", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	// Insert artifacts where the second one has an invalid artifact_type.
	// The CHECK constraint should cause the transaction to fail and roll back.
	badArtifacts := []ArtifactRecord{
		{ID: "art-ok", ProjectID: "proj-rollback", ArtifactType: "BRD", Content: "good", CreatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "art-bad", ProjectID: "proj-rollback", ArtifactType: "INVALID_TYPE", Content: "bad", CreatedAt: time.Now().UTC().Truncate(time.Second)},
	}

	err := s.InsertArtifacts(ctx, "proj-rollback", badArtifacts)
	if err == nil {
		t.Fatal("expected error from invalid artifact_type, got nil")
	}

	// Verify no partial data was committed.
	got, err := s.ListArtifacts(ctx, "proj-rollback")
	if err != nil {
		t.Fatalf("ListArtifacts after rollback: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 artifacts after rollback, got %d", len(got))
	}
}

func TestTransactionRollback_Tasks_OnInjectedFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{ID: "proj-task-rb", Name: "Task Rollback", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	// Insert tasks where the second has an invalid status.
	// The CHECK constraint should cause the transaction to fail and roll back.
	badTasks := []Task{
		{ID: "tk-ok", ProjectID: "proj-task-rb", Title: "Good task", Status: "TODO", UpdatedAt: time.Now().UTC().Truncate(time.Second)},
		{ID: "tk-bad", ProjectID: "proj-task-rb", Title: "Bad task", Status: "INVALID_STATUS", UpdatedAt: time.Now().UTC().Truncate(time.Second)},
	}

	err := s.InsertTasks(ctx, "proj-task-rb", badTasks)
	if err == nil {
		t.Fatal("expected error from invalid status, got nil")
	}

	got, err := s.ListTasks(ctx, "proj-task-rb")
	if err != nil {
		t.Fatalf("ListTasks after rollback: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 tasks after rollback, got %d", len(got))
	}
}

func TestListArtifacts_EmptyProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{ID: "proj-empty", Name: "Empty", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	got, err := s.ListArtifacts(ctx, "proj-empty")
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 artifacts, got %d", len(got))
	}
}

func TestListTasks_EmptyProject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	p := Project{ID: "proj-empty-t", Name: "Empty Tasks", CreatedAt: time.Now().UTC().Truncate(time.Second)}
	if err := s.InsertProject(ctx, p); err != nil {
		t.Fatalf("InsertProject: %v", err)
	}

	got, err := s.ListTasks(ctx, "proj-empty-t")
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(got))
	}
}

// ============================================================================
// Task 11.2: Property test — Database persistence round-trip
// Feature: elicitation-engine, Property 10: Database persistence round-trip
// Validates: Requirements 8.1, 8.2, 8.3, 8.4
// ============================================================================

func genProject(t *rapid.T) Project {
	return Project{
		ID:          rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "projectID"),
		Name:        rapid.StringMatching(`[A-Za-z ]{1,50}`).Draw(t, "name"),
		Description: rapid.StringMatching(`[A-Za-z0-9 .,]{0,100}`).Draw(t, "description"),
		SRSContext:  rapid.StringMatching(`[A-Za-z0-9 .,]{0,100}`).Draw(t, "srsContext"),
		CreatedAt:   time.Date(2025, 1, rapid.IntRange(1, 28).Draw(t, "day"), 10, 0, 0, 0, time.UTC),
	}
}

func genArtifactRecord(t *rapid.T, projectID string) ArtifactRecord {
	types := []string{"BRD", "SRS", "NFR"}
	aType := types[rapid.IntRange(0, 2).Draw(t, "artTypeIdx")]
	return ArtifactRecord{
		ID:           rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "artifactID"),
		ProjectID:    projectID,
		ArtifactType: aType,
		Content:      rapid.StringMatching(`[A-Za-z0-9 .,\n]{1,200}`).Draw(t, "content"),
		CreatedAt:    time.Date(2025, 2, rapid.IntRange(1, 28).Draw(t, "artDay"), 12, 0, 0, 0, time.UTC),
	}
}

func genTask(t *rapid.T, projectID string) Task {
	statuses := []string{"TODO", "PENDING_AGENT", "AGENT_RUNNING", "PR_READY", "FAILED"}
	status := statuses[rapid.IntRange(0, 4).Draw(t, "statusIdx")]
	return Task{
		ID:             rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "taskID"),
		ProjectID:      projectID,
		Title:          rapid.StringMatching(`[A-Za-z ]{1,50}`).Draw(t, "title"),
		GherkinStory:   rapid.StringMatching(`[A-Za-z0-9 ]{0,100}`).Draw(t, "gherkin"),
		MitigatedRisks: rapid.StringMatching(`[A-Za-z0-9 ]{0,50}`).Draw(t, "risks"),
		Status:         status,
		PRLink:         rapid.StringMatching(`[a-z0-9:/]{0,30}`).Draw(t, "prLink"),
		AgentLogs:      rapid.StringMatching(`[A-Za-z0-9 ]{0,50}`).Draw(t, "logs"),
		UpdatedAt:      time.Date(2025, 3, rapid.IntRange(1, 28).Draw(t, "taskDay"), 8, 0, 0, 0, time.UTC),
	}
}

func TestDBPersistenceRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s, err := NewSQLiteStore(":memory:")
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}
		defer s.Close()

		ctx := context.Background()
		if err := s.InitSchema(ctx); err != nil {
			t.Fatalf("InitSchema: %v", err)
		}

		// Generate and insert a project.
		proj := genProject(t)
		if err := s.InsertProject(ctx, proj); err != nil {
			t.Fatalf("InsertProject: %v", err)
		}

		gotProj, err := s.GetProject(ctx, proj.ID)
		if err != nil {
			t.Fatalf("GetProject: %v", err)
		}
		if gotProj.ID != proj.ID || gotProj.Name != proj.Name ||
			gotProj.Description != proj.Description || gotProj.SRSContext != proj.SRSContext ||
			!gotProj.CreatedAt.Equal(proj.CreatedAt) {
			t.Fatalf("Project round-trip mismatch:\n  want: %+v\n  got:  %+v", proj, gotProj)
		}

		// Generate and insert artifacts.
		numArt := rapid.IntRange(1, 5).Draw(t, "numArtifacts")
		artifacts := make([]ArtifactRecord, numArt)
		artIDs := make(map[string]bool)
		for i := 0; i < numArt; i++ {
			a := genArtifactRecord(t, proj.ID)
			// Ensure unique IDs within this batch.
			for artIDs[a.ID] {
				a.ID = rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "retryArtID")
			}
			artIDs[a.ID] = true
			artifacts[i] = a
		}
		if err := s.InsertArtifacts(ctx, proj.ID, artifacts); err != nil {
			t.Fatalf("InsertArtifacts: %v", err)
		}

		gotArts, err := s.ListArtifacts(ctx, proj.ID)
		if err != nil {
			t.Fatalf("ListArtifacts: %v", err)
		}
		if len(gotArts) != numArt {
			t.Fatalf("expected %d artifacts, got %d", numArt, len(gotArts))
		}

		artByID := make(map[string]ArtifactRecord)
		for _, a := range gotArts {
			artByID[a.ID] = a
		}
		for _, want := range artifacts {
			got, ok := artByID[want.ID]
			if !ok {
				t.Fatalf("artifact %q not found after round-trip", want.ID)
			}
			if got.ArtifactType != want.ArtifactType || got.Content != want.Content ||
				!got.CreatedAt.Equal(want.CreatedAt) {
				t.Fatalf("Artifact round-trip mismatch for %q:\n  want: %+v\n  got:  %+v", want.ID, want, got)
			}
		}

		// Generate and insert tasks.
		numTasks := rapid.IntRange(1, 5).Draw(t, "numTasks")
		tasks := make([]Task, numTasks)
		taskIDs := make(map[string]bool)
		for i := 0; i < numTasks; i++ {
			tk := genTask(t, proj.ID)
			for taskIDs[tk.ID] {
				tk.ID = rapid.StringMatching(`[a-z0-9]{8}`).Draw(t, "retryTaskID")
			}
			taskIDs[tk.ID] = true
			tasks[i] = tk
		}
		if err := s.InsertTasks(ctx, proj.ID, tasks); err != nil {
			t.Fatalf("InsertTasks: %v", err)
		}

		gotTasks, err := s.ListTasks(ctx, proj.ID)
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(gotTasks) != numTasks {
			t.Fatalf("expected %d tasks, got %d", numTasks, len(gotTasks))
		}

		taskByID := make(map[string]Task)
		for _, tk := range gotTasks {
			taskByID[tk.ID] = tk
		}
		for _, want := range tasks {
			got, ok := taskByID[want.ID]
			if !ok {
				t.Fatalf("task %q not found after round-trip", want.ID)
			}
			if got.Title != want.Title || got.GherkinStory != want.GherkinStory ||
				got.MitigatedRisks != want.MitigatedRisks || got.Status != want.Status ||
				got.PRLink != want.PRLink || got.AgentLogs != want.AgentLogs ||
				!got.UpdatedAt.Equal(want.UpdatedAt) {
				t.Fatalf("Task round-trip mismatch for %q:\n  want: %+v\n  got:  %+v", want.ID, want, got)
			}
		}
	})
}
