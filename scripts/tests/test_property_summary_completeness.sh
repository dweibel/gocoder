#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 9: Deployment summary completeness
# Validates: Requirements 10.2
#
# Property: For any successful deployment, the printed summary should contain
# the image tag, a timestamp, the target host, and the deploy directory.
#
# Strategy: Source deploy.sh to get validate_deploy. Create a mock ssh that
# succeeds for podman images queries (returns the image name) and for the
# deploy log write. For each iteration:
#   1. Generate random IMAGE_TAG, SSH_HOST, DEPLOY_DIR values
#   2. Set the globals accordingly
#   3. Call validate_deploy and capture stdout
#   4. Verify the output contains:
#      - The image tag string (IMAGE_NAME:IMAGE_TAG)
#      - A timestamp pattern (YYYY-MM-DD HH:MM:SS)
#      - The SSH_HOST value
#      - The DEPLOY_DIR value
# Minimum 100 iterations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh to get validate_deploy and other functions
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
SSH_LOG=""
STDOUT_FILE=""
STDERR_FILE=""

cleanup() {
    rm -rf "$MOCK_BIN" "$SSH_LOG" "$STDOUT_FILE" "$STDERR_FILE"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-summary9-mockbin-XXXXXX)
SSH_LOG=$(mktemp /tmp/pbt-summary9-sshlog-XXXXXX)
STDOUT_FILE=$(mktemp /tmp/pbt-summary9-stdout-XXXXXX)
STDERR_FILE=$(mktemp /tmp/pbt-summary9-stderr-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh script — succeeds for podman images and log writes
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh that returns the expected image for podman images queries
# and succeeds for mkdir/echo (deploy log write).

cmd="${@: -1}"
echo "$cmd" >> "$SSH_LOG_FILE"

# For podman images query: return the image name from MOCK_IMAGE_RESULT
if [[ "$cmd" == *"podman images"* ]]; then
    echo "$MOCK_IMAGE_RESULT"
    exit 0
fi

# For mkdir + echo (deploy log write): succeed silently
if [[ "$cmd" == *"mkdir -p"* ]]; then
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
HEX_CHARS="0123456789abcdef"

rand_string() {
    local len="$1"
    local charset="${2:-$ALPHANUM}"
    local result=""
    local charset_len=${#charset}
    for (( j=0; j<len; j++ )); do
        result+="${charset:$(( RANDOM % charset_len )):1}"
    done
    echo -n "$result"
}

rand_hex() {
    local len="$1"
    rand_string "$len" "$HEX_CHARS"
}

rand_path_segment() {
    # Generate a path-like string: /some/random/path
    local depth=$(( (RANDOM % 3) + 1 ))
    local result=""
    for (( d=0; d<depth; d++ )); do
        result+="/$(rand_string $(( (RANDOM % 8) + 3 )))"
    done
    echo -n "~$result"
}

rand_host() {
    # Generate a random IP-like or hostname string
    local choice=$(( RANDOM % 2 ))
    if [[ $choice -eq 0 ]]; then
        # IP address
        echo -n "$(( RANDOM % 256 )).$(( RANDOM % 256 )).$(( RANDOM % 256 )).$(( RANDOM % 256 ))"
    else
        # hostname
        echo -n "$(rand_string $(( (RANDOM % 8) + 4 ))).example.com"
    fi
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Deployment summary completeness ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh is found first
export PATH="$MOCK_BIN:$PATH"

# Set SSH variables that validate_deploy expects
export SSH_TARGET="mock@localhost"
export SSH_OPTS=""
export SSH_LOG_FILE="$SSH_LOG"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear state
    > "$SSH_LOG"
    > "$STDOUT_FILE"
    > "$STDERR_FILE"

    # Generate random values
    tag=$(rand_hex 7)
    host=$(rand_host)
    deploy_dir=$(rand_path_segment)

    # Set globals that validate_deploy reads
    export IMAGE_NAME="gocoder"
    export IMAGE_TAG="$tag"
    export SSH_HOST="$host"
    export DEPLOY_DIR="$deploy_dir"

    # The mock ssh should return the image name for podman images queries
    export MOCK_IMAGE_RESULT="${IMAGE_NAME}:${IMAGE_TAG}"

    # Call validate_deploy and capture output
    local_exit=0
    validate_deploy > "$STDOUT_FILE" 2>"$STDERR_FILE" || local_exit=$?

    stdout_content=$(cat "$STDOUT_FILE")

    # Verify: exit code is zero (successful validation)
    if [[ $local_exit -ne 0 ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: validate_deploy returned exit $local_exit (expected 0)"
        echo "  IMAGE_TAG=$tag SSH_HOST=$host DEPLOY_DIR=$deploy_dir"
        echo "  Stderr: $(cat "$STDERR_FILE")"
        continue
    fi

    # Verify: output contains the image tag (IMAGE_NAME:IMAGE_TAG)
    if [[ "$stdout_content" != *"${IMAGE_NAME}:${tag}"* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: output missing image tag '${IMAGE_NAME}:${tag}'"
        echo "  Output: $stdout_content"
        continue
    fi

    # Verify: output contains a timestamp pattern (YYYY-MM-DD HH:MM:SS)
    if ! echo "$stdout_content" | grep -qE '[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}'; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: output missing timestamp pattern YYYY-MM-DD HH:MM:SS"
        echo "  Output: $stdout_content"
        continue
    fi

    # Verify: output contains the SSH_HOST value
    if [[ "$stdout_content" != *"${host}"* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: output missing SSH_HOST '${host}'"
        echo "  Output: $stdout_content"
        continue
    fi

    # Verify: output contains the DEPLOY_DIR value
    if [[ "$stdout_content" != *"${deploy_dir}"* ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: output missing DEPLOY_DIR '${deploy_dir}'"
        echo "  Output: $stdout_content"
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
