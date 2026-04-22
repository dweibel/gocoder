# gocoder

A Go-based AI coding agent that processes Gherkin user stories against a software requirements context using LLMs via OpenRouter. Part of the Autonomous Requirements & Development Pipeline (ARDP).

## Project Structure

```
cmd/agent/       CLI entry point
agent/           Core agent package (conversation loop, config, tools, prompts)
scripts/         Deployment and test scripts
  deploy.sh      Build, transfer, and manage containers on OCI instance
  setup-secrets.sh  Provision podman secrets from local .env
  entrypoint.sh  Container entrypoint (repo cloning)
  tests/         Property-based tests for deployment scripts
Containerfile    Multi-stage build for linux/arm64
```

## Prerequisites

- Go 1.25+
- An [OpenRouter](https://openrouter.ai/) API key

## Configuration

Copy the example env file and set your API key:

```bash
cp .env.example .env
```

| Variable | Required | Default |
|---|---|---|
| `OPENROUTER_API_KEY` | Yes | — |
| `OPENROUTER_MODEL` | No | `anthropic/claude-sonnet-4` |
| `OPENROUTER_BASE_URL` | No | `https://openrouter.ai/api` |
| `OPENROUTER_MAX_TOKENS` | No | `4096` |
| `OPENROUTER_TIMEOUT` | No | `300` |

## Build

```bash
go build -o gocoder ./cmd/agent
```

Cross-compile for ARM64 (used by the container build):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o gocoder ./cmd/agent
```

## Usage

```bash
./gocoder --story path/to/story.md --context path/to/context.md
```

Flags:

| Flag | Description |
|---|---|
| `--story` | Path to Gherkin story file |
| `--context` | Path to SRS context file |
| `--output` | Output file path (default: stdout) |
| `--model` | Override model string |
| `--timeout` | Timeout in seconds (default: 300) |

## Tests

```bash
go test ./...
```

Property-based tests for deployment scripts:

```bash
for t in scripts/tests/test_property_*.sh; do bash "$t"; done
```

## Container Deployment

The project includes a containerized deployment workflow targeting an OCI Always Free ARM64 instance. The container runs as a long-lived, project-scoped instance that clones repos at startup and accepts multiple agent execution requests via `podman exec`.

```bash
# Provision secrets on the OCI instance
SSH_HOST=<ip> bash scripts/setup-secrets.sh

# Build and deploy the image
SSH_HOST=<ip> bash scripts/deploy.sh deploy

# Start a project instance
SSH_HOST=<ip> bash scripts/deploy.sh start --project myapp --repo <git-url>

# Run the agent
SSH_HOST=<ip> bash scripts/deploy.sh run --project myapp --story /workspace/repo/story.md --context /workspace/repo/context.md

# List / stop instances
SSH_HOST=<ip> bash scripts/deploy.sh list
SSH_HOST=<ip> bash scripts/deploy.sh stop --project myapp
```

## Current State

- Milestone 0 (CLI mode): complete and deployed
- Milestone 1 (HTTP server mode): not started

## License

Private — not licensed for redistribution.
