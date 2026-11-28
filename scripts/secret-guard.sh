#!/usr/bin/env bash
# Secret prevention checks shared by the prek hooks, scripts/gate.sh and CI.
#
#   secret-guard.sh staged            gitleaks over the staged diff
#   secret-guard.sh files PATH...     name + content checks (scripts/check-db-secrets.py)
#   secret-guard.sh range FROM TO     history scan of FROM..TO
#   secret-guard.sh push              history scan for a pre-push hook: every
#                                     ref git is pushing (read from stdin),
#                                     else every local branch and tag, minus
#                                     what the remotes already have
#   secret-guard.sh ci                CI: gitleaks over every ref, the file
#                                     checks on every tracked file and on the
#                                     commits not in $CI_BASE (default origin/main)
#   secret-guard.sh hooks             fail unless pre-commit and pre-push are prek's
#
# A history scan is gitleaks with -m (merge commits are diffed against each
# parent; without it an evil merge is invisible) plus the name/binary/DB checks
# on every file added or changed, merges included. gitleaks reads THIS script's
# .gitleaks.toml and .gitleaksignore, never the scanned repo's, and ignores
# `gitleaks:allow` comments. SECRET_GUARD_REPO names the repo to scan when the
# script is a trusted copy outside it (CI runs origin/main's copy).
#
# Never prints a secret value (gitleaks runs with --redact). Never bypass; if a
# check fires, stop, rotate the credential, and see "Secrets" in CLAUDE.md.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO="${SECRET_GUARD_REPO:-$ROOT}"
cd "$REPO" || exit 1
ZERO=0000000000000000000000000000000000000000

fail=0
bad() { echo "BLOCKED: $*" >&2; fail=1; }

# gitleaks_bin prints a gitleaks binary whose version equals the pin in
# mise.toml. A mise shim that has no version for this directory does not count.
gitleaks_bin() {
  local want c
  want="$(sed -n 's/^gitleaks = "\([^"]*\)".*/\1/p' "$ROOT/mise.toml")"
  for c in "$(command -v gitleaks 2>/dev/null)" "$(mise which gitleaks 2>/dev/null)"; do
    [ -n "$c" ] || continue
    if [ "$("$c" version 2>/dev/null)" = "$want" ]; then echo "$c"; return 0; fi
  done
  echo "gitleaks $want not available: run 'mise install' (pinned in mise.toml)" >&2
  return 1
}

GL_ARGS=(--config "$ROOT/.gitleaks.toml" --gitleaks-ignore-path "$ROOT/.gitleaksignore"
  --ignore-gitleaks-allow --redact --no-banner --log-level warn)

check_files() { python3 -I "$ROOT/scripts/check-db-secrets.py" "$@" || fail=1; }

cmd_staged() {
  local g
  g="$(gitleaks_bin)" || { fail=1; return; }
  "$g" git --staged "${GL_ARGS[@]}" "$REPO" || fail=1
}

# GitHub skips every push/pull_request workflow when a head commit message
# carries one of these, so a commit that does is a way to skip the CI scan.
SKIP_CI_RE='\[(skip ci|ci skip|no ci|skip actions|actions skip)\]|^skip-checks: *true'

check_skip_ci() {
  if git log --format=%B "$@" | grep -qiE "$SKIP_CI_RE"; then
    bad "a commit message in the scanned range asks GitHub to skip workflows (skip-ci marker); CI would not scan it"
  fi
}

# scan_history REVARG...: the REVARGs are `git rev-list` arguments.
scan_history() {
  local g
  check_skip_ci "$@"
  g="$(gitleaks_bin)" || { fail=1; return; }
  "$g" git "${GL_ARGS[@]}" --log-opts="-m $*" "$REPO" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --history "$@" || fail=1
}

cmd_push() {
  local tips=() lname lsha rname rsha
  if [ ! -t 0 ]; then
    while read -r lname lsha rname rsha; do
      [ -n "${lsha:-}" ] && [ "$lsha" != "$ZERO" ] && tips+=("$lsha")
    done
  fi
  if [ ${#tips[@]} -eq 0 ]; then
    # prek does not forward git's ref list: scan every local branch and tag
    scan_history HEAD --branches --tags --not --remotes
  else
    scan_history "${tips[@]}" --not --remotes
  fi
}

cmd_ci() {
  local g
  g="$(gitleaks_bin)" || { fail=1; return; }
  check_skip_ci HEAD --not "${CI_BASE:-origin/main}"
  "$g" git "${GL_ARGS[@]}" --log-opts="-m --all" "$REPO" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --tracked || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --history HEAD --not "${CI_BASE:-origin/main}" || fail=1
}

cmd_hooks() {
  local dir h
  dir="$(git rev-parse --git-common-dir)/hooks"
  [ -z "$(git config --get core.hooksPath)" ] || bad "core.hooksPath is set; hooks must live in $dir"
  for h in pre-commit pre-push; do
    if [ ! -x "$dir/$h" ] || ! grep -q 'prek' "$dir/$h"; then
      bad "$dir/$h is missing or not prek's: run 'prek install --hook-type pre-commit --hook-type pre-push'"
    fi
  done
}

mode=${1:-}
shift || true
case "$mode" in
  staged) cmd_staged ;;
  files) check_files "$@" ;;
  range) scan_history "$1..$2" ;;
  push) cmd_push ;;
  ci) cmd_ci ;;
  hooks) cmd_hooks ;;
  *) echo "usage: $0 staged|files PATH...|range FROM TO|push|ci|hooks" >&2; exit 2 ;;
esac
exit $fail
