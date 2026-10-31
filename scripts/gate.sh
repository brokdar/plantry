#!/usr/bin/env bash
# Merge gate: backend and frontend checks in parallel, then e2e.
#
#   scripts/gate.sh                 # backend + frontend + full e2e
#   scripts/gate.sh shop cook-mode  # backend + frontend + only these e2e specs
#   GATE_SKIP_BACKEND=1 / GATE_SKIP_FRONTEND=1 / GATE_SKIP_E2E=1 to skip a part
#
# Set E2E_LANE (1–9) when several worktrees run e2e at once (see
# frontend/playwright.config.ts). Logs land in $GATE_LOG_DIR (default
# /tmp/plantry-gate-<lane>); the last lines of each are printed on exit.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG="${GATE_LOG_DIR:-/tmp/plantry-gate-${E2E_LANE:-0}}"
mkdir -p "$LOG"
export GOWORK=off

backend() {
  cd "$ROOT/backend" &&
    go test -race -p 4 ./... &&
    go vet ./... &&
    golangci-lint run &&
    if [ -x scripts/check-sqlc-usage.sh ]; then scripts/check-sqlc-usage.sh; fi
}

frontend() {
  cd "$ROOT/frontend" && bun install --frozen-lockfile >/dev/null &&
    bun run check && bun run build
}

status=0
pids=()
names=()
if [ -z "${GATE_SKIP_BACKEND:-}" ]; then
  backend >"$LOG/backend.log" 2>&1 &
  pids+=($!); names+=(backend)
fi
if [ -z "${GATE_SKIP_FRONTEND:-}" ]; then
  frontend >"$LOG/frontend.log" 2>&1 &
  pids+=($!); names+=(frontend)
fi
for i in "${!pids[@]}"; do
  if wait "${pids[$i]}"; then echo "PASS ${names[$i]}"; else
    echo "FAIL ${names[$i]} (log: $LOG/${names[$i]}.log)"; status=1
  fi
done

if [ "$status" -eq 0 ] && [ -z "${GATE_SKIP_E2E:-}" ]; then
  if (cd "$ROOT/frontend" && bun run e2e -- "$@") >"$LOG/e2e.log" 2>&1; then
    echo "PASS e2e ${*:-(full)}"
  else
    echo "FAIL e2e (log: $LOG/e2e.log)"; status=1
  fi
fi

echo "--- summaries"
grep -hE '^(ok|FAIL|---)|^\s*(Test Files|Tests)\s|[0-9]+ (passed|failed|flaky|skipped)|^[0-9]+ issues?\.' \
  "$LOG"/*.log 2>/dev/null | grep -vE '^ok\s' | tail -40
exit "$status"
