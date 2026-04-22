#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 10: Dry-run produces commands without execution
# Validates: Requirements 10.4
#
# Property: For any set of deploy.sh arguments combined with --dry-run, the
# script should produce non-empty output describing the commands, and should
# not execute any SSH, scp, or podman commands.
#
# Strategy: Create mock ssh, scp, and podman scripts that log all invocations
# to a file. For each iteration:
#   1. Randomly select a subcommand: deploy, start, run, list, stop
#   2. Generate random flags appropriate for that subcommand
#   3. Add --dry-run flag
#   4. Set SSH_HOST to a random value
#   5. Call the corresponding cmd_* function with --dry-run
#   6. Verify: stdout is non-empty (describes what would be executed)
#   7. Verify: stdout contains "[dry-run]" prefix
#   8. Verify: the SSH/scp log file is empty (no actual SSH/scp commands executed)
# Minimum 100 iterations.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Source deploy.sh to get cmd_* functions
source "$REPO_ROOT/scripts/deploy.sh"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories and files
MOCK_BIN=""
EXEC_LOG=""
STDOUT_FILE=""
STDERR_FILE=""

cleanup() {
    rm -rf "$MOCK_BIN" "$EXEC_LOG" "$STDOUT_FILE" "$STDERR_FILE"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-dryrun10-mockbin-XXXXXX)
EXEC_LOG=$(mktemp /tmp/pbt-dryrun10-execlog-XXXXXX)
STDOUT_FILE=$(mktemp /tmp/pbt-dryrun10-stdout-XXXXXX)
STDERR_FILE=$(mktemp /tmp/pbt-dryrun10-stderr-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock ssh that logs all invocations
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/ssh" << 'MOCK_SSH_EOF'
#!/usr/bin/env bash
# Mock ssh: logs invocation to EXEC_LOG_FILE. Should NEVER be called in dry-run.
echo "SSH_CALL: $*" >> "$EXEC_LOG_FILE"
exit 0
MOCK_SSH_EOF
chmod +x "$MOCK_BIN/ssh"

# ---------------------------------------------------------------------------
# Create mock scp that logs all invocations
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/scp" << 'MOCK_SCP_EOF'
#!/usr/bin/env bash
# Mock scp: logs invocation to EXEC_LOG_FILE. Should NEVER be called in dry-run.
echo "SCP_CALL: $*" >> "$EXEC_LOG_FILE"
exit 0
MOCK_SCP_EOF
chmod +x "$MOCK_BIN/scp"

# ---------------------------------------------------------------------------
# Create mock podman that logs all invocations
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/podman" << 'MOCK_PODMAN_EOF'
#!/usr/bin/env bash
# Mock podman: logs invocation to EXEC_LOG_FILE. Should NEVER be called in dry-run.
echo "PODMAN_CALL: $*" >> "$EXEC_LOG_FILE"
exit 0
MOCK_PODMAN_EOF
chmod +x "$MOCK_BIN/podman"

# ---------------------------------------------------------------------------
# Character sets for random generation
# ---------------------------------------------------------------------------
ALPHANUM="abcdefghijklmnopqrstuvwxyz0123456789"
HEXCHARS="0123456789abcdef"

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

rand_host() {
    echo "$(( RANDOM % 256 )).$(( RANDOM % 256 )).$(( RANDOM % 256 )).$(( RANDOM % 256 ))"
}

rand_project() {
    local len=$(( (RANDOM % 10) + 3 ))
    rand_string "$len"
}

rand_repo_url() {
    local name
    name=$(rand_string $(( (RANDOM % 8) + 3 )))
    echo "https://github.com/user/${name}.git"
}

rand_path() {
    local name
    name=$(rand_string $(( (RANDOM % 8) + 3 )))
    echo "/workspace/${name}"
}

# ---------------------------------------------------------------------------
# Subcommand runners — each calls the corresponding cmd_* with --dry-run
# ---------------------------------------------------------------------------
SUBCOMMANDS=("deploy" "start" "run" "list" "stop")

run_dryrun_deploy() {
    local args=("--dry-run")
    # Randomly add --skip-build
    if (( RANDOM % 2 == 0 )); then
        args+=("--skip-build")
    fi
    cmd_deploy "${args[@]}"
}

run_dryrun_start() {
    local project
    project=$(rand_project)
    local args=("--project" "$project" "--dry-run")
    # Randomly add --replace
    if (( RANDOM % 2 == 0 )); then
        args+=("--replace")
    fi
    # Randomly add --mode
    if (( RANDOM % 2 == 0 )); then
        if (( RANDOM % 2 == 0 )); then
            args+=("--mode" "server")
            args+=("--port" "$(( (RANDOM % 9000) + 1024 ))")
        else
            args+=("--mode" "cli")
        fi
    fi
    # Randomly add 1-3 repos
    local repo_count=$(( (RANDOM % 3) + 1 ))
    for (( r=0; r<repo_count; r++ )); do
        args+=("--repo" "$(rand_repo_url)")
    done
    cmd_start "${args[@]}"
}

run_dryrun_run() {
    local project
    project=$(rand_project)
    local args=("--project" "$project" "--dry-run")
    # Randomly add --story
    if (( RANDOM % 2 == 0 )); then
        args+=("--story" "$(rand_path)")
    fi
    # Randomly add --context
    if (( RANDOM % 2 == 0 )); then
        args+=("--context" "$(rand_path)")
    fi
    # Randomly add --output
    if (( RANDOM % 2 == 0 )); then
        args+=("--output" "$(rand_path)")
    fi
    cmd_run "${args[@]}"
}

run_dryrun_list() {
    cmd_list "--dry-run"
}

run_dryrun_stop() {
    local project
    project=$(rand_project)
    cmd_stop "--project" "$project" "--dry-run"
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Dry-run produces commands without execution ==="
echo "Running $ITERATIONS iterations..."
echo ""

# Override PATH so our mock ssh/scp/podman is found first
export PATH="$MOCK_BIN:$PATH"
export EXEC_LOG_FILE="$EXEC_LOG"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Clear state
    > "$EXEC_LOG"
    > "$STDOUT_FILE"
    > "$STDERR_FILE"

    # Set SSH_HOST to a random IP so init_ssh succeeds
    export SSH_HOST
    SSH_HOST=$(rand_host)

    # Randomly select a subcommand
    sub_idx=$(( RANDOM % ${#SUBCOMMANDS[@]} ))
    subcmd="${SUBCOMMANDS[$sub_idx]}"

    # Run the dry-run subcommand, capture stdout and stderr
    local_exit=0
    "run_dryrun_${subcmd}" > "$STDOUT_FILE" 2>"$STDERR_FILE" || local_exit=$?

    stdout_content=$(cat "$STDOUT_FILE")
    exec_log_content=$(cat "$EXEC_LOG")

    # Verify 1: stdout is non-empty
    if [[ -z "$stdout_content" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i ($subcmd): stdout is empty (expected dry-run output)"
        echo "  SSH_HOST: $SSH_HOST"
        echo "  Stderr: $(cat "$STDERR_FILE")"
        continue
    fi

    # Verify 2: stdout contains "[dry-run]" prefix
    if ! echo "$stdout_content" | grep -q "\[dry-run\]"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i ($subcmd): stdout missing '[dry-run]' prefix"
        echo "  SSH_HOST: $SSH_HOST"
        echo "  Stdout: $stdout_content"
        continue
    fi

    # Verify 3: exec log is empty (no SSH/scp/podman commands were executed)
    if [[ -n "$exec_log_content" ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        echo "FAIL iteration $i ($subcmd): SSH/scp/podman commands were executed during dry-run"
        echo "  SSH_HOST: $SSH_HOST"
        echo "  Exec log:"
        sed 's/^/    /' "$EXEC_LOG"
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
