#!/usr/bin/env bash
set -e

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "🚀 Initializing development environment..."

# Ensure bun global bin is in PATH for all shell sessions
export PATH="$HOME/.bun/bin:$PATH"
if ! grep -q 'bun/bin' ~/.bashrc 2>/dev/null; then
    echo 'export PATH="$HOME/.bun/bin:$PATH"' >> ~/.bashrc
fi

curl -fsSL https://claude.ai/install.sh | bash

# Go tools
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
if ! command -v golangci-lint &>/dev/null; then
  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin"
fi

# Bun
if ! command -v bun &>/dev/null; then
  npm install -g bun
fi
bun install --cwd "$REPO_ROOT/frontend"

# Playwright browsers (skip with SKIP_PLAYWRIGHT=true for backend-only contributors)
if [ "${SKIP_PLAYWRIGHT:-false}" != "true" ]; then
  bunx playwright install --with-deps chromium
fi

echo "🤖 Installing agent-browser..."
bun install -g agent-browser

echo "✅ Development environment setup complete!"
