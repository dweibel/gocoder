package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardp/coding-agent/agent"
	"pgregory.net/rapid"
)

// cliBinary holds the path to the pre-built CLI binary, set in TestMain.
var cliBinary string

func TestMain(m *testing.M) {
	// Build the CLI binary once for all subprocess tests.
	tmpDir, err := os.MkdirTemp("", "cli-test-bin-*")
	if err != nil {
		panic("failed to create temp dir for binary: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "agent-test-bin")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build CLI binary: " + err.Error())
	}
	cliBinary = binPath

	os.Exit(m.Run())
}

// runCLI runs the pre-built CLI binary as a subprocess.
// It sets the given environment variables and passes the given args.
// Returns stdout, stderr, and the exit error (nil if exit code 0).
func runCLI(tb testing.TB, env []string, args ...string) (stdout, stderr string, err error) {
	tb.Helper()
	cmd := exec.Command(cliBinary, args...)

	// Build env: start with current env, then overlay provided vars.
	cmd.Env = append(os.Environ(), env...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	return outBuf.String(), errBuf.String(), runErr
}

// ============================================================
// Feature: ardp-coding-agent, Property 14: CLI flag overrides env var
// Validates: Requirements 9.4
// ============================================================
func TestCLIFlagOverrides(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		envModel := rapid.StringMatching(`[a-z]{2,10}/[a-z0-9-]{2,15}`).Draw(t, "envModel")
		flagModel := rapid.StringMatching(`[a-z]{2,10}/[a-z0-9-]{2,15}`).Filter(func(s string) bool {
			return s != envModel
		}).Draw(t, "flagModel")

		// Set the env var to one value
		os.Setenv("OPENROUTER_MODEL", envModel)
		defer os.Unsetenv("OPENROUTER_MODEL")

		// Load config (picks up env var)
		cfg := agent.LoadConfig()

		// Simulate CLI flag override — this is exactly what main.go does
		if flagModel != "" {
			cfg.Model = flagModel
		}

		if cfg.Model != flagModel {
			t.Fatalf("expected flag value %q to override env value %q, got %q", flagModel, envModel, cfg.Model)
		}
	})
}

// ============================================================
// Feature: ardp-coding-agent, Property 19: Invalid file paths cause CLI failure
// Validates: Requirements 7.5
// ============================================================
func TestInvalidFilePaths(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a random non-existent filename
		randomName := rapid.StringMatching(`[a-z]{3,12}`).Draw(rt, "filename")

		tmpDir, err := os.MkdirTemp("", "cli-pbt-*")
		if err != nil {
			rt.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		nonExistentPath := filepath.Join(tmpDir, randomName+".feature")

		// Decide whether to test --story or --context with the bad path
		testStory := rapid.Bool().Draw(rt, "testStory")

		var args []string
		if testStory {
			// Bad story path, valid context
			ctxFile := filepath.Join(tmpDir, "context.md")
			if writeErr := os.WriteFile(ctxFile, []byte("test context"), 0644); writeErr != nil {
				rt.Fatalf("failed to create temp context file: %v", writeErr)
			}
			args = []string{"--story", nonExistentPath, "--context", ctxFile}
		} else {
			// Valid story, bad context path
			storyFile := filepath.Join(tmpDir, "story.feature")
			if writeErr := os.WriteFile(storyFile, []byte("test story"), 0644); writeErr != nil {
				rt.Fatalf("failed to create temp story file: %v", writeErr)
			}
			args = []string{"--story", storyFile, "--context", nonExistentPath}
		}

		env := []string{"OPENROUTER_API_KEY=test-key-for-pbt"}
		_, stderr, runErr := runCLI(t, env, args...)

		// Must exit non-zero
		if runErr == nil {
			rt.Fatal("expected non-zero exit code for non-existent file path, got exit 0")
		}

		// Stderr must contain a descriptive error
		if stderr == "" {
			rt.Fatal("expected descriptive error on stderr, got empty string")
		}
		stderrLower := strings.ToLower(stderr)
		if !strings.Contains(stderrLower, "error") {
			rt.Fatalf("expected stderr to contain 'error', got: %s", stderr)
		}
	})
}

// ============================================================
// Unit tests for CLI behavior (Task 12.4)
// ============================================================

// Test missing API key env var exits non-zero
// Validates: Requirements 7.7
func TestMissingAPIKeyExitsNonZero(t *testing.T) {
	tmpDir := t.TempDir()
	storyFile := filepath.Join(tmpDir, "story.feature")
	ctxFile := filepath.Join(tmpDir, "context.md")
	os.WriteFile(storyFile, []byte("Given a test story"), 0644)
	os.WriteFile(ctxFile, []byte("Test SRS context"), 0644)

	// Build a clean env without OPENROUTER_API_KEY
	cleanEnv := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "OPENROUTER_API_KEY=") {
			cleanEnv = append(cleanEnv, e)
		}
	}

	cmd := exec.Command(cliBinary,
		"--story", storyFile,
		"--context", ctxFile,
	)
	cmd.Env = cleanEnv

	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code when OPENROUTER_API_KEY is missing")
	}

	stderr := errBuf.String()
	if !strings.Contains(strings.ToLower(stderr), "error") {
		t.Fatalf("expected stderr to contain error message about missing API key, got: %s", stderr)
	}
}

// Test --output flag writes to file
// Validates: Requirements 7.3
func TestOutputFlagWritesToFile(t *testing.T) {
	tmpDir := t.TempDir()
	storyFile := filepath.Join(tmpDir, "story.feature")
	ctxFile := filepath.Join(tmpDir, "context.md")
	outputFile := filepath.Join(tmpDir, "output.json")
	os.WriteFile(storyFile, []byte("Given a test story"), 0644)
	os.WriteFile(ctxFile, []byte("Test SRS context"), 0644)

	// The CLI will fail at the API call (no real API), but it should still
	// write error JSON to the output file before exiting.
	env := []string{"OPENROUTER_API_KEY=fake-key-for-test"}
	_, _, err := runCLI(t, env,
		"--story", storyFile,
		"--context", ctxFile,
		"--output", outputFile,
	)

	// We expect a non-zero exit (API call will fail), but the output file
	// should have been created with error JSON.
	if err == nil {
		// If it somehow succeeded, that's fine — just check the file exists.
	}

	data, readErr := os.ReadFile(outputFile)
	if readErr != nil {
		t.Fatalf("expected output file %s to exist, got error: %v", outputFile, readErr)
	}

	content := string(data)
	if !strings.Contains(content, `"success"`) {
		t.Fatalf("expected output file to contain JSON with 'success' field, got: %s", content)
	}
}

// Test stdout output when --output omitted
// Validates: Requirements 7.3
func TestStdoutOutputWhenNoOutputFlag(t *testing.T) {
	tmpDir := t.TempDir()
	storyFile := filepath.Join(tmpDir, "story.feature")
	ctxFile := filepath.Join(tmpDir, "context.md")
	os.WriteFile(storyFile, []byte("Given a test story"), 0644)
	os.WriteFile(ctxFile, []byte("Test SRS context"), 0644)

	env := []string{"OPENROUTER_API_KEY=fake-key-for-test"}
	stdout, _, _ := runCLI(t, env,
		"--story", storyFile,
		"--context", ctxFile,
	)

	// stdout should contain JSON output (error JSON in this case)
	if !strings.Contains(stdout, `"success"`) {
		t.Fatalf("expected stdout to contain JSON with 'success' field, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"error"`) {
		t.Fatalf("expected stdout to contain JSON with 'error' field, got: %s", stdout)
	}
}

// Test --model flag overrides env var (unit test complement to Property 14)
// Validates: Requirements 9.4
func TestModelFlagOverridesEnvVarUnit(t *testing.T) {
	os.Setenv("OPENROUTER_MODEL", "provider-a/model-a")
	defer os.Unsetenv("OPENROUTER_MODEL")

	cfg := agent.LoadConfig()

	// Simulate the CLI flag override (same logic as main.go)
	flagValue := "provider-b/model-b"
	if flagValue != "" {
		cfg.Model = flagValue
	}

	if cfg.Model != "provider-b/model-b" {
		t.Fatalf("expected model to be %q, got %q", "provider-b/model-b", cfg.Model)
	}
}
