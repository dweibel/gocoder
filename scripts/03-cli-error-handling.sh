#!/bin/bash
# Validation Step 3: CLI Error Handling Checks
set -e

echo "=== Step 3: CLI Error Handling Checks ==="

# Ensure no API key is set
unset OPENROUTER_API_KEY

echo "--- Missing API key ---"
output=$(go run ./cmd/agent/ --story /dev/null --context /dev/null 2>&1 || true)
echo "$output"
if echo "$output" | grep -q "APIKey must be non-empty"; then
  echo "PASS: Missing API key error detected"
else
  echo "FAIL: Expected 'APIKey must be non-empty' error"
  exit 1
fi

echo "--- Missing story file ---"
export OPENROUTER_API_KEY="test-key"
output=$(go run ./cmd/agent/ --story /nonexistent/path.feature --context /dev/null 2>&1 || true)
echo "$output"
if echo "$output" | grep -qi "error.*story"; then
  echo "PASS: Missing story file error detected"
else
  echo "FAIL: Expected story file error"
  exit 1
fi

echo "--- Missing context file ---"
output=$(go run ./cmd/agent/ --story /dev/null --context /nonexistent/path.md 2>&1 || true)
echo "$output"
if echo "$output" | grep -qi "error.*context"; then
  echo "PASS: Missing context file error detected"
else
  echo "FAIL: Expected context file error"
  exit 1
fi

unset OPENROUTER_API_KEY

echo "=== Step 3 PASSED ==="
