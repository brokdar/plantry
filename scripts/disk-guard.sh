#!/usr/bin/env bash
# Disk guard: run first by scripts/gate.sh so a full disk is caught before it
# corrupts the WSL vhdx. Limits (GB) can be overridden by env vars:
#   DISK_MIN_FREE_GB   free space needed on / and on /mnt/c (default 30)
#   GOCACHE_MAX_GB     Go build cache above this is cleaned (default 20)
#   DEV_DATA_MAX_GB    ~/.local/share/plantry-dev + Go cache, fail above (default 60)
set -uo pipefail

MIN_FREE=${DISK_MIN_FREE_GB:-30}
CACHE_MAX=${GOCACHE_MAX_GB:-20}
DEV_MAX=${DEV_DATA_MAX_GB:-60}
DEV_DIR="$HOME/.local/share/plantry-dev"
fail=0

free_gb() { df -Pk "$1" | awk 'NR==2 {printf "%d", $4/1048576}'; }
size_gb() { [ -d "$1" ] && du -sk "$1" 2>/dev/null | awk '{printf "%d", $1/1048576}' || echo 0; }

mounts=(/)
mountpoint -q /mnt/c 2>/dev/null && mounts+=(/mnt/c)
for m in "${mounts[@]}"; do
  f=$(free_gb "$m")
  if [ "$f" -lt "$MIN_FREE" ]; then
    echo "disk-guard: FAIL only ${f} GB free on $m (need $MIN_FREE). Free space: remove merged worktrees (git worktree remove), pack paid-run data (see CLAUDE.md 'Disk limits'), 'go clean -cache'." >&2
    fail=1
  fi
done

cache="$(go env GOCACHE 2>/dev/null)"
if [ -n "$cache" ]; then
  c=$(size_gb "$cache")
  if [ "$c" -gt "$CACHE_MAX" ]; then
    echo "disk-guard: Go cache $cache is ${c} GB (> $CACHE_MAX); running go clean -cache"
    go clean -cache
    echo "disk-guard: Go cache now $(size_gb "$cache") GB"
  fi
  c=$(size_gb "$cache")
else
  c=0
fi

d=$(size_gb "$DEV_DIR")
if [ $((d + c)) -gt "$DEV_MAX" ]; then
  echo "disk-guard: FAIL $DEV_DIR (${d} GB) + Go cache (${c} GB) exceed $DEV_MAX GB. Prune: pack scored paid-run data with 'python3 -I m3/dbpack.py pack <series>' in $DEV_DIR/loop, delete old series and logs." >&2
  fail=1
fi

[ "$fail" -eq 0 ] && echo "disk-guard: ok (go cache ${c} GB, dev data ${d} GB)"
exit "$fail"
