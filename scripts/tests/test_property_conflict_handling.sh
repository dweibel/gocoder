#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 8: Container name conflict handling
# Validates: Requirements 5.8, 5.9, 8.2
#
# Property: For any project name where a container gocoder-<project> already
# exists, the start subcommand without --replace should print a warning and
# exit without starting a new instance. With --replace, it should stop the
# existing container and start a new one.
#
# Strategy: Create a mock ssh script that simulates an existing container by
# returning the container name for podman ps -a queries. For each iteration:
#   1. Generate a random project name
#   2. Test WITHOUT replace: call start_container with replace=false
#      - Verify stderr contains "already exists" or "Warning"
#      - Verify NO podman run, podman stop, or podman rm was issued
#   3. Test WITH replace: call start_container with replace=true
#      - Verify podman stop gocoder-<project> was issued
#      - Verify podman rm gocoder-<project> was issued
#      - Verify podman run was issued (new container started)
# Minimum 100 iterations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh to get start_container, ensure_dirs, REQUIRED_SECRETS
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
SSH_LOG=""
STDERR_FILE=""

cleanup() {
    rm -rf "$MOCK_BIN" "$SSH_LOG" "$STDERR_FILE"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-conflict8-mockbin-XXXXXX)
SSH_LOG=$(mktemp /tmp/pbt-conflict8-sshlog-XXXXXX)
STDERR_FILE=$(mktemp /tmp/pbt-conflict8-stderr-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh script
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh that simulates an existing container for conflict tests.
# Uses SSH_LOG_FILE for logging and MOCK_CONTAINER_NAME for the existing name.

cmd="${@: -1}"
echo "$cmd" >> "$SSH_LOG_FILE"

# For podman ps -a --filter name=^<name>$: return the container name (existing)
if [[ "$cmd" == *"podman ps -a --filter"* ]]; then
    echo "$MOCK_CONTAINER_NAME"
    exit 0
fi

# For mkdir -p: succeed silently
if [[ "$cmd" == *"mkdir -p"* ]]; then
    exit 0
fi

# For podman stop: succeed
if [[ "$cmd" == *"podman stop"* ]]; then
    echo "$MOCK_CONTAINER_NAME"
    exit 0
fi

# For podman rm: succeed
if [[ "$cmd" == *"podman rm"* ]]; then
    echo "$MOCK_CONTAINER_NAME"
    exit 0
fi

# For podman run: succeed and return a fake container ID
if [[ "$cmd" == *"podman run"* ]]; then
    echo "abc123def456"
    exit 0
fi

# Default: succeed silently
exit 0
MOCK_SSH_EOF
chmod +x "$MOCK_BIN/ssh"

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
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Container name conflict handling ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh is found first
export PATH="$MOCK_BIN:$PATH"

# Set SSH variables that start_container expects
export SSH_TARGET="mock@localhost"
export SSH_OPTS=""
export SSH_LOG_FILE="$SSH_LOG"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Generate random project name (3-12 chars)
    proj_len=$(( (RANDOM % 10) + 3 ))
    project=$(rand_string "$proj_len")
    container_name="gocoder-${project}"

    # Export the container name so the mock ssh can return it
    export MOCK_CONTAINER_NAME="$container_name"

    # -----------------------------------------------------------------------
    # Part A: Test WITHOUT replace (replace=false)
    # -----------------------------------------------------------------------
    > "$SSH_LOG"
    > "$STDERR_FILE"

    start_container "$project" "false" "cli" "8080" \
        > /dev/null 2>"$STDERR_FILE" || true

    stderr_content=$(cat "$STDERR_FILE")
    ssh_log_content=$(cat "$SSH_LOG")

    # Verify: stderr contains "already exists" or "Warning"
    if ! echo "$stderr_content" | grep -qiE "already exists|warning"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (no-replace): stderr missing 'already exists' or 'Warning'"
        echo "  Project: $project"
        echo "  Stderr: $stderr_content"
        continue
    fi

    # Verify: NO podman run command was issued
    if echo "$ssh_log_content" | grep -q "podman run"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (no-replace): podman run was issued (should not start new container)"
        echo "  Project: $project"
        echo "  SSH log:"
        sed 's/^/    /' "$SSH_LOG"
        continue
    fi

    # Verify: NO podman stop or podman rm was issued
    if echo "$ssh_log_content" | grep -qE "podman stop|podman rm"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (no-replace): podman stop/rm was issued (should not touch existing container)"
        echo "  Project: $project"
        echo "  SSH log:"
        sed 's/^/    /' "$SSH_LOG"
        continue
    fi

    # -----------------------------------------------------------------------
    # Part B: Test WITH replace (replace=true)
    # -----------------------------------------------------------------------
    > "$SSH_LOG"
    > "$STDERR_FILE"

    start_container "$project" "true" "cli" "8080" \
        > /dev/null 2>"$STDERR_FILE" || true

    ssh_log_content=$(cat "$SSH_LOG")

    # Verify: podman stop gocoder-<project> was issued
    if ! echo "$ssh_log_content" | grep -q "podman stop ${container_name}"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (replace): podman stop ${container_name} not found in SSH log"
        echo "  Project: $project"
        echo "  SSH log:"
        sed 's/^/    /' "$SSH_LOG"
        continue
    fi

    # Verify: podman rm gocoder-<project> was issued
    if ! echo "$ssh_log_content" | grep -q "podman rm ${container_name}"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (replace): podman rm ${container_name} not found in SSH log"
        echo "  Project: $project"
        echo "  SSH log:"
        sed 's/^/    /' "$SSH_LOG"
        continue
    fi

    # Verify: podman run was issued (new container started)
    if ! echo "$ssh_log_content" | grep -q "podman run"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (replace): podman run not found in SSH log"
        echo "  Project: $project"
        echo "  SSH log:"
        sed 's/^/    /' "$SSH_LOG"
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
