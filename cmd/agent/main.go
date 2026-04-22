package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ardp/coding-agent/agent"
)

func main() {
	storyPath := flag.String("story", "", "Path to Gherkin story file")
	contextPath := flag.String("context", "", "Path to SRS context file")
	outputPath := flag.String("output", "", "Output file path (default: stdout)")
	modelFlag := flag.String("model", "", "Override model string")
	timeoutFlag := flag.Int("timeout", 0, "Timeout in seconds (default: 300, from OPENROUTER_TIMEOUT)")
	flag.Parse()

	// Load config from environment variables with defaults.
	cfg := agent.LoadConfig()

	// CLI --model flag overrides env var / default.
	if *modelFlag != "" {
		cfg.Model = *modelFlag
	}

	// CLI --timeout flag overrides env var / default.
	if *timeoutFlag > 0 {
		cfg.Timeout = time.Duration(*timeoutFlag) * time.Second
	}

	// Validate config (checks API key, model, max tokens).
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	// Read story file.
	storyBytes, err := os.ReadFile(*storyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading story file: %s\n", err)
		os.Exit(1)
	}

	// Read context file.
	contextBytes, err := os.ReadFile(*contextPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading context file: %s\n", err)
		os.Exit(1)
	}

	// Create agent and execute with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	a := agent.NewCodingAgent(cfg, nil)
	result, err := a.Execute(ctx, string(storyBytes), string(contextBytes))
	if err != nil {
		errJSON, _ := agent.SerializeError(err.Error())
		writeOutput(*outputPath, errJSON)
		os.Exit(1)
	}

	resultJSON, err := agent.SerializeResult(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error serializing result: %s\n", err)
		os.Exit(1)
	}

	writeOutput(*outputPath, resultJSON)
}

// writeOutput writes data to the specified file path, or to stdout if path is empty.
func writeOutput(path string, data []byte) {
	if path == "" {
		fmt.Println(string(data))
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %s\n", err)
		os.Exit(1)
	}
}
