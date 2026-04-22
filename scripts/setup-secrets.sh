#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# setup-secrets.sh — Provision podman secrets on OCI instance from local .env
#
# Usage: setup-secrets.sh [--env-file <path>]
#
# Reads KEY=VALUE pairs from a local .env file and creates corresponding
# podman secrets on the remote OCI instance via SSH. The .env file is never
# transferred to the instance (Req 2.7). Provisioning is idempotent: existing
# secrets are removed and recreated (Req 2.6).
#
# Requirements: 2.4, 2.6, 2.7
# =============================================================================

# SSH configuration
SSH_KEY="${SSH_KEY:-$HOME/.ssh/oci_agent_coder}"
SSH_USER="${SSH_USER:-opc}"
SSH_HOST="${SSH_HOST:-}"
SSH_TARGET=""
SSH_OPTS=""

# Required secrets (from agent/config.go and .env.example)
REQUIRED_SECRETS=(
    OPENROUTER_API_KEY
    OPENROUTER_MODEL
    OPENROUTER_BASE_URL
    OPENROUTER_MAX_TOKENS
    OPENROUTER_TIMEOUT
)

# Default env file path
ENV_FILE=".env"

# ---------------------------------------------------------------------------
# parse_env — Read .env file, skip comments and blank lines, output KEY=VALUE
#   $1 = path to .env file (default: .env)
#   Outputs one KEY=VALUE per line to stdout.
#   Skips blank lines, comment lines (# ...), and malformed lines.
# ---------------------------------------------------------------------------
parse_env() {
    local env_file="${1:-.env}"

    if [[ ! -f "$env_file" ]]; then
        echo "Error: .env file not found: $env_file" >&2
        return 1
    fi

    while IFS= read -r line || [[ -n "$line" ]]; do
        # Skip blank lines
        [[ -z "${line// /}" ]] && continue
        # Skip comment lines (leading whitespace + #)
        [[ "$line" =~ ^[[:space:]]*# ]] && continue
        # Trim leading whitespace
        line="${line#"${line%%[![:space:]]*}"}"
        # Trim trailing whitespace
        line="${line%"${line##*[![:space:]]}"}"
        # Must contain = to be a valid KEY=VALUE pair
        if [[ "$line" == *=* ]]; then
            echo "$line"
        else
            echo "Warning: skipping malformed line: $line" >&2
        fi
    done < "$env_file"
}

# ---------------------------------------------------------------------------
# provision_secret — Create a single podman secret on the remote instance
#   $1 = secret key name
#   $2 = secret value
#   Removes existing secret first (idempotent), then creates it via SSH.
#   The value is piped through stdin so it never appears in process args.
# ---------------------------------------------------------------------------
provision_secret() {
    local key="$1"
    local value="$2"
    local tmp_out
    tmp_out=$(mktemp /tmp/secret-provision-XXXXXX.txt)

    echo "  Provisioning secret: $key"

    # Remove existing secret (ignore error if it doesn't exist)
    # Redirect stdin from /dev/null to prevent ssh from consuming the while-read loop's stdin
    ssh $SSH_OPTS "$SSH_TARGET" "podman secret rm $key 2>/dev/null || true" < /dev/null > "$tmp_out" 2>&1
    cat "$tmp_out"

    # Create the secret — pipe value via stdin so it doesn't appear in args
    # Use printf to avoid issues with values containing backslashes or newlines
    printf '%s' "$value" | ssh $SSH_OPTS "$SSH_TARGET" "podman secret create $key -" > "$tmp_out" 2>&1
    local exit_code=$?
    cat "$tmp_out"

    rm -f "$tmp_out"

    if [[ $exit_code -ne 0 ]]; then
        echo "Error: failed to create secret $key" >&2
        return 1
    fi
}

# ---------------------------------------------------------------------------
# verify_secrets — List podman secrets on instance, confirm all required exist
#   Uses REQUIRED_SECRETS array. Returns 0 if all present, 1 if any missing.
# ---------------------------------------------------------------------------
verify_secrets() {
    local tmp_out
    tmp_out=$(mktemp /tmp/secret-verify-XXXXXX.txt)

    echo "Verifying secrets on remote instance..."

    ssh $SSH_OPTS "$SSH_TARGET" "podman secret ls --format '{{.Name}}'" > "$tmp_out" 2>&1
    local exit_code=$?

    if [[ $exit_code -ne 0 ]]; then
        echo "Error: failed to list secrets on remote instance" >&2
        cat "$tmp_out" >&2
        rm -f "$tmp_out"
        return 1
    fi

    local remote_secrets
    remote_secrets=$(cat "$tmp_out")
    rm -f "$tmp_out"

    local missing=()
    for key in "${REQUIRED_SECRETS[@]}"; do
        if ! echo "$remote_secrets" | grep -q "^${key}$"; then
            missing+=("$key")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Error: missing required secrets:" >&2
        for key in "${missing[@]}"; do
            echo "  - $key" >&2
        done
        return 1
    fi

    echo "All required secrets verified:"
    for key in "${REQUIRED_SECRETS[@]}"; do
        echo "  ✓ $key"
    done
}

# ---------------------------------------------------------------------------
# main — Parse flags, provision secrets, verify
# ---------------------------------------------------------------------------
main() {
    # Parse flags
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --env-file)
                ENV_FILE="${2:?--env-file requires a path argument}"
                shift 2
                ;;
            -h|--help)
                echo "Usage: setup-secrets.sh [--env-file <path>]"
                echo ""
                echo "Provision podman secrets on OCI instance from local .env file."
                echo ""
                echo "Options:"
                echo "  --env-file <path>  Path to .env file (default: .env)"
                echo "  -h, --help         Show this help message"
                exit 0
                ;;
            *)
                echo "Error: unknown flag: $1" >&2
                exit 1
                ;;
        esac
    done

    # Validate SSH_HOST is set
    if [[ -z "$SSH_HOST" ]]; then
        echo "Error: SSH_HOST must be set" >&2
        exit 1
    fi

    # Initialize SSH variables that depend on SSH_HOST
    SSH_TARGET="${SSH_USER}@${SSH_HOST}"
    SSH_OPTS="-i ${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10"

    echo "=== Setup Secrets ==="
    echo "Env file: $ENV_FILE"
    echo "Target:   $SSH_TARGET"
    echo ""

    # Parse the env file
    echo "Parsing $ENV_FILE..."
    local pairs
    pairs=$(parse_env "$ENV_FILE")

    if [[ -z "$pairs" ]]; then
        echo "Error: no key-value pairs found in $ENV_FILE" >&2
        exit 1
    fi

    # Provision each secret
    # Use fd 3 to avoid ssh consuming the while-read stdin
    echo ""
    echo "Provisioning secrets..."
    while IFS='=' read -r key value <&3; do
        # Only provision secrets that are in the required list
        for req in "${REQUIRED_SECRETS[@]}"; do
            if [[ "$key" == "$req" ]]; then
                provision_secret "$key" "$value"
                break
            fi
        done
    done 3<<< "$pairs"

    # Verify all required secrets exist
    echo ""
    verify_secrets

    echo ""
    echo "=== Secret setup complete ==="
}

# Run main only when executed directly (not when sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
