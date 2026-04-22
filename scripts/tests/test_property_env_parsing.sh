#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 1: .env parsing round trip
# Validates: Requirements 2.4
#
# Property: For any valid .env file containing key-value pairs (non-comment,
# non-blank lines in KEY=VALUE format), parse_env extracts every key and its
# corresponding value, and the count of extracted pairs equals the number of
# valid KEY=VALUE lines in the file.
#
# Strategy: Generate random .env content with valid KEY=VALUE pairs, comment
# lines, and blank lines. Write to a temp file, run parse_env, and verify
# the output count and content match expectations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source setup-secrets.sh to get parse_env
source "$REPO_ROOT/scripts/setup-secrets.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0
TMP_ENV=""
TMP_EXPECTED=""
TMP_ACTUAL=""

cleanup() {
    rm -f "$TMP_ENV" "$TMP_EXPECTED" "$TMP_ACTUAL"
}
trap cleanup EXIT

TMP_ENV=$(mktemp /tmp/pbt-env-XXXXXX)
TMP_EXPECTED=$(mktemp /tmp/pbt-expected-XXXXXX)
TMP_ACTUAL=$(mktemp /tmp/pbt-actual-XXXXXX)

# ---------------------------------------------------------------------------
# Character sets for generation (no /dev/urandom — use $RANDOM for speed)
# ---------------------------------------------------------------------------
KEY_CHARS="ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
VAL_CHARS="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:@-"
COMMENT_CHARS="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 "

# Generate a random string from a charset
# $1 = charset, $2 = length
rand_string() {
    local charset="$1"
    local len="$2"
    local result=""
    local charset_len=${#charset}
    for (( j=0; j<len; j++ )); do
        result+="${charset:$(( RANDOM % charset_len )):1}"
    done
    echo -n "$result"
}

# Generate a random alphanumeric key (1-15 chars)
rand_key() {
    local len=$(( (RANDOM % 15) + 1 ))
    rand_string "$KEY_CHARS" "$len"
}

# Generate a random value (0-30 chars)
rand_value() {
    local len=$(( RANDOM % 31 ))
    if [[ $len -eq 0 ]]; then
        echo -n ""
        return
    fi
    rand_string "$VAL_CHARS" "$len"
}

# Generate a random comment line
rand_comment() {
    local spaces=""
    local num_spaces=$(( RANDOM % 4 ))
    for (( s=0; s<num_spaces; s++ )); do
        spaces+=" "
    done
    local text_len=$(( (RANDOM % 20) + 1 ))
    local text
    text=$(rand_string "$COMMENT_CHARS" "$text_len")
    echo "${spaces}# ${text}"
}

# Generate a random blank line (0-5 spaces)
rand_blank() {
    local num_spaces=$(( RANDOM % 6 ))
    printf '%*s' "$num_spaces" ""
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: .env parsing round trip ==="
echo "Running $ITERATIONS iterations..."
echo ""

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear temp files
    > "$TMP_ENV"
    > "$TMP_EXPECTED"

    # Decide how many valid pairs (1-10)
    num_pairs=$(( (RANDOM % 10) + 1 ))
    # Decide how many comment lines (0-5)
    num_comments=$(( RANDOM % 6 ))
    # Decide how many blank lines (0-5)
    num_blanks=$(( RANDOM % 6 ))

    # Build the env file content and expected output
    declare -a env_lines=()
    declare -a expected_pairs=()

    # Generate valid KEY=VALUE pairs
    for (( p=0; p<num_pairs; p++ )); do
        key=$(rand_key)
        value=$(rand_value)
        env_lines+=("${key}=${value}")
        expected_pairs+=("${key}=${value}")
    done

    # Generate comment lines
    for (( c=0; c<num_comments; c++ )); do
        env_lines+=("$(rand_comment)")
    done

    # Generate blank lines
    for (( b=0; b<num_blanks; b++ )); do
        env_lines+=("$(rand_blank)")
    done

    # Shuffle lines: assign random sort keys and sort
    total=${#env_lines[@]}
    if [[ $total -gt 0 ]]; then
        {
            for (( idx=0; idx<total; idx++ )); do
                echo "$RANDOM ${env_lines[$idx]}"
            done
        } | sort -n | cut -d' ' -f2- > "$TMP_ENV"
    fi

    # Write expected pairs (sorted for comparison)
    printf '%s\n' "${expected_pairs[@]}" | sort > "$TMP_EXPECTED"

    # Run parse_env and capture output (discard stderr warnings), sort for comparison
    parse_env "$TMP_ENV" 2>/dev/null | sort > "$TMP_ACTUAL"

    # Count expected
    expected_count=${#expected_pairs[@]}

    # Count actual (non-empty lines)
    if [[ -s "$TMP_ACTUAL" ]]; then
        actual_count=$(grep -c . "$TMP_ACTUAL" || true)
    else
        actual_count=0
    fi

    # Check 1: count matches
    if [[ "$actual_count" -ne "$expected_count" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: expected $expected_count pairs, got $actual_count"
        echo "  Input file:"
        sed 's/^/    /' "$TMP_ENV"
        echo "  Expected:"
        sed 's/^/    /' "$TMP_EXPECTED"
        echo "  Actual:"
        sed 's/^/    /' "$TMP_ACTUAL"
        unset env_lines expected_pairs
        continue
    fi

    # Check 2: content matches
    if ! diff -q "$TMP_EXPECTED" "$TMP_ACTUAL" > /dev/null 2>&1; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: content mismatch"
        echo "  Expected:"
        sed 's/^/    /' "$TMP_EXPECTED"
        echo "  Actual:"
        sed 's/^/    /' "$TMP_ACTUAL"
        unset env_lines expected_pairs
        continue
    fi

    PASS_COUNT=$((PASS_COUNT + 1))
    unset env_lines expected_pairs
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
