#!/usr/bin/env bash
# Install the git hooks of "Secrets" in CLAUDE.md, once per clone (hooks live in
# the common git dir, so every worktree of the clone shares them):
#   pre-commit, commit-msg   prek (gitleaks on the staged diff, blocked files, DB
#                            check, commit-message scan)
#   pre-push                 scripts/pre-push-hook.sh, which reads git's ref list
#                            and runs scripts/secret-guard.sh push, then chains
#                            prek's own pre-push hooks (pre-push.prek)
# Safe to run again.
set -euo pipefail
cd "$(dirname "$0")/.."
dir="$(git rev-parse --git-common-dir)/hooks"
marker='secret-guard pre-push wrapper'
# let prek install its own pre-push first, never treat our wrapper as a legacy hook
if [ -f "$dir/pre-push" ] && grep -q "$marker" "$dir/pre-push"; then rm -f "$dir/pre-push"; fi
prek install --hook-type pre-commit --hook-type commit-msg --hook-type pre-push
if [ -f "$dir/pre-push" ] && ! grep -q "$marker" "$dir/pre-push"; then mv -f "$dir/pre-push" "$dir/pre-push.prek"; fi
cp scripts/pre-push-hook.sh "$dir/pre-push"
chmod +x "$dir/pre-push"
echo "hooks installed in $dir (pre-commit, commit-msg: prek; pre-push: secret-guard wrapper)"
