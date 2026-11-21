#!/usr/bin/env bash
# Merge gate, two tiers (see "Test speed contract" in CLAUDE.md).
#
#   scripts/gate.sh --fast [specs]  T1, per fix batch (~1 min): cheap checks,
#                                   go test without race (cache on), frontend
#                                   lint/typecheck/knip + unit tests of changed
#                                   files, e2e only for the named specs
#   scripts/gate.sh [specs]         T2, once per PR / hand-back (~2.5 min): cheap
#                                   checks, then in parallel: go test (no race,
#                                   then -race -short), vitest + build, e2e
#   GATE_SKIP_BACKEND=1 / GATE_SKIP_FRONTEND=1 / GATE_SKIP_E2E=1 skip a part
#
# Cheap checks run first, so a lint or deadcode slip fails in seconds, not
# after the tests. Every step prints its time. Budgets (test-budget.json) are
# checked on the measured CPU and wall time: tests may get slower only in
# proportion to the tests added, or with a written reason. A T2 pass is
# remembered per tree: a rerun on the same clean tree exits at once.
#
# Set E2E_LANE (1-9) when several worktrees run e2e at once (see
# frontend/playwright.config.ts). Logs land in a fresh directory per run.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODE=full
if [ "${1:-}" = "--fast" ]; then MODE=fast; shift; fi
SPECS=("$@")
LOG="$(mktemp -d "/tmp/plantry-gate-${E2E_LANE:-0}-XXXX")"
export GOWORK=off
RACE_FLAGS=(-race -short -gcflags=modernc.org/sqlite/...=-race=false)
CPU_EXEC="$ROOT/scripts/cpu-exec.py"
BUDGET="$ROOT/scripts/test-budget.py"

# T2 pass cache: the same clean tree, mode and specs passed before.
PASS_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/plantry-gate"
mkdir -p "$PASS_DIR"
KEY=""
if [ -z "$(git -C "$ROOT" status --porcelain)" ]; then
  KEY="$(git -C "$ROOT" rev-parse 'HEAD^{tree}')-$MODE-$(printf '%s' "${SPECS[*]:-}" | md5sum | cut -c1-8)"
  if [ -f "$PASS_DIR/$KEY" ]; then
    echo "already green: this tree passed the $MODE gate ($(cat "$PASS_DIR/$KEY"))"
    exit 0
  fi
fi

# One T2 gate at a time on the machine: parallel race runs fight for cores.
if [ "$MODE" = full ]; then
  exec 9>/tmp/plantry-gate.lock
  flock -n 9 || { echo "another full gate is running; waiting for it"; flock 9; }
fi

step() { # step NAME CMD... : run, time, log
  local name=$1 s=$SECONDS; shift
  if "$@" >>"$LOG/$name.log" 2>&1; then
    echo "PASS $name ($((SECONDS - s))s)"
  else
    echo "FAIL $name ($((SECONDS - s))s, log: $LOG/$name.log)"
    return 1
  fi
}

# --- cheap checks (seconds) -------------------------------------------------
checks_backend() {
  cd "$ROOT/backend" &&
    step vet go vet ./... &&
    step golangci-lint golangci-lint run &&
    step build-arm64 env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /dev/null ./cmd/plantry &&
    step build-armv7 env GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o /dev/null ./cmd/plantry &&
    step sqlc-usage bash scripts/check-sqlc-usage.sh &&
    step deadcode deadcode &&
    step test-patterns bash "$ROOT/scripts/check-test-patterns.sh"
}
deadcode() {
  local out
  out="$(go tool deadcode -test ./... | grep -v '/sqlcgen/' || true)"
  if [ -n "$out" ]; then echo "$out"; return 1; fi
}
checks_frontend() {
  cd "$ROOT/frontend" && bun install --frozen-lockfile >/dev/null &&
    step lint bun run lint &&
    step typecheck bun run typecheck &&
    step knip bun run knip &&
    step legacy bun run check:legacy
}

# --- tests ------------------------------------------------------------------
tests_backend() {
  cd "$ROOT/backend" || return 1
  local cpu="$LOG/go-cpu.jsonl" rcpu="$LOG/go-race-cpu.jsonl"
  if [ "$MODE" = fast ]; then
    # Cache on: only packages whose code or dependencies changed run.
    step go-test go test -p 4 ./...
    return
  fi
  # The step logs are the -json streams the budget check reads.
  step go-test env GO_TEST_CPU_LOG="$cpu" go test -p 4 -count=1 -json -exec "$CPU_EXEC" ./... || {
    grep -E '"Action":"fail"' "$LOG/go-test.log" | grep -oE '"Package":"[^"]+","Test":"[^"]+"' | sort -u | head -20
    return 1
  }
  step go-race env GO_TEST_CPU_LOG="$rcpu" go test -p 8 -count=1 -timeout 5m -json -exec "$CPU_EXEC" "${RACE_FLAGS[@]}" ./... || {
    grep -E '"Action":"fail"|WARNING: DATA RACE' "$LOG/go-race.log" | grep -oE '"Package":"[^"]+","Test":"[^"]+"|DATA RACE' | sort -u | head -20
    return 1
  }
  step go-budget python3 "$BUDGET" go "$LOG/go-test.log" "$cpu" &&
    step race-budget python3 "$BUDGET" race "$LOG/go-race.log" "$rcpu"
}
tests_frontend() {
  cd "$ROOT/frontend" || return 1
  if [ "$MODE" = fast ]; then
    local changed
    changed="$(git -C "$ROOT" diff --name-only HEAD -- frontend/src; git -C "$ROOT" diff --name-only origin/main...HEAD -- frontend/src)"
    changed="$(printf '%s\n' "$changed" | sort -u | sed 's#^frontend/##' | grep -E '\.(ts|tsx)$' || true)"
    if [ -z "$changed" ]; then echo "SKIP vitest (no frontend source changed)"; return 0; fi
    # shellcheck disable=SC2086
    step vitest bunx vitest related --run $changed
    return
  fi
  step vitest bunx vitest run --maxWorkers=8 --reporter=default --reporter=json --outputFile.json="$LOG/vitest.json" &&
    step vitest-budget python3 "$BUDGET" vitest "$LOG/vitest.json" &&
    step build bun run build
}
tests_e2e() {
  if [ "$MODE" = fast ] && [ ${#SPECS[@]} -eq 0 ]; then
    echo "SKIP e2e (fast gate: name the specs you touched)"
    return 0
  fi
  cd "$ROOT/frontend" || return 1
  if [ ${#SPECS[@]} -gt 0 ]; then
    step e2e bun run e2e -- "${SPECS[@]}"
    return
  fi
  step e2e env PLAYWRIGHT_JSON_OUTPUT_NAME="$LOG/e2e.json" bun run e2e -- --reporter=list,json &&
    step e2e-budget python3 "$BUDGET" e2e "$LOG/e2e.json"
}

timeouts() { step timeouts python3 "$BUDGET" timeouts && step budget-diff python3 "$BUDGET" diff; }

run_parts() { # run the given functions in parallel, fail if any fails
  local pids=() names=() status=0 i
  for f in "$@"; do "$f" & pids+=($!); names+=("$f"); done
  for i in "${!pids[@]}"; do wait "${pids[$i]}" || status=1; done
  return $status
}

start=$SECONDS
echo "gate $MODE, logs in $LOG"
status=0
parts=()
[ -z "${GATE_SKIP_BACKEND:-}" ] && parts+=(checks_backend)
[ -z "${GATE_SKIP_FRONTEND:-}" ] && parts+=(checks_frontend)
parts+=(timeouts)
run_parts "${parts[@]}" || status=1

# Backend tests, frontend tests and e2e in parallel: 126 s together against
# 180 s one after the other (measured on the 28-core dev box).
if [ $status -eq 0 ]; then
  parts=()
  [ -z "${GATE_SKIP_BACKEND:-}" ] && parts+=(tests_backend)
  [ -z "${GATE_SKIP_FRONTEND:-}" ] && parts+=(tests_frontend)
  [ -z "${GATE_SKIP_E2E:-}" ] && parts+=(tests_e2e)
  [ ${#parts[@]} -gt 0 ] && { run_parts "${parts[@]}" || status=1; }
fi

echo "--- gate $MODE: $([ $status -eq 0 ] && echo PASS || echo FAIL) in $((SECONDS - start))s"
grep -hE '^(--- FAIL|FAIL\s|panic:)|BUDGET FAIL|[0-9]+ (passed|failed|flaky)|^\s*(Test Files|Tests)\s' "$LOG"/*.log 2>/dev/null | tail -30
if [ $status -eq 0 ] && [ -n "$KEY" ] && [ -z "${GATE_SKIP_BACKEND:-}${GATE_SKIP_FRONTEND:-}${GATE_SKIP_E2E:-}" ]; then
  date -Is >"$PASS_DIR/$KEY"
fi
mkdir -p "$HOME/.local/share/plantry-dev"
printf '%s,%s,%s,%s,%s\n' "$(date -Is)" "$MODE" "$(git -C "$ROOT" rev-parse --short HEAD)" "$((SECONDS - start))" "$status" >>"$HOME/.local/share/plantry-dev/gate-times.csv"
exit $status
