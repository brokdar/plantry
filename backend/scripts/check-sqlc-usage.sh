#!/usr/bin/env bash
# Fails when a sqlc query has no caller outside tests and generated code:
# every `-- name: X` in internal/adapters/sqlite/queries/*.sql needs a `.X(`
# call on the sqlc query object (`q.X(`, `qtx.X(`, `queriesFor(ctx).X(`) in a non-test .go
# file of internal/adapters/sqlite, so same-named repo methods can't hide it.
set -euo pipefail
cd "$(dirname "$0")/.."

unused=0
for name in $(sed -nE 's/^-- name: ([A-Za-z0-9_]+) .*/\1/p' internal/adapters/sqlite/queries/*.sql); do
  if ! grep -rqE --include='*.go' --exclude='*_test.go' --exclude-dir=sqlcgen "\b(q|qtx|queriesFor\([a-zA-Z]*\))\.${name}\(" internal/adapters/sqlite; then
    echo "unused sqlc query: ${name}"
    unused=1
  fi
done
exit "$unused"
