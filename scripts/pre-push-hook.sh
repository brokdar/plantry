#!/bin/sh
# secret-guard pre-push wrapper. Installed as .git/hooks/pre-push by
# scripts/install-hooks.sh (a copy of this file; `scripts/secret-guard.sh hooks`
# and scripts/gate.sh fail when the installed copy differs).
#
# Why not a prek hook: prek does not forward git's stdin, so a prek pre-push hook
# never learns WHICH refs and shas are being pushed (a stash, a dangling sha, a
# tag on a blob, refs/custom/x). This wrapper reads that list itself, hands it to
# the guard, then chains prek's other pre-push hooks (pre-push.prek), if any.
tmp="$(mktemp)" || exit 1
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
top="$(git rev-parse --show-toplevel 2>/dev/null)" || top="$(pwd)"
guard="$top/scripts/secret-guard.sh"
if [ ! -f "$guard" ]; then
  echo "BLOCKED: $guard is missing (this branch predates the secret guard: rebase onto main)" >&2
  exit 1
fi
bash "$guard" push "$@" < "$tmp" || exit 1
hook="$(dirname "$0")/pre-push.prek"
if [ -x "$hook" ]; then
  "$hook" "$@" < "$tmp"
  exit $?
fi
exit 0
