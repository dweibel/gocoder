#!/bin/bash
# Setup a GitHub Actions self-hosted runner on the OCI instance.
#
# Prerequisites:
#   1. Generate a runner registration token at:
#      https://github.com/dweibel/gocoder/settings/actions/runners/new
#   2. SSH into the OCI instance:
#      ssh oci-agent
#
# Usage:
#   curl -sL <raw-url-to-this-script> | bash -s -- <REGISTRATION_TOKEN>
#   — or —
#   bash setup-runner.sh <REGISTRATION_TOKEN>

set -euo pipefail

TOKEN="${1:?Usage: $0 <GITHUB_RUNNER_REGISTRATION_TOKEN>}"
RUNNER_DIR="$HOME/actions-runner"
REPO_URL="https://github.com/dweibel/gocoder"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  aarch64|arm64) RUNNER_ARCH="arm64" ;;
  x86_64)        RUNNER_ARCH="x64"   ;;
  *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "==> Setting up GitHub Actions runner (${RUNNER_ARCH})..."

# Install dependencies
sudo dnf install -y libicu dotnet-runtime-8.0 2>/dev/null || \
  sudo yum install -y libicu 2>/dev/null || \
  sudo apt-get install -y libicu-dev 2>/dev/null || true

# Download latest runner
mkdir -p "$RUNNER_DIR" && cd "$RUNNER_DIR"
LATEST=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep -oP '"tag_name":\s*"v\K[^"]+')
TARBALL="actions-runner-linux-${RUNNER_ARCH}-${LATEST}.tar.gz"

if [ ! -f ".runner" ]; then
  echo "==> Downloading runner v${LATEST}..."
  curl -sL "https://github.com/actions/runner/releases/download/v${LATEST}/${TARBALL}" -o "$TARBALL"
  tar xzf "$TARBALL"
  rm -f "$TARBALL"
fi

# Configure (non-interactive)
./config.sh --url "$REPO_URL" --token "$TOKEN" --name "oci-arm64" --labels "self-hosted,linux,arm64" --unattended --replace

# Install and start as a systemd user service (no sudo needed)
echo "==> Installing as systemd user service..."
mkdir -p ~/.config/systemd/user

cat > ~/.config/systemd/user/actions-runner.service <<EOF
[Unit]
Description=GitHub Actions Runner
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$RUNNER_DIR
ExecStart=$RUNNER_DIR/run.sh
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable actions-runner
systemctl --user start actions-runner

# Enable lingering so the service runs without an active login session
loginctl enable-linger "$(whoami)"

echo ""
echo "==> Runner installed and running!"
echo "    Status: systemctl --user status actions-runner"
echo "    Logs:   journalctl --user -u actions-runner -f"
