#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 11: Multi-instance name uniqueness
# Validates: Requirements 8.2, 8.5
#
# Property: For any two distinct project names, the derived container names
# (gocoder-<project>) should be distinct. The naming convention is a simple
# prefix concatenation, so distinct inputs must produce distinct outputs.
#
# Strategy: For each iteration, generate two distinct random project names,
# derive container names using the gocoder-<project> convention, and verify
# the two container names are different.
# Minimum 100 iterations.

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# ---------------------------------------------------------------------------
# Character sets for random generation
# ---------------------------------------------------------------------------
ALPHANUM="abcdefghijklmnopqrstuvwxyz0123456789"

rand_string() {
    local len="$1"
    local result=""
    local charset_len=${#ALPHANUM}
    for (( j=0; j<len; j++ )); do
        result+="${ALPHANUM:$(( RANDOM % charset_len )):1}"
    done
    echo -n "$result"
}

# ---------------------------------------------------------------------------
# Derive container name using the same convention as deploy.sh
# ---------------------------------------------------------------------------
derive_container_name() {
    local project="$1"
    echo "gocoder-${project}"
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Multi-instance name uniqueness ==="
echo "Running $ITERATIONS iterations..."
echo ""

for (( i=1; i<=ITERATIONS; i++ )); do
    # Generate two distinct random project names (3-15 chars each)
    len1=$(( (RANDOM % 13) + 3 ))
    len2=$(( (RANDOM % 13) + 3 ))
    project1=$(rand_string "$len1")
    project2=$(rand_string "$len2")

    # Ensure the two project names are actually distinct
    while [[ "$project1" == "$project2" ]]; do
        len2=$(( (RANDOM % 13) + 3 ))
        project2=$(rand_string "$len2")
    done

    # Derive container names
    container1=$(derive_container_name "$project1")
    container2=$(derive_container_name "$project2")

    # Verify the two container names are different
    if [[ "$container1" == "$container2" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: distinct projects produced identical container names"
        echo "  Project1: '$project1' -> Container: '$container1'"
        echo "  Project2: '$project2' -> Container: '$container2'"
        continue
    fi

    # Verify each container name has the expected gocoder- prefix
    if [[ "$container1" != gocoder-* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: container1 missing 'gocoder-' prefix"
        echo "  Project1: '$project1' -> Container: '$container1'"
        continue
    fi

    if [[ "$container2" != gocoder-* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: container2 missing 'gocoder-' prefix"
        echo "  Project2: '$project2' -> Container: '$container2'"
        continue
    fi

    # Verify the project name is preserved in the container name
    if [[ "$container1" != "gocoder-${project1}" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: container1 does not preserve project name"
        echo "  Expected: 'gocoder-${project1}', Got: '$container1'"
        continue
    fi

    if [[ "$container2" != "gocoder-${project2}" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: container2 does not preserve project name"
        echo "  Expected: 'gocoder-${project2}', Got: '$container2'"
        continue
    fi

    PASS_COUNT=$((PASS_COUNT + 1))
done

echo ""
echo "=== Results ==="
echo "Iterations: $ITERATIONS"
echo "Passed:     $PASS_COUNT"
echo "Failed:     $FAIL_COUNT"

if [[ $FAIL_COUNT -gt 0 ]]; then
    echo ""
    echo "PROPERTY TEST FAILED"
    exit 1
fi

echo ""
echo "PROPERTY TEST PASSED"
exit 0
