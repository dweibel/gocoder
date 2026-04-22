#!/bin/bash
# Validation Step 6: Property Test Deep Dive
set -e

echo "=== Step 6: Property Test Deep Dive ==="

echo "--- JSON round-trip property (1000 iterations) ---"
go test ./agent/ -run TestAgentResultJSONRoundTrip -rapid.checks=1000 -v

echo "--- Max iteration limit property (1000 iterations) ---"
go test ./agent/ -run TestMaxIterationLimit -rapid.checks=1000 -v

echo "--- All property tests (500 iterations) ---"
go test ./agent/ -rapid.checks=500 -v

echo "=== Step 6 PASSED ==="
