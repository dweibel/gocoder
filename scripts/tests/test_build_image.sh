#!/usr/bin/env bash
# =============================================================================
# Unit tests for build_image() in deploy.sh
# Sources deploy.sh (source guard prevents main from running)
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_SCRIPT="$SCRIPT_DIR/../deploy.sh"

PASS=0
FAIL=0

pass() { ((PASS++)); echo "  PASS: $1"; }
fail() { ((FAIL++)); echo "  FAIL: $1"; }

# Create a temp directory for test isolation
TMPDIR_TEST=$(mktemp -d)
trap 'rm -rf "$TMPDIR_TEST"' EXIT

# ---------------------------------------------------------------------------
# Test 1: skip_build=true with existing archive returns 0
# ---------------------------------------------------------------------------
echo "Test: skip_build with existing archive"
output=$(
    set +e
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    touch "gocoder-abc1234.tar"
    git() {
        if [[ "$1" == "rev-parse" ]]; then echo "abc1234"; return 0; fi
        command git "$@"
    }
    build_image true 2>&1
    echo "EXIT_CODE=$?"
)
if echo "$output" | grep -q "EXIT_CODE=0" && echo "$output" | grep -q "Skipping build"; then
    pass "skip_build with matching archive succeeds"
else
    fail "skip_build with matching archive: $output"
fi
rm -f "$TMPDIR_TEST"/gocoder-*.tar

# ---------------------------------------------------------------------------
# Test 2: skip_build=true with no archive exits 1
# ---------------------------------------------------------------------------
echo "Test: skip_build with no archive"
rm -f "$TMPDIR_TEST"/gocoder-*.tar
output=$(
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    build_image true 2>&1
) 2>&1 && rc=0 || rc=$?
if [[ $rc -ne 0 ]] && echo "$output" | grep -q "Error \[build\]"; then
    pass "skip_build with no archive exits with error"
else
    fail "skip_build with no archive: rc=$rc output=$output"
fi

# ---------------------------------------------------------------------------
# Test 3: skip_build=true finds fallback archive
# ---------------------------------------------------------------------------
echo "Test: skip_build finds fallback archive with different tag"
touch "$TMPDIR_TEST/gocoder-oldsha99.tar"
output=$(
    set +e
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    git() {
        if [[ "$1" == "rev-parse" ]]; then echo "newsha11"; return 0; fi
        command git "$@"
    }
    build_image true 2>&1
    echo "EXIT_CODE=$?"
    echo "IMAGE_ARCHIVE=$IMAGE_ARCHIVE"
)
if echo "$output" | grep -q "EXIT_CODE=0" && echo "$output" | grep -q "Skipping build" && echo "$output" | grep -q "IMAGE_ARCHIVE=gocoder-oldsha99.tar"; then
    pass "skip_build fallback finds existing archive and updates IMAGE_ARCHIVE"
else
    fail "skip_build fallback: $output"
fi
rm -f "$TMPDIR_TEST"/gocoder-*.tar

# ---------------------------------------------------------------------------
# Test 4: Missing Containerfile exits 1
# ---------------------------------------------------------------------------
echo "Test: missing Containerfile"
rm -f "$TMPDIR_TEST/Containerfile"
output=$(
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    build_image false 2>&1
) 2>&1 && rc=0 || rc=$?
if [[ $rc -ne 0 ]] && echo "$output" | grep -q "Containerfile not found"; then
    pass "missing Containerfile exits with descriptive error"
else
    fail "missing Containerfile: rc=$rc output=$output"
fi

# ---------------------------------------------------------------------------
# Test 5: Error messages include [build] phase identifier
# ---------------------------------------------------------------------------
echo "Test: error messages include [build] phase"
rm -f "$TMPDIR_TEST"/gocoder-*.tar
output=$(
    set +e
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    build_image true 2>&1
)
if echo "$output" | grep -q "\[build\]"; then
    pass "error messages include [build] phase identifier"
else
    fail "phase identification missing: $output"
fi

# ---------------------------------------------------------------------------
# Test 6: IMAGE_TAG and IMAGE_ARCHIVE are set correctly
# ---------------------------------------------------------------------------
echo "Test: IMAGE_TAG and IMAGE_ARCHIVE globals are set"
touch "$TMPDIR_TEST/gocoder-abc1234.tar"
output=$(
    set +e
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    git() {
        if [[ "$1" == "rev-parse" ]]; then echo "abc1234"; return 0; fi
        command git "$@"
    }
    build_image true >/dev/null 2>&1
    echo "IMAGE_TAG=$IMAGE_TAG"
    echo "IMAGE_ARCHIVE=$IMAGE_ARCHIVE"
)
if echo "$output" | grep -q "IMAGE_TAG=abc1234" && echo "$output" | grep -q "IMAGE_ARCHIVE=gocoder-abc1234.tar"; then
    pass "IMAGE_TAG and IMAGE_ARCHIVE set correctly"
else
    fail "globals: $output"
fi
rm -f "$TMPDIR_TEST"/gocoder-*.tar

# ---------------------------------------------------------------------------
# Test 7: Timestamp fallback when git is unavailable
# ---------------------------------------------------------------------------
echo "Test: timestamp fallback when git unavailable"
output=$(
    set +e
    source "$DEPLOY_SCRIPT"
    cd "$TMPDIR_TEST"
    IMAGE_NAME="gocoder"
    BUILD_TARGET="./cmd/agent"
    # Hide git entirely
    git() { return 1; }
    command() {
        if [[ "$1" == "-v" && "$2" == "git" ]]; then return 1; fi
        builtin command "$@"
    }
    # Create a matching archive for skip_build
    ts=$(date +"%Y%m%d")
    touch "gocoder-${ts}-000000.tar"
    build_image true >/dev/null 2>&1
    echo "IMAGE_TAG=$IMAGE_TAG"
)
tag=$(echo "$output" | grep "IMAGE_TAG=" | sed 's/IMAGE_TAG=//')
if [[ "$tag" =~ ^[0-9]{8}-[0-9]{6}$ ]]; then
    pass "timestamp fallback produces correct YYYYMMDD-HHMMSS format"
else
    fail "timestamp fallback: tag=$tag"
fi
rm -f "$TMPDIR_TEST"/gocoder-*.tar

# ---------------------------------------------------------------------------
# Results
# ---------------------------------------------------------------------------
echo ""
echo "Results: $PASS passed, $FAIL failed"
if [[ $FAIL -gt 0 ]]; then
    exit 1
fi
