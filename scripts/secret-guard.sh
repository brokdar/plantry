#!/usr/bin/env bash
# Secret prevention checks shared by the prek hooks, scripts/gate.sh and CI.
#
#   secret-guard.sh staged            gitleaks over the staged diff + file checks on
#                                     every staged blob (the raw patterns run on all)
#   secret-guard.sh msg FILE          raw patterns over a commit message file
#   secret-guard.sh files PATH...     name + content checks (scripts/check-db-secrets.py)
#   secret-guard.sh range FROM TO     history scan of FROM..TO
#   secret-guard.sh push [REMOTE URL] pre-push: git's ref list on stdin (installed
#                                     by scripts/pre-push-hook.sh; prek does not
#                                     forward stdin). Scans the new commits of every
#                                     pushed sha (minus what `git ls-remote REMOTE`
#                                     says the remote has, never local remote-tracking
#                                     refs), ref names, tag messages; refuses
#                                     non-commit objects
#   secret-guard.sh ci                CI: gitleaks over every ref, the file
#                                     checks on every tracked file and on the
#                                     commits not in $CI_BASE (default origin/main)
#   secret-guard.sh hooks             fail unless pre-commit, commit-msg are prek's and
#                                     pre-push is the guard's wrapper
#
# A history scan is gitleaks with -m (merge commits are diffed against each
# parent; without it an evil merge is invisible) plus the name/binary/DB checks
# on every file added or changed, merges included, plus the raw credential patterns
# over commit messages and author/committer fields. gitleaks reads THIS script's
# .gitleaks.toml and .gitleaksignore, never the scanned repo's, and ignores
# `gitleaks:allow` comments. Attributes cannot hide content: the raw patterns run
# over every blob whatever .gitattributes says, gitleaks runs with `git log --text`,
# GIT_ATTR_SOURCE is the empty tree, and a non-empty .git/info/attributes is refused.
# The gitleaks binary must match scripts/gitleaks-binary.sha256. SECRET_GUARD_REPO names the repo to scan when the
# script is a trusted copy outside it (CI runs origin/main's copy).
#
# Never prints a secret value (gitleaks runs with --redact). Never bypass; if a
# check fires, stop, rotate the credential, and see "Secrets" in CLAUDE.md.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
REPO="${SECRET_GUARD_REPO:-$ROOT}"
cd "$REPO" || exit 1
ZERO=0000000000000000000000000000000000000000
TIMEOUT="$(command -v timeout 2>/dev/null || true)"

fail=0
bad() { echo "BLOCKED: $*" >&2; fail=1; }

# Attributes cannot hide content from the scans (see the header): ignore every
# .gitattributes (empty tree), the system and global attributes files.
export GIT_ATTR_NOSYSTEM=1 GIT_ATTR_SOURCE=4b825dc642cb6eb9a060e54bf8d69288fbee4904
export GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.attributesFile GIT_CONFIG_VALUE_0=/dev/null

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# gitleaks_bin prints a gitleaks binary whose SHA-256 is the pin for this platform
# in scripts/gitleaks-binary.sha256. The `version` output is never trusted: a fake
# binary on PATH can print anything.
gitleaks_bin() {
  local plat want c
  plat="$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)"
  want="$(awk -v p="$plat" '$1 == p { print $2 }' "$ROOT/scripts/gitleaks-binary.sha256")"
  if [ -z "$want" ]; then
    echo "no pinned gitleaks SHA-256 for $plat in scripts/gitleaks-binary.sha256" >&2
    return 1
  fi
  for c in "$(mise which gitleaks 2>/dev/null)" "$(command -v gitleaks 2>/dev/null)"; do
    [ -n "$c" ] && [ -f "$c" ] || continue
    if [ "$(sha256_of "$c")" = "$want" ]; then echo "$c"; return 0; fi
  done
  echo "no gitleaks binary with the pinned SHA-256 (mise.toml pin): run 'mise install'" >&2
  return 1
}

GL_ARGS=(--config "$ROOT/.gitleaks.toml" --gitleaks-ignore-path "$ROOT/.gitleaksignore"
  --ignore-gitleaks-allow --redact --no-banner --log-level warn)

check_files() { python3 -I "$ROOT/scripts/check-db-secrets.py" "$@" || fail=1; }

# check_text LABEL: the raw credential patterns over stdin (never on the right of a
# pipe: that runs in a subshell and loses `fail`; use < file, <<< or < <(cmd))
check_text() { python3 -I "$ROOT/scripts/check-db-secrets.py" --text "$1" || fail=1; }

# a .git/info/attributes file with content could hide diffs from gitleaks
check_info_attributes() {
  local f
  f="$(git rev-parse --git-path info/attributes)"
  if [ -f "$f" ] && grep -qvE '^[[:space:]]*(#|$)' "$f"; then
    bad "$f sets attributes (-diff/binary/...) that can hide content from the scans; empty it"
  fi
}

cmd_staged() {
  local g
  g="$(gitleaks_bin)" || { fail=1; return; }
  check_info_attributes
  "$g" git --staged "${GL_ARGS[@]}" "$REPO" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --staged || fail=1
}

cmd_msg() { check_text "commit message" < "${1:?usage: secret-guard.sh msg FILE}"; }

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
  check_info_attributes
  "$g" git "${GL_ARGS[@]}" --log-opts="-m --text $*" "$REPO" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --history "$@" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --messages "$@" || fail=1
}

# cmd_push REMOTE URL: git's pre-push stdin is `<local ref> <local sha> <remote ref>
# <remote sha>` per ref. Nothing local is trusted to say what the remote has:
# the exclusions come from `git ls-remote REMOTE`, so a forged
# refs/remotes/... cannot switch the scan off.
cmd_push() {
  local remote="${1:-}" tips=() excl=() lname lsha rname rsha t obj depth names="" tagtext="" lsr sha ty
  while read -r lname lsha rname rsha; do
    [ -n "${lsha:-}" ] || continue
    names+="$rname"$'\n'
    [ "$lsha" = "$ZERO" ] && continue
    if ! t="$(git cat-file -t "$lsha" 2>/dev/null)"; then
      bad "pushed object $lsha ($rname) is not in this repository"
      continue
    fi
    obj="$lsha"
    depth=0
    while [ "$t" = tag ] && [ $depth -lt 20 ]; do
      tagtext+="$(git cat-file -p "$obj")"$'\n'
      obj="$(git cat-file -p "$obj" | sed -n 's/^object //p;/^$/q' | head -n1)"
      t="$(git cat-file -t "$obj" 2>/dev/null)" || t=missing
      depth=$((depth + 1))
    done
    if [ "$t" = commit ]; then tips+=("$obj"); else bad "$rname points at a $t object; only commits (or tags of commits) may be pushed"; fi
  done
  [ -z "$names" ] || check_text "pushed ref name" <<<"$names"
  [ -z "$tagtext" ] || check_text "annotated tag" <<<"$tagtext"
  [ ${#tips[@]} -gt 0 ] || return 0
  if [ -n "$remote" ] && lsr="$(${TIMEOUT:+$TIMEOUT 60} git ls-remote "$remote" 2>/dev/null)"; then
    while read -r sha ty; do
      [ "$ty" = commit ] || [ "$ty" = tag ] && excl+=("$sha")
    done < <(cut -f1 <<<"$lsr" | git cat-file --batch-check='%(objectname) %(objecttype)' 2>/dev/null)
  else
    echo "secret-guard: cannot list the remote; scanning every commit reachable from the pushed refs" >&2
  fi
  scan_history "${tips[@]}" --not "${excl[@]}"
}

cmd_ci() {
  local g
  g="$(gitleaks_bin)" || { fail=1; return; }
  check_skip_ci HEAD --not "${CI_BASE:-origin/main}"
  check_info_attributes
  "$g" git "${GL_ARGS[@]}" --log-opts="-m --text --all" "$REPO" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --tracked || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --history HEAD --not "${CI_BASE:-origin/main}" || fail=1
  python3 -I "$ROOT/scripts/check-db-secrets.py" --messages --all || fail=1
  # ref names, annotated tag messages, refs that point at blobs or trees
  check_text "ref name" < <(git for-each-ref --format='%(refname)')
  local tagtext="" ref ty peeled
  while read -r ref ty peeled; do
    [ -n "$ref" ] || continue
    if [ "$ty" = tag ]; then tagtext+="$(git cat-file -p "$ref")"$'\n'; fi
    [ "$ty" = tag ] && ty="$peeled"
    [ "$ty" = commit ] || bad "$ref points at a ${ty:-missing} object"
  done < <(git for-each-ref --format='%(objectname) %(objecttype) %(*objecttype)')
  [ -z "$tagtext" ] || check_text "annotated tag" <<<"$tagtext"
}

cmd_hooks() {
  local dir h
  dir="$(git rev-parse --git-common-dir)/hooks"
  [ -z "$(git config --get core.hooksPath)" ] || bad "core.hooksPath is set; hooks must live in $dir"
  for h in pre-commit commit-msg; do
    if [ ! -x "$dir/$h" ] || ! grep -q 'prek' "$dir/$h"; then
      bad "$dir/$h is missing or not prek's: run scripts/install-hooks.sh"
    fi
  done
  if [ ! -x "$dir/pre-push" ] || ! cmp -s "$ROOT/scripts/pre-push-hook.sh" "$dir/pre-push"; then
    bad "$dir/pre-push is missing or not scripts/pre-push-hook.sh (prek cannot see the pushed refs): run scripts/install-hooks.sh"
  fi
}

mode=${1:-}
shift || true
case "$mode" in
  staged) cmd_staged ;;
  files) check_files "$@" ;;
  range) scan_history "$1..$2" ;;
  push) cmd_push "$@" ;;
  msg) cmd_msg "$@" ;;
  ci) cmd_ci ;;
  hooks) cmd_hooks ;;
  *) echo "usage: $0 staged|msg FILE|files PATH...|range FROM TO|push [REMOTE URL]|ci|hooks" >&2; exit 2 ;;
esac
exit $fail
