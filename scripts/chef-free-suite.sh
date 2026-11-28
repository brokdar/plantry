#!/usr/bin/env bash
# The free Chef evaluation sets (SPEC section 10, "Offline evaluation / Free"):
# fake provider, rules and fixtures, no key and no network. Each set is a
# `go test -run` selection of existing tests; the cache stays on. Prints
# pass/fail per set and exits non-zero when one fails.
#
#   scripts/chef-free-suite.sh
#
# The paid sets (finds, import check, substitutions, help, month cost) are
# `plantry-chef-canary -mode ...`, see backend/CLAUDE.md "Paid canary".
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/backend" || exit 1
export GOWORK=off

fail=0
results=()

# set <name> <package> <-run regex> [<package> <-run regex>]...
set_run() {
  local name="$1"; shift
  local start=$SECONDS ok=1 out
  while [ "$#" -ge 2 ]; do
    out="$(go test -p 4 "$1" -run "$2" 2>&1)"
    # A -run that matches nothing passes silently; that is a failure here.
    if [ "$?" != 0 ] || echo "$out" | grep -q "no tests to run"; then
      ok=0
      echo "$out" | tail -40
    fi
    shift 2
  done
  local took=$((SECONDS - start))
  if [ "$ok" = 1 ]; then
    results+=("$(printf 'PASS  %-34s %3ds' "$name" "$took")")
  else
    results+=("$(printf 'FAIL  %-34s %3ds' "$name" "$took")")
    fail=1
  fi
}

set_run "plate fit (good and flawed plates)" \
  ./internal/chefsets 'TestDevSet|TestFixtureSize|TestRichnessFollowsFatShare|TestValidateRejects|TestFieldClasses' \
  ./internal/domain/platefit 'TestDevSet'
set_run "tip checks" ./internal/domain/tips 'TestCandidates'
results+=("$(printf 'n/a   %-34s' "reserved-cell transitions")  (C5 skipped: not built)")
set_run "robots handling" \
  ./internal/adapters/httpfetch 'TestPolite_Robots' \
  ./internal/domain/finds 'TestPipeline_RobotsRefusedSiteLeftOut'
set_run "German 72-line matching, no key" \
  ./internal/chefsets 'TestMatchingSetsLoad|TestGermanSetIsTheRepoTestPages|TestMatchRates_NoKey|TestBaselineLocalMatchRates' \
  ./internal/domain/food 'TestMatch_OfflineCatalogueWithoutFDCKey'
set_run "taste maths over simulated weeks" ./internal/domain/taste 'TestSimulatedWeeks|TestFourWeeks'
set_run "gates on planted unsafe recipes" \
  ./internal/domain/gates 'TestPlantedRecipes|TestDeterministicAndPointers|TestFindingPointsAtLine' \
  ./internal/domain/importer 'TestCheck_'
set_run "migrations" ./internal/adapters/sqlite 'TestMigration'
set_run "paid sets load and validate" ./internal/chefsets 'TestPaidSets' ./internal/chefcanary 'TestImportCases'

echo
echo "Chef free suite"
printf '%s\n' "${results[@]}"
[ "$fail" = 0 ] && echo "all free sets pass" || echo "SOME FREE SETS FAIL"
exit "$fail"
