#!/usr/bin/env bash
# Self-test of the secret-prevention layers: every bypass the adversarial
# review found, reproduced in a throwaway repo with FAKE keys, must be blocked.
# Runs in scripts/gate.sh. Never touches this repo's history or index, never
# prints a key. SELFTEST_SRC=<dir> tests another copy of the guard files (used to
# prove the tests fail against the pre-fix scripts).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="${SELFTEST_SRC:-$ROOT}"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
# gitleaks binary at the pinned version (a mise shim does not work in a temp dir)
want="$(sed -n 's/^gitleaks = "\([^"]*\)".*/\1/p' "$SRC/mise.toml")"
GL=""
for c in "$(cd "$ROOT" && mise which gitleaks 2>/dev/null)" "$(command -v gitleaks 2>/dev/null)"; do
  [ -n "$c" ] && [ "$("$c" version 2>/dev/null)" = "$want" ] && { GL="$c"; break; }
done
[ -n "$GL" ] || { echo "selftest: gitleaks $want not available (mise install)" >&2; exit 1; }
export PATH="$(dirname "$GL"):$PATH"

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null HOME="$T/home"
mkdir -p "$HOME"
GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example.invalid GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example.invalid
export GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL GIT_COMMITTER_NAME GIT_COMMITTER_EMAIL

start=$SECONDS
pass=0
failed=0
rows=()

# fake credentials: random, never real
rnd() { head -c 200 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "$1"; }
K_ANT="sk-ant-api03-$(rnd 40)"
K_GH="ghp_$(rnd 36)"
K_PROJ="sk-proj-$(rnd 44)"
K_ENV="PLANTRY_SECRET_KEY=$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
KEYS=("$K_ANT" "$K_GH" "$K_PROJ" "$K_ENV")

ok() { pass=$((pass + 1)); rows+=("ok    $1"); }
no() { failed=$((failed + 1)); rows+=("FAIL  $1  ($2)"); }
# expect_blocked LABEL CMD...: the command must exit non-zero
expect_blocked() { local l=$1; shift; if "$@" >/dev/null 2>&1; then no "$l" "not blocked"; else ok "$l"; fi; }
expect_pass() { local l=$1; shift; if "$@" >/dev/null 2>&1; then ok "$l"; else no "$l" "wrongly blocked"; fi; }

# new_repo DIR: a repo with the guard files copied in, one clean commit on main
new_repo() {
  local d=$1
  mkdir -p "$d/scripts"
  git init -q -b main "$d"
  for f in secret-guard.sh secret-ci.sh check-db-secrets.py secret-patterns.txt binary-allowlist.txt secret-guard-files.txt; do
    [ -f "$SRC/scripts/$f" ] && cp "$SRC/scripts/$f" "$d/scripts/$f"
  done
  cp "$SRC/.gitleaks.toml" "$SRC/.gitleaksignore" "$SRC/mise.toml" "$d/"
  mkdir -p "$d/.github/workflows" "$d/.claude/hooks"
  cp "$SRC/prek.toml" "$d/"
  echo base > "$d/f.txt"
  git -C "$d" add -A
  git -C "$d" commit -q -m base
}
guard() { ( cd "$1" && shift && bash scripts/secret-guard.sh "$@" ); }
mkdb() { # mkdb FILE [encrypted] : a SQLite DB with an app_settings row, key-like value
  python3 -I - "$1" "${2:-key}" "$K_ANT" <<'PY'
import sqlite3, sys
path, mode, key = sys.argv[1:4]
c = sqlite3.connect(path)
c.execute("CREATE TABLE app_settings(key TEXT PRIMARY KEY, value TEXT, encrypted INTEGER)")
if mode == "key":
    c.execute("INSERT INTO app_settings VALUES('k', ?, 0)", (key,))
elif mode == "enc":
    c.execute("INSERT INTO app_settings VALUES('k', 'abc', 1)")
else:
    c.execute("INSERT INTO app_settings VALUES('theme', 'dark', 0)")
c.commit(); c.execute("VACUUM"); c.close()
PY
}

# leak_files REPO: files gitleaks flags in the staged diff, with the guard's own
# config flags (one gitleaks run covers many cases; a run costs ~0.5 s)
leak_files() {
  rm -f "$T/rep.json"
  "$GL" git --staged --config "$1/.gitleaks.toml" --gitleaks-ignore-path "$1/.gitleaksignore" --ignore-gitleaks-allow \
    --redact --no-banner --log-level error -r "$T/rep.json" --report-format json "$1" >/dev/null 2>&1
  [ -f "$T/rep.json" ] && python3 -I -c 'import json,sys; [print(f["File"]) for f in json.load(open(sys.argv[1]))]' "$T/rep.json"
}
in_list() { grep -qxF -- "$1" <<<"$2"; }

# ── 1. evil merge: gitleaks without -m shows no diff for a merge commit ───────
R="$T/merge"; new_repo "$R"
(
  cd "$R" || exit 1
  git checkout -q -b ma; echo a > f.txt; git commit -qam a
  git checkout -q main; git checkout -q -b mb; echo b > f.txt; git commit -qam b
  git merge ma >/dev/null 2>&1
  printf 'resolved %s\n' "$K_ANT" > f.txt
  git add f.txt; git commit -q --no-verify -m evil
)
expect_blocked "1 evil merge carrying a key is found by a range scan" guard "$R" range "$(git -C "$R" rev-parse main)" mb

# ── 2. multi-ref push: only the first ref used to be scanned ──────────────────
R="$T/push"; new_repo "$R"
git init -q --bare -b main "$T/push-origin.git"
git -C "$R" remote add origin "$T/push-origin.git"
git -C "$R" push -q origin main 2>/dev/null
(
  cd "$R" || exit 1
  git checkout -q -b p3; printf 'token=%s\n' "$K_ANT" > l3.txt; git add l3.txt; git commit -q --no-verify -m leak
  git checkout -q main; git checkout -q -b clean2; echo ok > c2.txt; git add c2.txt; git commit -q -m clean
)
P3="$(git -C "$R" rev-parse p3)"; C2="$(git -C "$R" rev-parse clean2)"
Z=0000000000000000000000000000000000000000
push_clean_first() { printf 'refs/heads/clean2 %s refs/heads/clean2 %s\nrefs/heads/p3 %s refs/heads/p3 %s\n' "$C2" "$Z" "$P3" "$Z" | guard "$R" push; }
push_leak_first() { printf 'refs/heads/p3 %s refs/heads/p3 %s\nrefs/heads/clean2 %s refs/heads/clean2 %s\n' "$P3" "$Z" "$C2" "$Z" | guard "$R" push; }
expect_blocked "2 push scan: clean ref listed first, leaking ref second" push_clean_first
expect_blocked "2 push scan: leaking ref first" push_leak_first
expect_blocked "2 push scan without a ref list on stdin covers every local branch" guard "$R" push </dev/null
cat > "$R/.git/hooks/pre-push" <<'HOOK'
#!/bin/sh
exec bash scripts/secret-guard.sh push
HOOK
chmod +x "$R/.git/hooks/pre-push"
git -C "$R" push origin clean2 p3 >/dev/null 2>&1
if git --git-dir="$T/push-origin.git" rev-parse -q --verify refs/heads/p3 >/dev/null; then no "2 real git push origin clean2 p3 keeps p3 off the remote" "p3 reached the remote"; else ok "2 real git push origin clean2 p3 keeps p3 off the remote"; fi

# ── 3. gitleaks default path allowlist: keys in .svg .pdf go.sum ... ──────────
R="$T/paths"; new_repo "$R"
PFILES=(x.svg x.pdf x.doc x.bin x.jpg go.sum package-lock.json node_modules/x/i.js vendor/a.txt)
for f in "${PFILES[@]}"; do
  for i in 0 1 2 3; do
    n="$f.k$i"; [ "$f" = go.sum ] && n="k$i/go.sum"; [ "$f" = package-lock.json ] && n="k$i/package-lock.json"
    ( cd "$R" && mkdir -p "$(dirname "$n")" && printf 'token=%s\n' "${KEYS[$i]}" > "$n" && git add -f "$n" )
  done
done
# the nit: a key hidden in scripts/secret-patterns.txt
( cd "$R" && printf 'raw\t%s\n' "$K_ANT" >> scripts/secret-patterns.txt && git add scripts/secret-patterns.txt )
expect_blocked "3 guard staged blocks the commit" guard "$R" staged
FOUND="$(leak_files "$R")"
for f in "${PFILES[@]}"; do
  for i in 0 1 2 3; do
    n="$f.k$i"; [ "$f" = go.sum ] && n="k$i/go.sum"; [ "$f" = package-lock.json ] && n="k$i/package-lock.json"
    if in_list "$n" "$FOUND"; then ok "3 key kind $i in $n is found"; else no "3 key kind $i in $n is found" "gitleaks skipped it"; fi
  done
done
if in_list scripts/secret-patterns.txt "$FOUND"; then ok "3 key placed in scripts/secret-patterns.txt is found"; else no "3 key placed in scripts/secret-patterns.txt is found" "skipped"; fi
( cd "$R" && git reset -q --hard )

# ── 4. binary / compressed content ────────────────────────────────────────────
R="$T/bin"; new_repo "$R"
( cd "$R" && printf '\x00\x01%s\x00' "$K_ANT" > blob.bin
  printf '%s' "$K_ANT" | gzip > k.gz
  python3 -I -c "import zipfile,sys; z=zipfile.ZipFile('k.zip','w'); z.writestr('a.txt', sys.argv[1]); z.close()" "$K_ANT"
  printf '\x89PNG\r\n\x1a\n\x00\x00%s\x00' "$K_ANT" > k.png
  printf '%s' "$K_ANT" | iconv -t UTF-16LE > k16.txt
  printf '\x00\x01\x02random-unknown-bytes\x00' > unknown.dat )
for f in blob.bin k.gz k.zip k.png k16.txt unknown.dat; do
  expect_blocked "4 binary $f is not waved through" guard "$R" files "$f"
done
( cd "$R" && printf '\x89PNG\r\n\x1a\n\x00\x00IHDR\x00' > clean.png )
expect_pass "4 a clean PNG passes" guard "$R" files clean.png

# ── 5. databases under any name ───────────────────────────────────────────────
R="$T/db"; new_repo "$R"
for n in x.dat x.bak x.db3 x.sqlite-wal x.sqlite-shm x.DB x.sdb x.s3db x.SQLITE3 testdata/x.dat; do
  ( cd "$R" && mkdir -p "$(dirname "$n")" && mkdb "$n" key )
  expect_blocked "5 key in a SQLite DB named $n" guard "$R" files "$n"
done
( cd "$R" && mkdb x.dbz key && gzip -c x.dbz > x.db.gz )
expect_blocked "5 gzip of a DB (x.db.gz)" guard "$R" files x.db.gz
( cd "$R" && mkdir -p testdata && mkdb testdata/enc.dat enc && mkdb testdata/clean.dat clean )
expect_blocked "5 encrypted app_settings row in testdata/enc.dat" guard "$R" files testdata/enc.dat
expect_pass "5 clean fixture DB under testdata passes (control)" guard "$R" files testdata/clean.dat
expect_blocked "5 clean DB outside testdata is still rejected" guard "$R" files x.db3
( cd "$R" && mkdb hist.dat key && git add hist.dat && git commit -q -m db )
expect_blocked "5 history scan finds the keyed DB under a odd name" guard "$R" range "$(git -C "$R" rev-parse HEAD~1)" HEAD

# ── 6. env-secret allowlist must match the placeholder shape only ─────────────
R="$T/env"; new_repo "$R"
( cd "$R" && printf 'PLANTRY_SECRET_KEY=change-me-change-me-aaaa\n' > ok1.txt
  printf 'PLANTRY_AI_API_KEY=your-api-key-here\n' > ok2.txt
  printf 'PLANTRY_SECRET_KEY=Ab3dE6gH9jK2m%sN5pQ8sT1vW4\n' fake > bad1.txt
  printf 'PLANTRY_AI_API_KEY=Ab3dE6gH9jK2m%sN5pQ8sT1vW4\n' example > bad2.txt
  printf 'PLANTRY_SECRET_KEY=Ab3dE6gH9jK2m%sN5pQ8sT1vW4\n' xxxx > bad3.txt
  git add -f ok1.txt ok2.txt bad1.txt bad2.txt bad3.txt )
FOUND="$(leak_files "$R")"
for f in ok1.txt ok2.txt; do if in_list "$f" "$FOUND"; then no "6 placeholder in $f is allowed" "flagged"; else ok "6 placeholder in $f is allowed"; fi; done
for f in bad1.txt bad2.txt bad3.txt; do if in_list "$f" "$FOUND"; then ok "6 real-looking value in $f (contains fake/example/xxxx) is found"; else no "6 real-looking value in $f is found" "waved through"; fi; done
( cd "$R" && git reset -q --hard )

# ── 7. prek: the vendored-skills exclude must not skip the secret hooks ───────
if grep -qE '^exclude[[:space:]]*=' "$SRC/prek.toml"; then no "7 prek.toml has no global exclude" "global exclude found"; else ok "7 prek.toml has no global exclude"; fi
if command -v prek >/dev/null 2>&1; then
  R="$T/prek"; new_repo "$R"
  ( cd "$R" && mkdir -p .claude/skills/s && printf 'x\n' > .claude/skills/s/secret.key && git add -f .claude/skills/s/secret.key )
  expect_blocked "7 prek secret-files hook sees a secret.key under .claude/skills" bash -c "cd '$R' && prek run secret-files --stage pre-commit --files .claude/skills/s/secret.key"
  ( cd "$R" && git reset -q --hard )
fi

# ── 8. skip-ci markers in a commit message ────────────────────────────────────
R="$T/skip"; new_repo "$R"
( cd "$R" && echo y > y.txt && git add y.txt && git commit -q -m "tidy [skip ci]" )
expect_blocked "8 a commit asking GitHub to skip workflows is refused" guard "$R" range "$(git -C "$R" rev-parse HEAD~1)" HEAD

# ── 9. CI runs origin/main's copy: a branch cannot weaken its own check ───────
R="$T/ci"; new_repo "$R"
git init -q --bare -b main "$T/ci-origin.git"
git -C "$R" remote add origin "$T/ci-origin.git"
git -C "$R" push -q origin main 2>/dev/null
git -C "$R" fetch -q origin
git -C "$R" branch -q --set-upstream-to=origin/main main
TRUSTED="$T/trusted"; mkdir -p "$TRUSTED"
git -C "$R" archive origin/main | tar -x -C "$TRUSTED"
run_ci() { ( cd "$R" && git checkout -q "$1" && EVENT_NAME=pull_request PR_LABELS="${2:-}" bash "$TRUSTED/scripts/secret-ci.sh" "$R" ); }
( cd "$R" && git checkout -q -b evil main
  printf '\n[allowlist]\npaths = [%s.*%s]\n' "'''" "'''" >> .gitleaks.toml
  printf 'token=%s\n' "$K_ANT" > leak.txt
  git add -A && git commit -q --no-verify -m "weaken and leak" )
expect_blocked "9 branch that allowlists everything in its own .gitleaks.toml still fails" run_ci evil secret-guard-change
( cd "$R" && git checkout -q main && git branch -q -D evil && git checkout -q -b benign main && echo '# note' >> .gitleaks.toml && git commit -qam "tweak guard config" )
expect_blocked "9 PR changing a guard file without the label fails" run_ci benign
expect_pass "9 the same PR with the secret-guard-change label passes" run_ci benign secret-guard-change
( cd "$R" && git checkout -q main && git checkout -q -b clean main && echo z > z.txt && git add z.txt && git commit -qm "clean change" )
expect_pass "9 a clean PR passes" run_ci clean
( cd "$R" && git checkout -q main && git branch -q -D benign clean && git checkout -q -b noci main && echo q > q.txt && git add q.txt && git commit -qm "x [ci skip]" )
expect_blocked "9 CI refuses a skip-ci commit message" run_ci noci
# workflow file hygiene: no `if:` that keys off skip markers, and the limit is documented
if grep -rniE '^\s*if:.*skip' "$SRC/.github/workflows" >/dev/null 2>&1; then no "9 workflows have no skip-ci conditions" "if: with skip found"; else ok "9 workflows have no skip-ci conditions"; fi
if grep -q 'skip ci' "$SRC/.github/workflows/secrets.yml" && grep -qi 'skip' "$SRC/CLAUDE.md"; then ok "9 skip-ci limit documented in secrets.yml and CLAUDE.md"; else no "9 skip-ci limit documented in secrets.yml and CLAUDE.md" "missing"; fi
if grep -q 'CI runs after the push' "$SRC/CLAUDE.md"; then ok "9 CLAUDE.md says CI is a detective layer"; else no "9 CLAUDE.md says CI is a detective layer" "missing"; fi

# ── 10. the guard-file list covers what the hook protects ─────────────────────
for f in scripts/secret-guard.sh prek.toml .gitleaks.toml .gitleaksignore scripts/secret-patterns.txt .github/workflows/secrets.yml; do
  if grep -qxF "$f" "$SRC/scripts/secret-guard-files.txt" 2>/dev/null; then ok "10 $f is a guard file"; else no "10 $f is a guard file" "not listed"; fi
done

printf '%s\n' "${rows[@]}"
echo "secret-guard selftest: $pass passed, $failed failed in $((SECONDS - start)) s"
[ "$failed" -eq 0 ]
