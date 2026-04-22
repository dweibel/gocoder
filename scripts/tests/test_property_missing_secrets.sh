#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 2: Missing secret detection is complete
# Validates: Requirements 2.5
#
# Property: For any set of required secret names and any subset of those secrets
# that actually exist on the remote host, the check_secrets function should report
# exactly the secrets that are missing (the set difference), and should return a
# non-zero exit code if and only if that set is non-empty.
#
# Strategy: Create a mock ssh script that returns a configurable subset of
# REQUIRED_SECRETS when podman secret ls is called. Source deploy.sh to get
# check_secrets and REQUIRED_SECRETS. For each iteration, randomly select a
# subset of secrets to be "present", call check_secrets, and verify:
#   - Exit code is 0 iff all secrets are present
#   - Each missing secret appears in stderr output
#   - No present secret appears in the "missing" error output

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh to get check_secrets and REQUIRED_SECRETS
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
PRESENT_FILE=""
STDERR_FILE=""

cleanup() {
    rm -rf "$MOCK_BIN" "$PRESENT_FILE" "$STDERR_FILE"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-mockbin-XXXXXX)
PRESENT_FILE=$(mktemp /tmp/pbt-present-XXXXXX)
STDERR_FILE=$(mktemp /tmp/pbt-stderr-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh script
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh that intercepts podman secret ls commands.
# Uses MOCK_PRESENT_FILE env var — a file listing present secret names (one per line).

# The actual command is the last argument
cmd="${@: -1}"

if [[ "$cmd" == *"podman secret ls"* ]]; then
    # Return the list of "present" secrets
    cat "$MOCK_PRESENT_FILE" 2>/dev/null || true
    exit 0
fi

# Unknown command — pass through silently
exit 0
MOCK_SSH_EOF
chmod +x "$MOCK_BIN/ssh"

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Missing secret detection is complete ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh is found first
export PATH="$MOCK_BIN:$PATH"

# Set SSH variables that check_secrets expects
export SSH_TARGET="mock@localhost"
export SSH_OPTS=""
export MOCK_PRESENT_FILE="$PRESENT_FILE"

NUM_SECRETS=${#REQUIRED_SECRETS[@]}

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear state
    > "$PRESENT_FILE"
    > "$STDERR_FILE"

    # Randomly select a subset of REQUIRED_SECRETS to be "present"
    declare -A present_set=()
    declare -A missing_set=()

    for secret in "${REQUIRED_SECRETS[@]}"; do
        if (( RANDOM % 2 == 0 )); then
            echo "$secret" >> "$PRESENT_FILE"
            present_set["$secret"]=1
        else
            missing_set["$secret"]=1
        fi
    done

    expected_missing_count=${#missing_set[@]}

    # Call check_secrets, capture exit code and stderr
    local_exit=0
    check_secrets > /dev/null 2>"$STDERR_FILE" || local_exit=$?

    stderr_content=$(cat "$STDERR_FILE")

    # Verify exit code
    if [[ $expected_missing_count -eq 0 ]]; then
        # All secrets present — should return 0
        if [[ $local_exit -ne 0 ]]; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
            echo "FAIL iteration $i: expected exit 0 (all secrets present), got $local_exit"
            echo "  Present: ${!present_set[*]}"
            echo "  Stderr: $stderr_content"
            unset present_set missing_set
            continue
        fi
    else
        # Some secrets missing — should return non-zero
        if [[ $local_exit -eq 0 ]]; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
            echo "FAIL iteration $i: expected non-zero exit (missing secrets), got 0"
            echo "  Missing: ${!missing_set[*]}"
            echo "  Stderr: $stderr_content"
            unset present_set missing_set
            continue
        fi
    fi

    # Verify each missing secret appears in stderr
    check_failed=false
    for secret in "${!missing_set[@]}"; do
        if [[ "$stderr_content" != *"$secret"* ]]; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
            echo "FAIL iteration $i: missing secret '$secret' not found in stderr"
            echo "  Stderr: $stderr_content"
            check_failed=true
            break
        fi
    done

    if [[ "$check_failed" == true ]]; then
        unset present_set missing_set
        continue
    fi

    # Verify no present secret appears in the "missing" error lines
    # Only check lines that are part of the missing-secret report (lines with "  - ")
    missing_lines=$(grep '  - ' "$STDERR_FILE" 2>/dev/null || true)

    for secret in "${!present_set[@]}"; do
        if echo "$missing_lines" | grep -q "$secret"; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
            echo "FAIL iteration $i: present secret '$secret' incorrectly listed as missing"
            echo "  Present: ${!present_set[*]}"
            echo "  Missing lines: $missing_lines"
            check_failed=true
            break
        fi
    done

    if [[ "$check_failed" == true ]]; then
        unset present_set missing_set
        continue
    fi

    PASS_COUNT=$((PASS_COUNT + 1))
    unset present_set missing_set
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
