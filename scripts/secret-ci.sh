#!/usr/bin/env bash
# CI entry for .github/workflows/secrets.yml. Run THIS script from a checkout of
# origin/main (the trusted copy), pointing at the checkout under test:
#
#   trusted/scripts/secret-ci.sh REPO_DIR
#
# so a branch cannot weaken its own check: the scripts, .gitleaks.toml,
# .gitleaksignore and the pattern lists all come from origin/main, and REPO_DIR
# only supplies git data. Env: EVENT_NAME (pull_request[_target] adds the
# guard-file check), PR_LABELS (comma-separated), CI_BASE (default origin/main).
#
# CI is a DETECTIVE layer: it runs after the push. The preventive layers are the
# local prek hooks and GitHub push protection (see "Secrets" in CLAUDE.md).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO="$(cd "${1:?usage: secret-ci.sh REPO_DIR}" && pwd)"
export SECRET_GUARD_REPO="$REPO"
fail=0

bash "$ROOT/scripts/secret-guard.sh" ci || fail=1

case "${EVENT_NAME:-}" in
  pull_request|pull_request_target)
    changed=""
    while IFS= read -r f; do
      case "$f" in ''|'#'*) continue ;; esac
      if git -C "$REPO" diff --name-only "origin/main...HEAD" -- "$f" | grep -q .; then changed="$changed $f"; fi
    done < "$ROOT/scripts/secret-guard-files.txt"
    if [ -n "$changed" ]; then
      case ",${PR_LABELS:-}," in
        *,secret-guard-change,*) echo "guard files changed with the secret-guard-change label:$changed" ;;
        *) echo "BLOCKED: this PR changes secret-prevention files and has no secret-guard-change label (owner applies it):$changed" >&2
           fail=1 ;;
      esac
    fi
    ;;
esac
exit $fail
