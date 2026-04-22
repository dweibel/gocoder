#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 3: Secret provisioning idempotence
# Validates: Requirements 2.6
#
# Property: For any .env file, running setup-secrets.sh provisioning twice in
# succession with the same input should produce the same set of podman secrets
# with the same values as running it once.
# Formally: provision(provision(state, env), env) == provision(state, env)
#
# Strategy: Create a mock `ssh` script that intercepts podman secret commands
# and stores secrets as files in a temp directory. Source setup-secrets.sh to
# get parse_env and provision_secret. For each iteration, generate random .env
# content, provision once, snapshot state, provision again, snapshot state,
# and verify both snapshots are identical.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source setup-secrets.sh to get parse_env and provision_secret
source "$REPO_ROOT/scripts/setup-secrets.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
MOCK_SECRETS=""
TMP_ENV=""
SNAPSHOT_1=""
SNAPSHOT_2=""

cleanup() {
    rm -rf "$MOCK_BIN" "$MOCK_SECRETS" "$TMP_ENV" "$SNAPSHOT_1" "$SNAPSHOT_2"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-mockbin-XXXXXX)
MOCK_SECRETS=$(mktemp -d /tmp/pbt-secrets-XXXXXX)
TMP_ENV=$(mktemp /tmp/pbt-env-XXXXXX)
SNAPSHOT_1=$(mktemp -d /tmp/pbt-snap1-XXXXXX)
SNAPSHOT_2=$(mktemp -d /tmp/pbt-snap2-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh script
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh that intercepts podman secret commands.
# Uses MOCK_SECRETS_DIR env var as the secret store directory.

# Skip ssh options and target user@host — find the actual command
# ssh is called as: ssh $SSH_OPTS "$SSH_TARGET" "command string"
# We need to extract the last argument which is the remote command.
cmd="${@: -1}"

if [[ "$cmd" == *"podman secret rm "* ]]; then
    # Extract secret name: "podman secret rm <key> 2>/dev/null || true"
    key=$(echo "$cmd" | sed -n 's/.*podman secret rm \([^ ]*\).*/\1/p')
    rm -f "$MOCK_SECRETS_DIR/$key"
    exit 0

elif [[ "$cmd" == *"podman secret create "* ]]; then
    # Extract secret name: "podman secret create <key> -"
    key=$(echo "$cmd" | sed -n 's/.*podman secret create \([^ ]*\) -.*/\1/p')
    # Read value from stdin
    cat > "$MOCK_SECRETS_DIR/$key"
    echo "$key"
    exit 0

elif [[ "$cmd" == *"podman secret ls"* ]]; then
    # List all secret names (one per line)
    if [[ -d "$MOCK_SECRETS_DIR" ]]; then
        ls "$MOCK_SECRETS_DIR" 2>/dev/null || true
    fi
    exit 0

else
    # Unknown command — pass through silently
    exit 0
fi
MOCK_SSH_EOF
chmod +x "$MOCK_BIN/ssh"

# ---------------------------------------------------------------------------
# Character sets for generation
# ---------------------------------------------------------------------------
KEY_CHARS="ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_"
VAL_CHARS="ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./:@-"

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

# Generate a random key (1-15 chars, uppercase + digits + underscore)
rand_key() {
    local len=$(( (RANDOM % 15) + 1 ))
    rand_string "$KEY_CHARS" "$len"
}

# Generate a random value (1-30 chars)
rand_value() {
    local len=$(( (RANDOM % 30) + 1 ))
    rand_string "$VAL_CHARS" "$len"
}

# ---------------------------------------------------------------------------
# Snapshot the mock secrets directory into a target directory
# Copies all files (secret name = filename, secret value = content)
# ---------------------------------------------------------------------------
snapshot_secrets() {
    local target_dir="$1"
    rm -rf "${target_dir:?}"/*
    if compgen -G "$MOCK_SECRETS_DIR/*" > /dev/null 2>&1; then
        cp "$MOCK_SECRETS_DIR"/* "$target_dir/"
    fi
}

# ---------------------------------------------------------------------------
# Compare two snapshot directories
# Returns 0 if identical (same files, same contents), 1 otherwise
# ---------------------------------------------------------------------------
compare_snapshots() {
    local dir1="$1"
    local dir2="$2"
    diff -r "$dir1" "$dir2" > /dev/null 2>&1
}

# ---------------------------------------------------------------------------
# Provision all secrets from a .env file using parse_env + provision_secret
# ---------------------------------------------------------------------------
run_provisioning() {
    local env_file="$1"
    local pairs
    pairs=$(parse_env "$env_file" 2>/dev/null) || return 0

    if [[ -z "$pairs" ]]; then
        return 0
    fi

    while IFS='=' read -r key value; do
        [[ -z "$key" ]] && continue
        provision_secret "$key" "$value" > /dev/null 2>&1
    done <<< "$pairs"
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Secret provisioning idempotence ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh is found first
export PATH="$MOCK_BIN:$PATH"

# Set SSH variables that provision_secret expects
export SSH_TARGET="mock@localhost"
export SSH_OPTS=""
export MOCK_SECRETS_DIR="$MOCK_SECRETS"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear state
    rm -rf "${MOCK_SECRETS:?}"/*
    rm -rf "${SNAPSHOT_1:?}"/*
    rm -rf "${SNAPSHOT_2:?}"/*
    > "$TMP_ENV"

    # Generate random .env content (1-8 key-value pairs)
    num_pairs=$(( (RANDOM % 8) + 1 ))
    for (( p=0; p<num_pairs; p++ )); do
        echo "$(rand_key)=$(rand_value)" >> "$TMP_ENV"
    done

    # Run provisioning once
    run_provisioning "$TMP_ENV"

    # Snapshot after first run
    snapshot_secrets "$SNAPSHOT_1"

    # Run provisioning again with same input
    run_provisioning "$TMP_ENV"

    # Snapshot after second run
    snapshot_secrets "$SNAPSHOT_2"

    # Verify: both snapshots must be identical
    if ! compare_snapshots "$SNAPSHOT_1" "$SNAPSHOT_2"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: state after second provisioning differs from first"
        echo "  Input .env:"
        sed 's/^/    /' "$TMP_ENV"
        echo "  Snapshot 1 (after first run):"
        for f in "$SNAPSHOT_1"/*; do
            [[ -f "$f" ]] && echo "    $(basename "$f")=$(cat "$f")"
        done
        echo "  Snapshot 2 (after second run):"
        for f in "$SNAPSHOT_2"/*; do
            [[ -f "$f" ]] && echo "    $(basename "$f")=$(cat "$f")"
        done
        continue
    fi

    # Also verify the secret count matches the number of unique keys
    # (last value wins for duplicate keys, matching real podman behavior)
    declare -A expected_keys
    while IFS='=' read -r key value; do
        [[ -z "$key" ]] && continue
        expected_keys["$key"]=1
    done < "$TMP_ENV"
    expected_count=${#expected_keys[@]}
    unset expected_keys

    actual_count=0
    if compgen -G "$SNAPSHOT_1/*" > /dev/null 2>&1; then
        actual_count=$(ls "$SNAPSHOT_1" | wc -l)
    fi

    if [[ "$actual_count" -ne "$expected_count" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i: expected $expected_count secrets, got $actual_count"
        echo "  Input .env:"
        sed 's/^/    /' "$TMP_ENV"
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
