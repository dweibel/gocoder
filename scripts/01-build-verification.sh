#!/bin/bash
# Validation Step 1: Build Verification
set -e

echo "=== Step 1: Build Verification ==="

echo "--- Compiling all packages ---"
go build ./...
echo "PASS: go build ./..."

echo "--- Static analysis ---"
go vet ./...
echo "PASS: go vet ./..."

echo "=== Step 1 PASSED ==="
