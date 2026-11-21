#!/usr/bin/env bash
# Structural guards against slow-test anti-patterns (see "Test speed contract"
# in CLAUDE.md). Deterministic, so no timing noise: each rule names a pattern
# that once cost minutes of gate time.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/backend" || exit 1
fail=0
bad() { echo "test-patterns: $1"; echo "$2" | sed 's/^/  /'; fail=1; }

# 1. Test databases go through testhelper (DSN/DSNWith/NewTestDB): its DSN
#    turns fsync off. Own DSNs fsynced every commit (planning 50 s -> 8 s).
#    A read-only source database (mode=ro) never writes and may stay.
out="$(grep -rn 'sql.Open("sqlite"' --include='*_test.go' . | grep -v 'testhelper\.DSN' | grep -v 'mode=ro' || true)"
[ -n "$out" ] && bad "open test databases with testhelper.DSN/DSNWith (fsync off), not an own DSN" "$out"

# 2. Eval sweeps in tests run in parallel: one scenario after the other made
#    internal/eval and cmd/plantry-eval the gate's critical path.
out="$(grep -rn 'Config{Chooser:' --include='*_test.go' internal/eval cmd/plantry-eval | grep -E 'Runs: [0-9]' | grep -v 'Parallel:' || true)"
[ -n "$out" ] && bad "eval.Run in tests sets Parallel (runtime.NumCPU())" "$out"

# 3. No -count=1 in scripts and docs outside the gate: it switches off Go's
#    test cache, which is what runs only the affected packages.
out="$(grep -rn -- 'go test[^`]*-count=1' "$ROOT"/scripts/*.sh "$ROOT"/*.md "$ROOT"/backend/*.md "$ROOT"/frontend/*.md 2>/dev/null | grep -v 'scripts/gate.sh' | grep -v 'check-test-patterns.sh' | grep -v -i -E 'flake|never' || true)"
[ -n "$out" ] && bad "do not recommend go test -count=1 (it disables the test cache); only for chasing a flake" "$out"

# 4. Every testhelper.Slow test is listed in test-budget.json with a reason.
python3 "$ROOT/scripts/test-budget.py" slowlist || fail=1

# 5. Playwright: projects that run one test at a time stay on the allowlist.
#    A serial project adds its whole duration to the e2e critical path.
serial="$(grep -c 'workers: 1' "$ROOT/frontend/playwright.config.ts" || true)"
max="$(python3 -c "import json;print(json.load(open('$ROOT/test-budget.json'))['e2e_serial_projects_max'])")"
if [ "$serial" -gt "$max" ]; then
  bad "frontend/playwright.config.ts has $serial serial projects (workers: 1), allowed $max" "give the specs their own server or data instead of serializing them"
fi

exit $fail
