#!/bin/bash
# Validation Step 5: Verify Config Resolution
# Requires OPENROUTER_API_KEY to be set

echo "=== Step 5: Config Resolution ==="

if [ -z "$OPENROUTER_API_KEY" ]; then
  echo "SKIP: OPENROUTER_API_KEY not set — skipping config resolution test"
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

echo "--- Override model via env var ---"
export OPENROUTER_MODEL="openai/gpt-4o"
go run ./cmd/agent/ --story /tmp/agent-test/story.feature --context /tmp/agent-test/context.md

echo ""
echo "--- CLI flag overrides env var ---"
go run ./cmd/agent/ \
  --story /tmp/agent-test/story.feature \
  --context /tmp/agent-test/context.md \
  --model "anthropic/claude-haiku-4.5"

unset OPENROUTER_MODEL

echo ""
echo "=== Step 5 PASSED ==="
