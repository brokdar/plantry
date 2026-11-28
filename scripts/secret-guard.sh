#!/usr/bin/env bash
# Secret prevention checks shared by the prek hooks, scripts/gate.sh and CI.
#
#   secret-guard.sh staged            gitleaks over the staged diff
#   secret-guard.sh files PATH...     blocked-file names + SQLite credential check
#   secret-guard.sh range FROM TO     gitleaks over FROM..TO, plus blocked files
#                                     and DB checks on every file added or
#                                     changed in that range
#   secret-guard.sh push              range for a pre-push hook: prek exports
#                                     PRE_COMMIT_FROM_REF/PRE_COMMIT_TO_REF;
#                                     new branches fall back to origin/main
#   secret-guard.sh hooks             fail unless pre-commit and pre-push are prek's
#
# Never prints a secret value (gitleaks runs with --redact). Never bypass; if a
# check fires, stop, rotate the credential, and see "Secrets" in CLAUDE.md.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT" || exit 1
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

# is_testdata PATH: SQLite fixtures are allowed only under a testdata dir.
is_testdata() { case "$1" in testdata/*|*/testdata/*) return 0 ;; esac; return 1; }

# check_name PATH: block credential-carrying file names (and DBs outside testdata).
check_name() {
  local p=$1 b
  b="$(basename "$p")"
  case "$b" in
    *.example) ;;
    .env|.env.*|secret.key|*.pem|*.p12|*.pfx|*.key|id_rsa*|id_ed25519*)
      bad "$p is a credential file name; never commit it" ;;
  esac
  case "$p" in .claude/settings.local.json) bad "$p is local-only settings" ;; esac
  case "$b" in
    *.db|*.sqlite|*.sqlite[0-9]*)
      is_testdata "$p" || bad "$p is a database outside a testdata directory" ;;
  esac
}

is_db() { case "$1" in *.db|*.sqlite|*.sqlite[0-9]*) return 0 ;; esac; return 1; }

cmd_files() {
  local dbs=() p
  for p in "$@"; do
    check_name "$p"
    if is_db "$p" && [ -f "$p" ]; then dbs+=("$p"); fi
  done
  if [ ${#dbs[@]} -gt 0 ]; then
    python3 -I "$ROOT/scripts/check-db-secrets.py" "${dbs[@]}" || fail=1
  fi
}

cmd_staged() {
  local g
  g="$(gitleaks_bin)" || { fail=1; return; }
  "$g" git --staged --redact --no-banner --log-level warn "$ROOT" || fail=1
}

cmd_range() {
  local from=$1 to=$2 g tmp sha p
  g="$(gitleaks_bin)" || { fail=1; return; }
  "$g" git --redact --no-banner --log-level warn --log-opts="$from..$to" "$ROOT" || fail=1
  tmp="$(mktemp -d)"
  for sha in $(git rev-list "$from..$to"); do
    while IFS= read -r -d '' p; do
      check_name "$p"
      if is_db "$p"; then
        mkdir -p "$tmp/$sha"
        # keep the base name: the check reports the path, never the content
        if git show "$sha:$p" >"$tmp/$sha/$(basename "$p")" 2>/dev/null; then
          python3 -I "$ROOT/scripts/check-db-secrets.py" "$tmp/$sha/$(basename "$p")" 2>&1 | sed "s#$tmp/$sha/#$sha:$(dirname "$p")/#" >&2
          [ "${PIPESTATUS[0]}" -eq 0 ] || fail=1
        fi
      fi
    done < <(git diff-tree --no-commit-id --name-only -r -z --diff-filter=AM -m "$sha")
  done
  rm -rf "$tmp"
}

cmd_push() {
  local from="${PRE_COMMIT_FROM_REF:-}" to="${PRE_COMMIT_TO_REF:-HEAD}"
  if [ -z "$from" ] || [ "$from" = "$ZERO" ]; then
    # new remote branch: everything not yet on origin/main
    from="$(git merge-base origin/main "$to" 2>/dev/null || true)"
    if [ -z "$from" ]; then from="$(git rev-list --max-parents=0 "$to" | tail -1)"; fi
  fi
  cmd_range "$from" "$to"
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
  files) cmd_files "$@" ;;
  range) cmd_range "$1" "$2" ;;
  push) cmd_push ;;
  hooks) cmd_hooks ;;
  *) echo "usage: $0 staged|files PATH...|range FROM TO|push|hooks" >&2; exit 2 ;;
esac
exit $fail
