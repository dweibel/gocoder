#!/bin/bash
# Validation Step 2: Run the Full Test Suite
set -e

echo "=== Step 2: Full Test Suite ==="

echo "--- Running all tests ---"
go test ./... -v -count=1

echo "=== Step 2 PASSED ==="
