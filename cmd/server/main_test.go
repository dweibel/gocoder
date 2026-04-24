package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// serverBinary holds the path to the pre-built server binary, set in TestMain.
var serverBinary string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "server-test-bin-*")
	if err != nil {
		panic("failed to create temp dir for binary: " + err.Error())
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "server-test-bin")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("failed to build server binary: " + err.Error())
	}
	serverBinary = binPath

	os.Exit(m.Run())
}

// freePort returns an available TCP port.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

// startServer starts the server binary on the given port with a fake API key
// and returns the running *exec.Cmd. The caller must stop the process.
func startServer(t *testing.T, port string) *exec.Cmd {
	t.Helper()

	// Create a temp dir for the DB so tests don't pollute the workspace.
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test.db")

	// We need prompts dir and templates dir relative to the binary's working dir.
	// Use the workspace root (two levels up from cmd/server/).
	workDir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("failed to resolve workspace root: %v", err)
	}

	cmd := exec.Command(serverBinary)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(),
		"ELICITATION_API_KEY=fake-test-key",
		fmt.Sprintf("ARDP_PORT=%s", port),
		fmt.Sprintf("ARDP_DB_PATH=%s", dbPath),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start server: %v", err)
	}

	// Wait for the server to be ready by polling the health endpoint.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return cmd
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	cmd.Process.Kill()
	t.Fatal("server did not become ready within 5 seconds")
	return nil
}

func TestServerStartsAndHealthCheck(t *testing.T) {
	port := freePort(t)
	cmd := startServer(t, port)
	defer cmd.Process.Kill()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		t.Fatalf("health check request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /health, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status 'ok', got %q", body["status"])
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	port := freePort(t)
	cmd := startServer(t, port)

	// Send SIGINT for graceful shutdown.
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		cmd.Process.Kill()
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	// Wait for the process to exit (with timeout).
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited. A clean shutdown may return exit 0 or a signal-based exit.
		if err != nil {
			// On some systems SIGINT causes a non-zero exit — that's acceptable
			// as long as the process actually stopped.
			if !strings.Contains(err.Error(), "signal") && !strings.Contains(err.Error(), "exit status") {
				t.Logf("server exited with: %v (acceptable for signal-based shutdown)", err)
			}
		}
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("server did not shut down within 5 seconds after SIGINT")
	}

	// Verify the server is no longer accepting connections.
	_, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err == nil {
		t.Fatal("expected connection refused after shutdown, but health check succeeded")
	}
}

func TestServerFailsFastOnMissingAPIKey(t *testing.T) {
	// Build a clean env without any API key.
	cleanEnv := []string{}
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "ELICITATION_API_KEY=") &&
			!strings.HasPrefix(e, "OPENROUTER_API_KEY=") {
			cleanEnv = append(cleanEnv, e)
		}
	}
	cleanEnv = append(cleanEnv, "ARDP_PORT=0")

	workDir, _ := filepath.Abs(filepath.Join("..", ".."))
	cmd := exec.Command(serverBinary)
	cmd.Dir = workDir
	cmd.Env = cleanEnv

	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when API key is missing")
	}

	if !strings.Contains(strings.ToLower(string(output)), "apikey") &&
		!strings.Contains(strings.ToLower(string(output)), "api key") &&
		!strings.Contains(strings.ToLower(string(output)), "api_key") {
		t.Fatalf("expected error about missing API key, got: %s", string(output))
	}
}
