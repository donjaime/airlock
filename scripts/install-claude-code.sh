#!/usr/bin/env bash
set -euo pipefail

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found; cannot install Claude Code automatically."
  exit 0
fi

echo "Installing Claude Code (best-effort) ..."
npm install -g @anthropic-ai/claude-code || {
  echo "WARNING: Failed to install @anthropic-ai/claude-code via npm."
  echo "You can install Claude Code manually inside the container once you enter."
  exit 0
}

echo "Claude Code installed."
