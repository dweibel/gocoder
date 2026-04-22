#!/usr/bin/env bash
set -euo pipefail

# Feature: container-deployment, Property 12: Repo clone idempotence
# Validates: Requirements 6.2, 6.3
#
# Property: For any REPO_URLS value containing one or more URLs, if a repo's
# subdirectory already contains a .git directory, the entrypoint should skip
# cloning that repo. If the subdirectory does not exist, it should clone.
# The total number of repo subdirectories should equal the number of unique
# repo names derived from the URLs.
#
# Strategy: Create a mock git command that simulates clone by creating the
# target directory with a .git subdirectory. For each iteration, generate
# random REPO_URLS, randomly pre-create some repos (with .git dirs) to
# simulate existing clones, run the entrypoint clone logic, and verify:
#   - Pre-existing repos were NOT re-cloned (mock git not called for them)
#   - Missing repos WERE cloned (mock git was called for them)
#   - Total subdirectories equals the number of unique repo names

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

ITERATIONS=100
PASS_COUNT=0
FAIL_COUNT=0

# Temp directories
MOCK_BIN=""
TMP_WORKSPACE=""
CLONE_LOG=""

cleanup() {
    rm -rf "$MOCK_BIN" "$TMP_WORKSPACE" "$CLONE_LOG"
}
trap cleanup EXIT

MOCK_BIN=$(mktemp -d /tmp/pbt-mockbin-XXXXXX)
CLONE_LOG=$(mktemp /tmp/pbt-clonelog-XXXXXX)

# ---------------------------------------------------------------------------
# Create mock git command
# ---------------------------------------------------------------------------
cat > "$MOCK_BIN/git" << 'MOCK_GIT_EOF'
#!/usr/bin/env bash
# Mock git that simulates "git clone <url> <target>"
# Creates target dir with a .git subdirectory and logs the clone call.
if [[ "$1" == "clone" && -n "${2:-}" && -n "${3:-}" ]]; then
    url="$2"
    target="$3"
    mkdir -p "$target/.git"
    # Log the clone call (url and target) for verification
    echo "CLONED:$url:$target" >> "$CLONE_LOG_FILE"
    exit 0
fi
exit 0
MOCK_GIT_EOF
chmod +x "$MOCK_BIN/git"

# ---------------------------------------------------------------------------
# Character sets for random repo name generation
# ---------------------------------------------------------------------------
NAME_CHARS="abcdefghijklmnopqrstuvwxyz0123456789"

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

# Generate a random repo name (3-12 chars)
rand_repo_name() {
    local len=$(( (RANDOM % 10) + 3 ))
    rand_string "$NAME_CHARS" "$len"
}

# ---------------------------------------------------------------------------
# Run the entrypoint clone logic in an isolated environment
# We replicate the entrypoint.sh logic but with a configurable workspace path
# ---------------------------------------------------------------------------
run_clone_logic() {
    local workspace="$1"
    local repo_urls="$2"

    if [ -n "${repo_urls:-}" ]; then
        local IFS=','
        for url in $repo_urls; do
            local repo_name
            repo_name=$(basename "$url" .git)
            local target="$workspace/$repo_name"
            if [ -d "$target/.git" ]; then
                echo "Repo $repo_name already cloned, reusing."
            else
                echo "Cloning $url into $target..."
                git clone "$url" "$target"
                echo "Clone of $repo_name complete."
            fi
        done
    fi
}

# ---------------------------------------------------------------------------
# Test loop
# ---------------------------------------------------------------------------

echo "=== Property Test: Repo clone idempotence ==="
echo "Running $ITERATIONS iterations..."
echo ""

export PATH="$MOCK_BIN:$PATH"
export CLONE_LOG_FILE="$CLONE_LOG"

for (( i=1; i<=ITERATIONS; i++ )); do
    # Create fresh workspace
    TMP_WORKSPACE=$(mktemp -d /tmp/pbt-workspace-XXXXXX)

    # Clear clone log
    > "$CLONE_LOG"

    # Generate 1-5 unique repo names
    num_repos=$(( (RANDOM % 5) + 1 ))
    declare -A repo_names_map=()
    declare -a repo_names=()
    declare -a repo_urls=()

    while [[ ${#repo_names[@]} -lt $num_repos ]]; do
        name=$(rand_repo_name)
        if [[ -z "${repo_names_map[$name]+x}" ]]; then
            repo_names_map["$name"]=1
            repo_names+=("$name")
            # Randomly add .git suffix or not to the URL
            if (( RANDOM % 2 == 0 )); then
                repo_urls+=("https://github.com/user/${name}.git")
            else
                repo_urls+=("https://github.com/user/${name}")
            fi
        fi
    done

    # Build comma-separated REPO_URLS
    url_str=""
    for (( u=0; u<${#repo_urls[@]}; u++ )); do
        if [[ $u -gt 0 ]]; then
            url_str+=","
        fi
        url_str+="${repo_urls[$u]}"
    done

    # Randomly pre-create some repos (simulate existing clones)
    declare -A pre_existing=()
    for name in "${repo_names[@]}"; do
        if (( RANDOM % 2 == 0 )); then
            mkdir -p "$TMP_WORKSPACE/$name/.git"
            pre_existing["$name"]=1
        fi
    done

    # Run the clone logic
    run_clone_logic "$TMP_WORKSPACE" "$url_str" > /dev/null 2>&1

    # ---------------------------------------------------------------------------
    # Verify properties
    # ---------------------------------------------------------------------------
    failed=false

    # Check 1: Pre-existing repos should NOT have been cloned (not in clone log)
    for name in "${!pre_existing[@]}"; do
        if grep -q "CLONED:.*:$TMP_WORKSPACE/$name\$" "$CLONE_LOG" 2>/dev/null; then
            echo "FAIL iteration $i: pre-existing repo '$name' was re-cloned"
            echo "  REPO_URLS: $url_str"
            echo "  Pre-existing: ${!pre_existing[*]}"
            echo "  Clone log:"
            sed 's/^/    /' "$CLONE_LOG"
            failed=true
            break
        fi
    done

    # Check 2: Non-pre-existing repos SHOULD have been cloned
    if [[ "$failed" == false ]]; then
        for name in "${repo_names[@]}"; do
            if [[ -z "${pre_existing[$name]+x}" ]]; then
                # This repo was NOT pre-existing, so it should have been cloned
                if ! grep -q "CLONED:.*:$TMP_WORKSPACE/$name\$" "$CLONE_LOG" 2>/dev/null; then
                    echo "FAIL iteration $i: missing repo '$name' was NOT cloned"
                    echo "  REPO_URLS: $url_str"
                    echo "  Pre-existing: ${!pre_existing[*]}"
                    echo "  Clone log:"
                    sed 's/^/    /' "$CLONE_LOG"
                    failed=true
                    break
                fi
            fi
        done
    fi

    # Check 3: Total subdirectories equals unique repo name count
    if [[ "$failed" == false ]]; then
        actual_dirs=0
        if compgen -G "$TMP_WORKSPACE/*" > /dev/null 2>&1; then
            actual_dirs=$(find "$TMP_WORKSPACE" -mindepth 1 -maxdepth 1 -type d | wc -l)
        fi
        expected_dirs=${#repo_names[@]}

        if [[ "$actual_dirs" -ne "$expected_dirs" ]]; then
            echo "FAIL iteration $i: expected $expected_dirs subdirs, got $actual_dirs"
            echo "  REPO_URLS: $url_str"
            echo "  Pre-existing: ${!pre_existing[*]}"
            echo "  Workspace contents:"
            ls -la "$TMP_WORKSPACE" 2>/dev/null | sed 's/^/    /'
            failed=true
        fi
    fi

    # Check 4: Each repo subdir has a .git directory
    if [[ "$failed" == false ]]; then
        for name in "${repo_names[@]}"; do
            if [[ ! -d "$TMP_WORKSPACE/$name/.git" ]]; then
                echo "FAIL iteration $i: repo '$name' missing .git directory"
                echo "  REPO_URLS: $url_str"
                failed=true
                break
            fi
        done
    fi

    if [[ "$failed" == true ]]; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
    else
        PASS_COUNT=$((PASS_COUNT + 1))
    fi

    # Cleanup workspace and associative arrays
    rm -rf "$TMP_WORKSPACE"
    unset repo_names_map repo_names repo_urls pre_existing
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
