#!/usr/bin/env bash
set -euo pipefail

# =============================================================================
# deploy.sh — Build, transfer, and manage gocoder containers on OCI instance
#
# Usage: deploy.sh <subcommand> [flags]
#
# Subcommands:
#   deploy    Build, transfer, and load the image (does not start containers)
#   start     Start a new project-scoped container instance
#   run       Execute the agent inside a running container instance
#   list      List all running gocoder container instances
#   stop      Stop and remove a container instance by project name
#
# Requirements: 3.1, 3.6, 5.1, 6.1, 6.4, 6.8, 6.9, 10.4
# =============================================================================

# ---------------------------------------------------------------------------
# Configuration variables
# ---------------------------------------------------------------------------
SSH_KEY="${SSH_KEY:-$HOME/.ssh/oci_agent_coder}"
SSH_USER="${SSH_USER:-opc}"
SSH_HOST="${SSH_HOST:-}"
IMAGE_NAME="${IMAGE_NAME:-gocoder}"
DEPLOY_DIR="${DEPLOY_DIR:-~/deploy/gocoder}"
SERVER_PORT="${SERVER_PORT:-8080}"
BUILD_TARGET="${BUILD_TARGET:-./cmd/agent}"

# SSH helper variables (initialized after SSH_HOST validation)
SSH_TARGET=""
SSH_OPTS=""

# Required secrets (from agent/config.go)
REQUIRED_SECRETS=(
    OPENROUTER_API_KEY
    OPENROUTER_MODEL
    OPENROUTER_BASE_URL
    OPENROUTER_MAX_TOKENS
    OPENROUTER_TIMEOUT
)

# ---------------------------------------------------------------------------
# init_ssh — Initialize SSH variables after config is set
# ---------------------------------------------------------------------------
init_ssh() {
    if [[ -z "$SSH_HOST" ]]; then
        echo "Error: SSH_HOST must be set" >&2
        exit 1
    fi
    SSH_TARGET="${SSH_USER}@${SSH_HOST}"
    SSH_OPTS="-i ${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10"
}

# ---------------------------------------------------------------------------
# usage — Print help and exit
# ---------------------------------------------------------------------------
usage() {
    cat <<'EOF'
Usage: deploy.sh <subcommand> [flags]

Subcommands:
  deploy                Build, transfer, and load the image (does not start containers)
  start                 Start a new project-scoped container instance
  run                   Execute the agent inside a running container instance
  list                  List all running gocoder container instances
  stop                  Stop and remove a container instance by project name

Flags (deploy):
  --skip-build          Skip the build phase, reuse existing archive
  --dry-run             Print commands without executing

Flags (start):
  --project <name>      Project identifier (required, used for container name)
  --repo <url>          Git repository URL to clone (repeatable for multiple repos)
  --replace             Stop and replace existing instance with same project name
  --mode <cli|server>   Container execution mode (default: cli)
  --port <number>       Port mapping for server mode (default: 8080)
  --dry-run             Print commands without executing

Flags (run):
  --project <name>      Project identifier (required)
  --story <path>        Path to story file on OCI instance
  --context <path>      Path to context file on OCI instance
  --output <path>       Output file path (optional)
  --dry-run             Print commands without executing

Flags (stop):
  --project <name>      Project identifier (required)
  --dry-run             Print commands without executing

Flags (list):
  --dry-run             Print commands without executing

Environment:
  SSH_KEY               Path to SSH private key (default: ~/.ssh/oci_agent_coder)
  SSH_USER              Remote user (default: opc)
  SSH_HOST              OCI instance IP (required)
  IMAGE_NAME            Container image name (default: gocoder)
  DEPLOY_DIR            Remote deployment directory (default: ~/deploy/gocoder)
  BUILD_TARGET          Go build target path (default: ./cmd/agent)
EOF
    exit 0
}

# ---------------------------------------------------------------------------
# Function stubs — implementations added in subsequent tasks
# ---------------------------------------------------------------------------

# Build the container image for linux/arm64, tag with git SHA + latest
# Saves to .tar archive. Supports --skip-build to skip this phase.
# Requirements: 3.1, 3.2, 3.3, 3.4, 3.5
build_image() {
    local skip_build="${1:-false}"

    # Detect container engine (podman preferred over docker in WSL)
    local engine=""
    if command -v podman &>/dev/null; then
        engine="podman"
    elif command -v docker &>/dev/null; then
        engine="docker"
    else
        echo "Error [build]: neither podman nor docker found" >&2
        exit 1
    fi

    # Determine version tag: git short SHA or timestamp fallback
    local version_tag
    if command -v git &>/dev/null && git rev-parse --short HEAD &>/dev/null 2>&1; then
        version_tag=$(git rev-parse --short HEAD)
    else
        version_tag=$(date +"%Y%m%d-%H%M%S")
    fi

    IMAGE_TAG="$version_tag"
    IMAGE_ARCHIVE="${IMAGE_NAME}-${version_tag}.tar"

    # Skip build if requested — look for existing archive
    if [[ "$skip_build" == true ]]; then
        if [[ -f "$IMAGE_ARCHIVE" ]]; then
            echo "Skipping build, reusing existing archive: $IMAGE_ARCHIVE"
            return 0
        fi
        # Also check for any existing archive matching the image name
        local existing
        existing=$(ls ${IMAGE_NAME}-*.tar 2>/dev/null | head -1 || true)
        if [[ -n "$existing" ]]; then
            IMAGE_ARCHIVE="$existing"
            # Extract version tag from filename
            IMAGE_TAG="${existing#${IMAGE_NAME}-}"
            IMAGE_TAG="${IMAGE_TAG%.tar}"
            echo "Skipping build, reusing existing archive: $IMAGE_ARCHIVE"
            return 0
        fi
        echo "Error [build]: --skip-build specified but no existing archive found" >&2
        exit 1
    fi

    # Verify Containerfile exists
    if [[ ! -f "Containerfile" ]]; then
        echo "Error [build]: Containerfile not found in project root" >&2
        exit 1
    fi

    # Build the image for linux/arm64
    echo "Building image ${IMAGE_NAME}:${version_tag} for linux/arm64..."
    if ! $engine build \
        --platform linux/arm64 \
        --build-arg BUILD_TARGET="$BUILD_TARGET" \
        -t "${IMAGE_NAME}:${version_tag}" \
        -t "${IMAGE_NAME}:latest" \
        -f Containerfile . 2>&1; then
        echo "Error [build]: container build failed" >&2
        exit 1
    fi

    # Save image to tar archive
    echo "Saving image to ${IMAGE_ARCHIVE}..."
    if ! $engine save -o "$IMAGE_ARCHIVE" "${IMAGE_NAME}:${version_tag}" "${IMAGE_NAME}:latest" 2>&1; then
        echo "Error [build]: failed to save image archive" >&2
        exit 1
    fi

    echo "Build complete: ${IMAGE_ARCHIVE}"
}

# Transfer image archive to OCI instance via scp, load with podman load
# Requirements: 4.1, 4.2, 4.3, 4.4, 4.5, 4.6
transfer_image() {
    # Verify the local archive exists
    if [[ ! -f "$IMAGE_ARCHIVE" ]]; then
        echo "Error [transfer]: image archive not found: $IMAGE_ARCHIVE" >&2
        exit 1
    fi

    local archive_basename
    archive_basename=$(basename "$IMAGE_ARCHIVE")

    # Ensure remote deploy directory and logs directory exist
    echo "Ensuring remote directories exist..."
    if ! ssh $SSH_OPTS "$SSH_TARGET" "mkdir -p $DEPLOY_DIR $DEPLOY_DIR/logs" > /tmp/transfer_mkdir.txt 2>&1; then
        cat /tmp/transfer_mkdir.txt >&2
        echo "Error [transfer]: failed to create remote directories" >&2
        exit 1
    fi
    cat /tmp/transfer_mkdir.txt

    # Transfer archive via scp
    echo "Transferring $IMAGE_ARCHIVE to $SSH_TARGET:$DEPLOY_DIR/..."
    if ! scp $SSH_OPTS "$IMAGE_ARCHIVE" "$SSH_TARGET:$DEPLOY_DIR/" > /tmp/transfer_scp.txt 2>&1; then
        cat /tmp/transfer_scp.txt >&2
        echo "Error [transfer]: scp transfer failed" >&2
        exit 1
    fi
    cat /tmp/transfer_scp.txt
    echo "Transfer complete."

    # Load image via podman load using nohup + log polling (ARM64 is slow)
    echo "Loading image on remote host (this may take a while on ARM64)..."
    local load_log="$DEPLOY_DIR/logs/load.log"

    # Clear any previous load log
    ssh $SSH_OPTS "$SSH_TARGET" "rm -f $load_log" > /dev/null 2>&1 || true

    # Kick off podman load in background with nohup
    ssh $SSH_OPTS "$SSH_TARGET" "nohup bash -c 'podman load -i $DEPLOY_DIR/$archive_basename > $load_log 2>&1; echo DONE >> $load_log' &>/dev/null &"

    # Poll for completion
    local poll_result=""
    local max_polls=120  # 10 minutes at 5-second intervals
    local poll_count=0
    while [[ $poll_count -lt $max_polls ]]; do
        sleep 5
        poll_result=$(ssh $SSH_OPTS "$SSH_TARGET" "tail -1 $load_log 2>/dev/null" 2>/dev/null || true)
        if [[ "$poll_result" == "DONE" ]]; then
            break
        fi
        poll_count=$((poll_count + 1))
    done

    if [[ "$poll_result" != "DONE" ]]; then
        echo "Error [load]: podman load timed out or failed" >&2
        # Retrieve load log for diagnostics
        ssh $SSH_OPTS "$SSH_TARGET" "cat $load_log" > /tmp/transfer_load.txt 2>&1 || true
        cat /tmp/transfer_load.txt >&2
        exit 1
    fi

    # Retrieve and display load output (excluding the DONE marker)
    ssh $SSH_OPTS "$SSH_TARGET" "head -n -1 $load_log" > /tmp/transfer_load.txt 2>&1
    cat /tmp/transfer_load.txt

    # Check if podman load reported an error in its output
    if grep -qi "error" /tmp/transfer_load.txt 2>/dev/null; then
        echo "Error [load]: podman load reported an error" >&2
        cat /tmp/transfer_load.txt >&2
        exit 1
    fi

    # Remove archive from remote to conserve disk space
    echo "Removing remote archive..."
    ssh $SSH_OPTS "$SSH_TARGET" "rm -f $DEPLOY_DIR/$archive_basename" > /tmp/transfer_rm.txt 2>&1 || true
    cat /tmp/transfer_rm.txt

    echo "Image transfer and load complete."
}

# Verify each required podman secret exists on the OCI instance
# Requirements: 2.5
check_secrets() {
    echo "Checking required podman secrets on remote host..."

    # List existing podman secrets on the instance
    local tmp_secrets="/tmp/check_secrets_list.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" "podman secret ls --format '{{.Name}}'" > "$tmp_secrets" 2>&1; then
        cat "$tmp_secrets" >&2
        echo "Error [secrets]: failed to list podman secrets on remote host" >&2
        return 1
    fi

    # Read existing secrets into an array
    local existing_secrets
    existing_secrets=$(cat "$tmp_secrets")

    # Check each required secret against the list
    local missing=()
    for secret in "${REQUIRED_SECRETS[@]}"; do
        if ! echo "$existing_secrets" | grep -qx "$secret"; then
            missing+=("$secret")
        fi
    done

    # Report results
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "Error [secrets]: missing required podman secrets:" >&2
        for m in "${missing[@]}"; do
            echo "  - $m" >&2
        done
        return 1
    fi

    echo "All required podman secrets are present."
}

# Start a project-scoped container instance with podman run -d
# Requirements: 5.1, 5.2, 5.3, 5.4, 5.6, 5.7, 5.8, 5.9, 8.1, 8.2, 8.3, 8.5, 9.2, 9.3, 9.4
start_container() {
    local project="${1:?start_container requires a project name}"
    local replace="${2:-false}"
    local mode="${3:-cli}"
    local port="${4:-$SERVER_PORT}"
    shift 4 || true
    local repos=("$@")

    local container_name="gocoder-${project}"

    # Create per-project directories on the remote host
    ensure_dirs "$project"

    # Check if a container with this name already exists
    echo "Checking for existing container '${container_name}'..."
    local tmp_check="/tmp/start_container_check.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" \
        "podman ps -a --filter name=^${container_name}\$ --format '{{.Names}}'" \
        > "$tmp_check" 2>&1; then
        cat "$tmp_check" >&2
        echo "Error [run]: failed to check existing containers" >&2
        return 1
    fi

    local existing
    existing=$(cat "$tmp_check" | grep -x "$container_name" || true)

    if [[ -n "$existing" ]]; then
        if [[ "$replace" == true ]]; then
            echo "Replacing existing container '${container_name}'..."
            local tmp_stop="/tmp/start_container_stop.txt"
            if ! ssh $SSH_OPTS "$SSH_TARGET" "podman stop ${container_name}" \
                > "$tmp_stop" 2>&1; then
                cat "$tmp_stop" >&2
                echo "Warning [run]: failed to stop container (may already be stopped)" >&2
            fi
            cat "$tmp_stop"

            local tmp_rm="/tmp/start_container_rm.txt"
            if ! ssh $SSH_OPTS "$SSH_TARGET" "podman rm ${container_name}" \
                > "$tmp_rm" 2>&1; then
                cat "$tmp_rm" >&2
                echo "Error [run]: failed to remove existing container '${container_name}'" >&2
                return 1
            fi
            cat "$tmp_rm"
            echo "Existing container removed."
        else
            echo "Warning: container '${container_name}' already exists. Use --replace to stop and replace it." >&2
            return 0
        fi
    fi

    # Build the podman run command
    local run_cmd="podman run -d"
    run_cmd+=" --name ${container_name}"
    run_cmd+=" --restart=on-failure"

    # Inject each required secret as an environment variable
    for secret in "${REQUIRED_SECRETS[@]}"; do
        run_cmd+=" --secret ${secret},type=env"
    done

    # Pass repo URLs as a comma-separated environment variable
    if [[ ${#repos[@]} -gt 0 ]]; then
        local repo_urls
        repo_urls=$(IFS=','; echo "${repos[*]}")
        run_cmd+=" -e REPO_URLS=${repo_urls}"
    fi

    # Mount the project workspace volume
    run_cmd+=" -v ${DEPLOY_DIR}/projects/${project}/workspace:/workspace:Z"

    # Server mode: add port mapping
    if [[ "$mode" == "server" ]]; then
        run_cmd+=" -p ${port}:${port}"
    fi

    # Image
    run_cmd+=" ${IMAGE_NAME}:latest"

    # Start the container via SSH
    echo "Starting container '${container_name}' (mode=${mode})..."
    local tmp_run="/tmp/start_container_run.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" "$run_cmd" > "$tmp_run" 2>&1; then
        cat "$tmp_run" >&2
        echo "Error [run]: failed to start container '${container_name}'" >&2
        return 1
    fi
    cat "$tmp_run"

    echo "Container '${container_name}' started successfully."
}

# Execute the agent inside a running container via podman exec
# Requirements: 6.1, 6.5, 6.6, 6.7
run_exec() {
    local project="${1:?run_exec requires a project name}"
    local story="${2:-}"
    local context="${3:-}"
    local output="${4:-}"

    local container_name="gocoder-${project}"

    # Check if the container is running
    local tmp_check="/tmp/run_exec_check.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" \
        "podman ps --filter name=^${container_name}\$ --filter status=running --format '{{.Names}}'" \
        > "$tmp_check" 2>&1; then
        cat "$tmp_check" >&2
        echo "Error [exec]: failed to query container status" >&2
        return 1
    fi

    local running
    running=$(cat "$tmp_check" | grep -x "$container_name" || true)

    if [[ -z "$running" ]]; then
        echo "Error [exec]: Container ${container_name} is not running" >&2
        return 1
    fi

    # Build the podman exec command
    local exec_cmd="podman exec ${container_name} gocoder"
    if [[ -n "$story" ]]; then
        exec_cmd+=" --story ${story}"
    fi
    if [[ -n "$context" ]]; then
        exec_cmd+=" --context ${context}"
    fi
    if [[ -n "$output" ]]; then
        exec_cmd+=" --output ${output}"
    fi

    # Execute via SSH, capture output to temp file
    echo "Executing agent in container '${container_name}'..."
    local tmp_exec="/tmp/run_exec_output.txt"
    local exec_exit=0
    ssh $SSH_OPTS "$SSH_TARGET" "$exec_cmd" > "$tmp_exec" 2>&1 || exec_exit=$?

    # Display output locally
    cat "$tmp_exec"

    # On non-zero exit: retrieve container logs, print to stderr, propagate exit code
    if [[ $exec_exit -ne 0 ]]; then
        echo "Error [exec]: agent exited with code ${exec_exit}" >&2
        echo "Retrieving container logs..." >&2
        local tmp_logs="/tmp/run_exec_logs.txt"
        ssh $SSH_OPTS "$SSH_TARGET" "podman logs ${container_name}" > "$tmp_logs" 2>&1 || true
        cat "$tmp_logs" >&2
        return "$exec_exit"
    fi

    echo "Agent execution complete."
}

# List all running gocoder container instances
# Requirements: 6.8
list_instances() {
    echo "Listing gocoder container instances..."

    local tmp_list="/tmp/list_instances_out.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" \
        "podman ps -a --filter name=gocoder- --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'" \
        > "$tmp_list" 2>&1; then
        cat "$tmp_list" >&2
        echo "Error [list]: failed to list container instances" >&2
        return 1
    fi
    cat "$tmp_list"
}

# Stop and remove a container instance by project name
# Requirements: 6.9
stop_container() {
    local project="${1:?stop_container requires a project name}"
    local container_name="gocoder-${project}"

    echo "Stopping container '${container_name}'..."
    local tmp_stop="/tmp/stop_container_stop.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" "podman stop ${container_name}" \
        > "$tmp_stop" 2>&1; then
        cat "$tmp_stop" >&2
        echo "Error [stop]: failed to stop container '${container_name}'" >&2
        return 1
    fi
    cat "$tmp_stop"

    echo "Removing container '${container_name}'..."
    local tmp_rm="/tmp/stop_container_rm.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" "podman rm ${container_name}" \
        > "$tmp_rm" 2>&1; then
        cat "$tmp_rm" >&2
        echo "Error [stop]: failed to remove container '${container_name}'" >&2
        return 1
    fi
    cat "$tmp_rm"

    echo "Container '${container_name}' stopped and removed."
}

# Validate deployment: verify image loaded, print summary, log operations
# Requirements: 10.1, 10.2, 10.5
validate_deploy() {
    echo "Validating deployment..."

    # Verify the image is loaded on the remote host
    local tmp_images="/tmp/validate_deploy_images.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" \
        "podman images --filter reference=$IMAGE_NAME --format '{{.Repository}}:{{.Tag}}'" \
        > "$tmp_images" 2>&1; then
        cat "$tmp_images" >&2
        echo "Error [validate]: failed to query podman images on remote host" >&2
        return 1
    fi

    local loaded_images
    loaded_images=$(cat "$tmp_images")

    if [[ -z "$loaded_images" ]]; then
        echo "Error [validate]: image $IMAGE_NAME not found on remote host" >&2
        return 1
    fi

    # Build deployment summary
    local deploy_timestamp
    deploy_timestamp=$(date +"%Y-%m-%d %H:%M:%S %Z")

    local summary=""
    summary+="=== Deployment Summary ===\n"
    summary+="Image tag:        ${IMAGE_NAME}:${IMAGE_TAG}\n"
    summary+="Deployment time:  ${deploy_timestamp}\n"
    summary+="Target host:      ${SSH_HOST}\n"
    summary+="Deploy directory: ${DEPLOY_DIR}\n"
    summary+="=========================="

    # Print summary locally
    echo -e "$summary"

    # Log summary to deploy.log on the remote host
    local tmp_log="/tmp/validate_deploy_log.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" \
        "mkdir -p $DEPLOY_DIR/logs && echo -e '$summary' >> $DEPLOY_DIR/logs/deploy.log" \
        > "$tmp_log" 2>&1; then
        cat "$tmp_log" >&2
        echo "Error [validate]: failed to write deploy log on remote host" >&2
        return 1
    fi
    cat "$tmp_log"

    echo "Deployment validated successfully."
}

# Create required directories on OCI instance
# Requirements: 7.1, 7.2, 7.3, 7.4, 7.5
ensure_dirs() {
    local project="${1:?ensure_dirs requires a project name}"

    echo "Ensuring deployment directories exist for project '$project'..."

    local tmp_dirs="/tmp/ensure_dirs_out.txt"
    if ! ssh $SSH_OPTS "$SSH_TARGET" "mkdir -p \
        $DEPLOY_DIR \
        $DEPLOY_DIR/logs \
        $DEPLOY_DIR/projects/$project/workspace \
        $DEPLOY_DIR/projects/$project/logs" > "$tmp_dirs" 2>&1; then
        cat "$tmp_dirs" >&2
        echo "Error [dirs]: failed to create deployment directories on remote host" >&2
        return 1
    fi
    cat "$tmp_dirs"

    echo "Deployment directories ready."
}

# ---------------------------------------------------------------------------
# cmd_deploy — Build, transfer, and load the image
# ---------------------------------------------------------------------------
cmd_deploy() {
    local skip_build=false
    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-build)
                skip_build=true
                shift
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Error: unknown flag for deploy: $1" >&2
                exit 1
                ;;
        esac
    done

    init_ssh

    if [[ "$dry_run" == true ]]; then
        echo "[dry-run] Would build image: $IMAGE_NAME (skip_build=$skip_build)"
        echo "[dry-run] Would transfer image to $SSH_TARGET:$DEPLOY_DIR/"
        echo "[dry-run] Would load image via podman load on $SSH_HOST"
        echo "[dry-run] Would check secrets on $SSH_HOST"
        echo "[dry-run] Would validate deployment on $SSH_HOST"
        return 0
    fi

    build_image "$skip_build"
    transfer_image
    check_secrets
    validate_deploy
}

# ---------------------------------------------------------------------------
# cmd_start — Start a new project-scoped container instance
# ---------------------------------------------------------------------------
cmd_start() {
    local project=""
    local repos=()
    local replace=false
    local mode="cli"
    local port="$SERVER_PORT"
    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --project)
                project="${2:?--project requires a name argument}"
                shift 2
                ;;
            --repo)
                repos+=("${2:?--repo requires a URL argument}")
                shift 2
                ;;
            --replace)
                replace=true
                shift
                ;;
            --mode)
                mode="${2:?--mode requires a value (cli|server)}"
                shift 2
                ;;
            --port)
                port="${2:?--port requires a number argument}"
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Error: unknown flag for start: $1" >&2
                exit 1
                ;;
        esac
    done

    if [[ -z "$project" ]]; then
        echo "Error: --project is required for start" >&2
        exit 1
    fi

    init_ssh

    if [[ "$dry_run" == true ]]; then
        local container_name="gocoder-${project}"
        echo "[dry-run] Would create directories on $SSH_HOST for project '$project'"
        echo "[dry-run] Would check for existing container '${container_name}' on $SSH_HOST"
        if [[ "$replace" == true ]]; then
            echo "[dry-run] Would stop and remove existing container '${container_name}' if present (--replace)"
        fi
        local run_cmd="podman run -d --name ${container_name} --restart=on-failure"
        for secret in "${REQUIRED_SECRETS[@]}"; do
            run_cmd+=" --secret ${secret},type=env"
        done
        if [[ ${#repos[@]} -gt 0 ]]; then
            local repo_urls
            repo_urls=$(IFS=','; echo "${repos[*]}")
            run_cmd+=" -e REPO_URLS=${repo_urls}"
        fi
        run_cmd+=" -v ${DEPLOY_DIR}/projects/${project}/workspace:/workspace:Z"
        if [[ "$mode" == "server" ]]; then
            run_cmd+=" -p ${port}:${port}"
        fi
        run_cmd+=" ${IMAGE_NAME}:latest"
        echo "[dry-run] Would execute on $SSH_HOST: $run_cmd"
        return 0
    fi

    start_container "$project" "$replace" "$mode" "$port" "${repos[@]+"${repos[@]}"}"
}

# ---------------------------------------------------------------------------
# cmd_run — Execute the agent inside a running container instance
# ---------------------------------------------------------------------------
cmd_run() {
    local project=""
    local story=""
    local context=""
    local output=""
    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --project)
                project="${2:?--project requires a name argument}"
                shift 2
                ;;
            --story)
                story="${2:?--story requires a path argument}"
                shift 2
                ;;
            --context)
                context="${2:?--context requires a path argument}"
                shift 2
                ;;
            --output)
                output="${2:?--output requires a path argument}"
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Error: unknown flag for run: $1" >&2
                exit 1
                ;;
        esac
    done

    if [[ -z "$project" ]]; then
        echo "Error: --project is required for run" >&2
        exit 1
    fi

    init_ssh

    if [[ "$dry_run" == true ]]; then
        local container_name="gocoder-${project}"
        local exec_cmd="podman exec ${container_name} gocoder"
        if [[ -n "$story" ]]; then
            exec_cmd+=" --story ${story}"
        fi
        if [[ -n "$context" ]]; then
            exec_cmd+=" --context ${context}"
        fi
        if [[ -n "$output" ]]; then
            exec_cmd+=" --output ${output}"
        fi
        echo "[dry-run] Would check if container '${container_name}' is running on $SSH_HOST"
        echo "[dry-run] Would execute on $SSH_HOST: $exec_cmd"
        return 0
    fi

    run_exec "$project" "$story" "$context" "$output"
}

# ---------------------------------------------------------------------------
# cmd_list — List all running gocoder container instances
# ---------------------------------------------------------------------------
cmd_list() {
    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Error: unknown flag for list: $1" >&2
                exit 1
                ;;
        esac
    done

    init_ssh

    if [[ "$dry_run" == true ]]; then
        echo "[dry-run] Would list gocoder containers on $SSH_HOST"
        return 0
    fi

    list_instances
}

# ---------------------------------------------------------------------------
# cmd_stop — Stop and remove a container instance by project name
# ---------------------------------------------------------------------------
cmd_stop() {
    local project=""
    local dry_run=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --project)
                project="${2:?--project requires a name argument}"
                shift 2
                ;;
            --dry-run)
                dry_run=true
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Error: unknown flag for stop: $1" >&2
                exit 1
                ;;
        esac
    done

    if [[ -z "$project" ]]; then
        echo "Error: --project is required for stop" >&2
        exit 1
    fi

    init_ssh

    if [[ "$dry_run" == true ]]; then
        local container_name="gocoder-${project}"
        echo "[dry-run] Would execute on $SSH_HOST: podman stop ${container_name}"
        echo "[dry-run] Would execute on $SSH_HOST: podman rm ${container_name}"
        return 0
    fi

    stop_container "$project"
}

# ---------------------------------------------------------------------------
# main — Subcommand dispatch
# ---------------------------------------------------------------------------
main() {
    if [[ $# -eq 0 ]]; then
        usage
    fi

    local subcommand="$1"
    shift

    case "$subcommand" in
        deploy)
            cmd_deploy "$@"
            ;;
        start)
            cmd_start "$@"
            ;;
        run)
            cmd_run "$@"
            ;;
        list)
            cmd_list "$@"
            ;;
        stop)
            cmd_stop "$@"
            ;;
        -h|--help|help)
            usage
            ;;
        *)
            echo "Error: unknown subcommand: $subcommand" >&2
            echo "Run 'deploy.sh --help' for usage." >&2
            exit 1
            ;;
    esac
}

# Source guard: only run main when executed directly, not when sourced by tests
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
