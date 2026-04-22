#!/bin/bash
# Validation Step 4: End-to-End Run with a Real API Key
# Requires OPENROUTER_API_KEY to be set

echo "=== Step 4: End-to-End Run ==="

if [ -z "$OPENROUTER_API_KEY" ]; then
  echo "SKIP: OPENROUTER_API_KEY not set — skipping e2e test"
  exit 0
fi

set -e

mkdir -p /tmp/agent-test

cat > /tmp/agent-test/story.feature << 'EOF'
Feature: Hello World
  Scenario: Generate a greeting
    Given a Go program
    When the user runs it
    Then it prints "Hello, World!" to stdout
EOF

cat > /tmp/agent-test/context.md << 'EOF'
# SRS Context
- Language: Go
- The program must be a single main.go file
- Use only the standard library
- Include a main function
EOF

echo "--- Running with default model ---"
go run ./cmd/agent/ \
  --story /tmp/agent-test/story.feature \
  --context /tmp/agent-test/context.md

echo ""
echo "--- Running with specific model and output file ---"
go run ./cmd/agent/ \
  --story /tmp/agent-test/story.feature \
  --context /tmp/agent-test/context.md \
  --model "anthropic/claude-sonnet-4" \
  --output /tmp/agent-test/result.json

echo "--- Result ---"
cat /tmp/agent-test/result.json

echo ""
echo "=== Step 4 PASSED ==="
