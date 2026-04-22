#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 4: Image tag contains version identifier
# Validates: Requirements 3.2, 3.3
#
# Property: For any git short SHA string (7 hex characters), the build_image
# function should produce image tags where one tag contains that exact SHA
# string and another tag equals "latest". Additionally, IMAGE_TAG should equal
# the SHA and IMAGE_ARCHIVE should equal "${IMAGE_NAME}-<sha>.tar".
#
# Strategy: Generate random 7-hex-char SHA strings. Mock git to return each
# SHA, mock docker/podman to capture build and save commands (logging them for
# verification), create a fake Containerfile, call build_image false, and
# verify the tags and global variables.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh (source guard prevents main from running)
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
WORK_DIR=""
CMD_LOG=""

cleanup() {
    rm -rf "$MOCK_BIN" "$WORK_DIR"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-mockbin-XXXXXX)
WORK_DIR=$(mktemp -d /tmp/pbt-workdir-XXXXXX)
CMD_LOG="$WORK_DIR/cmd.log"

# ---------------------------------------------------------------------------
# Create mock docker script that logs commands and succeeds
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/docker" << 'MOCK_DOCKER_EOF'
#!/usr/bin/env bash
# Mock docker that logs commands and simulates success.
# Uses CMD_LOG_FILE env var to record invocations.

echo "docker $*" >> "$CMD_LOG_FILE"

if [[ "$1" == "build" ]]; then
    # Simulate successful build — just exit 0
    echo "Successfully built mock-image"
    exit 0
elif [[ "$1" == "save" ]]; then
    # Find the -o flag and create an empty tar file
    while [[ $# -gt 0 ]]; do
        if [[ "$1" == "-o" ]]; then
            touch "$2"
            break
        fi
        shift
    done
    exit 0
else
    exit 0
fi
MOCK_DOCKER_EOF
chmod +x "$MOCK_BIN/docker"

# ---------------------------------------------------------------------------
# Hex charset for SHA generation
# ---------------------------------------------------------------------------
HEX_CHARS="0123456789abcdef"

# Generate a random 7-hex-char string
rand_sha() {
    local result=""
    for (( j=0; j<7; j++ )); do
        result+="${HEX_CHARS:$(( RANDOM % 16 )):1}"
    done
    echo -n "$result"
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Image tag contains version identifier ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Put mock docker first on PATH
export PATH="$MOCK_BIN:$PATH"
export CMD_LOG_FILE="$CMD_LOG"

# Create a fake Containerfile in the work directory
touch "$WORK_DIR/Containerfile"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Generate random SHA
    sha=$(rand_sha)

    # Clear command log and any previous archives
    > "$CMD_LOG"
    rm -f "$WORK_DIR"/*.tar

    # Run build_image in a subshell with mocked git and working directory
    output=$(
        cd "$WORK_DIR"
        IMAGE_NAME="gocoder"
        BUILD_TARGET="./cmd/agent"

        # Mock git to return our random SHA
        git() {
            if [[ "$1" == "rev-parse" && "$2" == "--short" && "$3" == "HEAD" ]]; then
                echo "$sha"
                return 0
            fi
            command git "$@"
        }
        # Mock command -v to find docker (already on PATH via MOCK_BIN)
        # No override needed — the mock docker is on PATH

        build_image false 2>&1
        echo "IMAGE_TAG=$IMAGE_TAG"
        echo "IMAGE_ARCHIVE=$IMAGE_ARCHIVE"
    )

    # Read the command log
    logged_cmds=""
    if [[ -f "$CMD_LOG" ]]; then
        logged_cmds=$(cat "$CMD_LOG")
    fi

    failed=false
    reason=""

    # Check 1: docker build command includes -t IMAGE_NAME:<sha>
    if ! echo "$logged_cmds" | grep -q "docker build.*-t gocoder:${sha}"; then
        failed=true
        reason="docker build missing -t gocoder:${sha} tag"
    fi

    # Check 2: docker build command includes -t IMAGE_NAME:latest
    if ! echo "$logged_cmds" | grep -q "docker build.*-t gocoder:latest"; then
        failed=true
        reason="${reason:+$reason; }docker build missing -t gocoder:latest tag"
    fi

    # Check 3: IMAGE_TAG equals the SHA
    if ! echo "$output" | grep -q "IMAGE_TAG=${sha}"; then
        failed=true
        reason="${reason:+$reason; }IMAGE_TAG not set to ${sha}"
    fi

    # Check 4: IMAGE_ARCHIVE equals gocoder-<sha>.tar
    if ! echo "$output" | grep -q "IMAGE_ARCHIVE=gocoder-${sha}.tar"; then
        failed=true
        reason="${reason:+$reason; }IMAGE_ARCHIVE not set to gocoder-${sha}.tar"
    fi

    # Check 5: docker save command includes both tags
    if ! echo "$logged_cmds" | grep -q "docker save.*gocoder:${sha}"; then
        failed=true
        reason="${reason:+$reason; }docker save missing gocoder:${sha}"
    fi
    if ! echo "$logged_cmds" | grep -q "docker save.*gocoder:latest"; then
        failed=true
        reason="${reason:+$reason; }docker save missing gocoder:latest"
    fi

    if [[ "$failed" == true ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i (sha=$sha): $reason"
        echo "  Logged commands:"
        echo "$logged_cmds" | sed 's/^/    /'
        echo "  Output:"
        echo "$output" | sed 's/^/    /'
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
