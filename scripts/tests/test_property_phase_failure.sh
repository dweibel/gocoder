#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 7: Phase failure identification
# Validates: Requirements 3.5, 4.4, 6.4, 10.3
#
# Property: For any deployment phase that fails, the error output should contain
# the name of the failed phase (as a tag like [build], [secrets], etc.) and the
# script should exit with a non-zero code.
#
# Strategy: Source deploy.sh to get all phase functions. Create a mock ssh script
# that can be configured to fail for specific commands. For each iteration:
#   1. Randomly select a phase to fail: build, secrets, run, exec, stop, dirs
#   2. Configure the mock/environment to trigger that phase's failure
#   3. Call the corresponding function
#   4. Verify: exit code is non-zero
#   5. Verify: stderr contains the phase tag (e.g., [build], [secrets], etc.)
# Minimum 100 iterations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh to get all functions
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
SSH_LOG=""
STDERR_FILE=""
STDOUT_FILE=""

cleanup() {
    rm -rf "$MOCK_BIN" "$SSH_LOG" "$STDERR_FILE" "$STDOUT_FILE"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-phase7-mockbin-XXXXXX)
SSH_LOG=$(mktemp /tmp/pbt-phase7-sshlog-XXXXXX)
STDERR_FILE=$(mktemp /tmp/pbt-phase7-stderr-XXXXXX)
STDOUT_FILE=$(mktemp /tmp/pbt-phase7-stdout-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh script — fails based on MOCK_FAIL_PATTERN env var
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh that fails when the command matches MOCK_FAIL_PATTERN.
# Uses SSH_LOG_FILE for logging.

cmd="${@: -1}"
echo "$cmd" >> "$SSH_LOG_FILE"

# If the command matches the fail pattern, exit non-zero
if [[ -n "${MOCK_FAIL_PATTERN:-}" && "$cmd" == *"$MOCK_FAIL_PATTERN"* ]]; then
    echo "mock failure for pattern: $MOCK_FAIL_PATTERN" >&2
    exit 1
fi

# For podman ps checks: return empty (no running container) — used by exec phase
if [[ "$cmd" == *"podman ps"* && "${MOCK_RETURN_EMPTY_PS:-}" == "true" ]]; then
    echo ""
    exit 0
fi

# Default: succeed silently
exit 0
MOCK_SSH_EOF
chmod +x "$MOCK_BIN/ssh"

# Also create a mock scp that always succeeds (not needed for failure tests)
cat > "$MOCK_BIN/scp" << 'MOCK_SCP_EOF'
#!/usr/bin/env bash
exit 0
MOCK_SCP_EOF
chmod +x "$MOCK_BIN/scp"

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
# Phase definitions: name, tag, setup function, test function
# ---------------------------------------------------------------------------
# Each phase has:
#   - tag: the error tag expected in stderr (e.g., [build])
#   - setup: function to configure the environment for failure
#   - test: function that calls the phase and captures exit code + stderr

PHASES=("build" "secrets" "run" "exec" "stop" "dirs")
PHASE_TAGS=("[build]" "[secrets]" "[run]" "[exec]" "[stop]" "[dirs]")

run_phase_build() {
    # build_image fails when Containerfile is not found
    # We run from a temp dir where no Containerfile exists
    local tmpdir
    tmpdir=$(mktemp -d /tmp/pbt-phase7-builddir-XXXXXX)
    (cd "$tmpdir" && build_image "false") > "$STDOUT_FILE" 2>"$STDERR_FILE"
    local rc=$?
    rm -rf "$tmpdir"
    return $rc
}

run_phase_secrets() {
    # check_secrets fails when ssh podman secret ls fails
    export MOCK_FAIL_PATTERN="podman secret ls"
    export MOCK_RETURN_EMPTY_PS=""
    check_secrets > "$STDOUT_FILE" 2>"$STDERR_FILE"
}

run_phase_run() {
    # start_container fails when ssh podman run fails
    local project
    project=$(rand_string $(( (RANDOM % 8) + 3 )))
    export MOCK_FAIL_PATTERN="podman run"
    export MOCK_RETURN_EMPTY_PS=""
    start_container "$project" "false" "cli" "8080" > "$STDOUT_FILE" 2>"$STDERR_FILE"
}

run_phase_exec() {
    # run_exec fails when container is not running (podman ps returns empty)
    local project
    project=$(rand_string $(( (RANDOM % 8) + 3 )))
    export MOCK_FAIL_PATTERN=""
    export MOCK_RETURN_EMPTY_PS="true"
    run_exec "$project" "" "" "" > "$STDOUT_FILE" 2>"$STDERR_FILE"
}

run_phase_stop() {
    # stop_container fails when ssh podman stop fails
    local project
    project=$(rand_string $(( (RANDOM % 8) + 3 )))
    export MOCK_FAIL_PATTERN="podman stop"
    export MOCK_RETURN_EMPTY_PS=""
    stop_container "$project" > "$STDOUT_FILE" 2>"$STDERR_FILE"
}

run_phase_dirs() {
    # ensure_dirs fails when ssh mkdir -p fails
    local project
    project=$(rand_string $(( (RANDOM % 8) + 3 )))
    export MOCK_FAIL_PATTERN="mkdir -p"
    export MOCK_RETURN_EMPTY_PS=""
    ensure_dirs "$project" > "$STDOUT_FILE" 2>"$STDERR_FILE"
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Phase failure identification ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh/scp is found first
export PATH="$MOCK_BIN:$PATH"

# Set SSH variables that functions expect
export SSH_TARGET="mock@localhost"
export SSH_OPTS=""
export SSH_LOG_FILE="$SSH_LOG"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear state
    > "$SSH_LOG"
    > "$STDERR_FILE"
    > "$STDOUT_FILE"
    export MOCK_FAIL_PATTERN=""
    export MOCK_RETURN_EMPTY_PS=""

    # Randomly select a phase
    phase_idx=$(( RANDOM % ${#PHASES[@]} ))
    phase="${PHASES[$phase_idx]}"
    expected_tag="${PHASE_TAGS[$phase_idx]}"

    # Run the phase function, capture exit code
    local_exit=0
    "run_phase_${phase}" || local_exit=$?

    stderr_content=$(cat "$STDERR_FILE")

    # Verify: exit code is non-zero
    if [[ $local_exit -eq 0 ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: phase '$phase' returned exit 0 (expected non-zero)"
        echo "  Expected tag: $expected_tag"
        echo "  Stderr: $stderr_content"
        continue
    fi

    # Verify: stderr contains the phase tag
    if [[ "$stderr_content" != *"$expected_tag"* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: phase '$phase' stderr missing tag '$expected_tag'"
        echo "  Exit code: $local_exit"
        echo "  Stderr: $stderr_content"
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
