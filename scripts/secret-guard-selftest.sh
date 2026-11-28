#!/usr/bin/env bash
# Self-test of the secret-prevention layers: every bypass the adversarial
# reviews found, reproduced in a throwaway repo with FAKE keys, must be blocked,
# and for the specific reason (expect_blocked takes the message it must print:
# a guard that always fails would pass a bare exit-code check).
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
# expect_blocked LABEL REGEX CMD...: the command must exit non-zero AND print REGEX
expect_blocked() {
  local l=$1 re=$2 out rc
  shift 2
  out="$("$@" 2>&1)"; rc=$?
  if [ $rc -eq 0 ]; then no "$l" "not blocked"
  elif grep -qE -- "$re" <<<"$out"; then ok "$l"
  else no "$l" "blocked for another reason (exit $rc): $(head -c 160 <<<"$out" | tr '\n' ' ')"; fi
}
expect_pass() { local l=$1; shift; if "$@" >/dev/null 2>&1; then ok "$l"; else no "$l" "wrongly blocked"; fi; }

# new_repo DIR: a repo with the guard files copied in, one clean commit on main
new_repo() {
  local d=$1
  mkdir -p "$d/scripts"
  git init -q -b main "$d"
  for f in secret-guard.sh secret-ci.sh check-db-secrets.py secret-patterns.txt binary-allowlist.txt secret-guard-files.txt \
    gitleaks-binary.sha256 pre-push-hook.sh install-hooks.sh; do
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
expect_blocked "1 evil merge carrying a key is found by a range scan" "leaks found|matches credential" guard "$R" range "$(git -C "$R" rev-parse main)" mb

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
push_clean_first() { printf 'refs/heads/clean2 %s refs/heads/clean2 %s\nrefs/heads/p3 %s refs/heads/p3 %s\n' "$C2" "$Z" "$P3" "$Z" | guard "$R" push origin; }
push_leak_first() { printf 'refs/heads/p3 %s refs/heads/p3 %s\nrefs/heads/clean2 %s refs/heads/clean2 %s\n' "$P3" "$Z" "$C2" "$Z" | guard "$R" push origin; }
expect_blocked "2 push scan: clean ref listed first, leaking ref second" "leaks found|matches credential" push_clean_first
expect_blocked "2 push scan: leaking ref first" "leaks found|matches credential" push_leak_first
cp "$SRC/scripts/pre-push-hook.sh" "$R/.git/hooks/pre-push"; chmod +x "$R/.git/hooks/pre-push"
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
expect_blocked "3 guard staged blocks the commit" "leaks found|matches credential" guard "$R" staged
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
  expect_blocked "4 binary $f is not waved through" "$f: .*(credential pattern|archive|unknown binary)" guard "$R" files "$f"
done
( cd "$R" && printf '\x89PNG\r\n\x1a\n\x00\x00IHDR\x00' > clean.png )
expect_pass "4 a clean PNG passes" guard "$R" files clean.png

# ── 5. databases under any name ───────────────────────────────────────────────
R="$T/db"; new_repo "$R"
for n in x.dat x.bak x.db3 x.sqlite-wal x.sqlite-shm x.DB x.sdb x.s3db x.SQLITE3 testdata/x.dat; do
  ( cd "$R" && mkdir -p "$(dirname "$n")" && mkdb "$n" key )
  expect_blocked "5 key in a SQLite DB named $n" "$n: .*(credential|database|looks like)" guard "$R" files "$n"
done
( cd "$R" && mkdb x.dbz key && gzip -c x.dbz > x.db.gz )
expect_blocked "5 gzip of a DB (x.db.gz)" "x.db.gz: gzip archive" guard "$R" files x.db.gz
( cd "$R" && mkdir -p testdata && mkdb testdata/enc.dat enc && mkdb testdata/clean.dat clean )
expect_blocked "5 encrypted app_settings row in testdata/enc.dat" "is encrypted" guard "$R" files testdata/enc.dat
expect_pass "5 clean fixture DB under testdata passes (control)" guard "$R" files testdata/clean.dat
expect_blocked "5 clean DB outside testdata is still rejected" "x.db3: database outside a testdata" guard "$R" files x.db3
( cd "$R" && mkdb hist.dat key && git add hist.dat && git commit -q -m db )
expect_blocked "5 history scan finds the keyed DB under a odd name" "hist.dat: .*(credential|looks like)" guard "$R" range "$(git -C "$R" rev-parse HEAD~1)" HEAD

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
  expect_blocked "7 prek secret-files hook sees a secret.key under .claude/skills" "credential file name" bash -c "cd '$R' && prek run secret-files --stage pre-commit --files .claude/skills/s/secret.key"
  ( cd "$R" && git reset -q --hard )
fi

# ── 8. skip-ci markers in a commit message ────────────────────────────────────
R="$T/skip"; new_repo "$R"
( cd "$R" && echo y > y.txt && git add y.txt && git commit -q -m "tidy [skip ci]" )
expect_blocked "8 a commit asking GitHub to skip workflows is refused" "skip-ci marker" guard "$R" range "$(git -C "$R" rev-parse HEAD~1)" HEAD

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
expect_blocked "9 branch that allowlists everything in its own .gitleaks.toml still fails" "leaks found|matches credential" run_ci evil secret-guard-change
( cd "$R" && git checkout -q main && git branch -q -D evil && git checkout -q -b benign main && echo '# note' >> .gitleaks.toml && git commit -qam "tweak guard config" )
expect_blocked "9 PR changing a guard file without the label fails" "no secret-guard-change label" run_ci benign
expect_pass "9 the same PR with the secret-guard-change label passes" run_ci benign secret-guard-change
( cd "$R" && git checkout -q main && git checkout -q -b clean main && echo z > z.txt && git add z.txt && git commit -qm "clean change" )
expect_pass "9 a clean PR passes" run_ci clean
( cd "$R" && git checkout -q main && git branch -q -D benign clean && git checkout -q -b noci main && echo q > q.txt && git add q.txt && git commit -qm "x [ci skip]" )
expect_blocked "9 CI refuses a skip-ci commit message" "skip-ci marker" run_ci noci
# workflow file hygiene: no `if:` that keys off skip markers, and the limit is documented
if grep -rniE '^\s*if:.*skip' "$SRC/.github/workflows" >/dev/null 2>&1; then no "9 workflows have no skip-ci conditions" "if: with skip found"; else ok "9 workflows have no skip-ci conditions"; fi
if grep -q 'skip ci' "$SRC/.github/workflows/secrets.yml" && grep -qi 'skip' "$SRC/CLAUDE.md"; then ok "9 skip-ci limit documented in secrets.yml and CLAUDE.md"; else no "9 skip-ci limit documented in secrets.yml and CLAUDE.md" "missing"; fi
if grep -q 'CI runs after the push' "$SRC/CLAUDE.md"; then ok "9 CLAUDE.md says CI is a detective layer"; else no "9 CLAUDE.md says CI is a detective layer" "missing"; fi

# ── 10. the guard-file list covers what the hook protects ─────────────────────
for f in scripts/secret-guard.sh prek.toml .gitleaks.toml .gitleaksignore scripts/secret-patterns.txt .github/workflows/secrets.yml; do
  if grep -qxF "$f" "$SRC/scripts/secret-guard-files.txt" 2>/dev/null; then ok "10 $f is a guard file"; else no "10 $f is a guard file" "not listed"; fi
done

# ── real-push helpers: a bare remote and the installed pre-push wrapper ───────
# new_pushrepo DIR: repo + bare origin + .git/hooks/pre-push = the wrapper
new_pushrepo() {
  local d=$1
  new_repo "$d"
  git init -q --bare -b main "$d-origin.git"
  git -C "$d" remote add origin "$d-origin.git"
  git -C "$d" push -q origin main 2>/dev/null
  cp "$SRC/scripts/pre-push-hook.sh" "$d/.git/hooks/pre-push"; chmod +x "$d/.git/hooks/pre-push"
}
# expect_push_blocked LABEL REGEX REPO REMOTEREF PUSHARGS...: `git push origin PUSHARGS`
# must fail with REGEX in its output and REMOTEREF (if given) must not exist on the remote
expect_push_blocked() {
  local l=$1 re=$2 d=$3 rref=$4 out rc
  shift 4
  out="$(cd "$d" && git push origin "$@" 2>&1)"; rc=$?
  if [ $rc -eq 0 ]; then no "$l" "push succeeded"; return; fi
  if [ -n "$rref" ] && git --git-dir="$d-origin.git" rev-parse -q --verify "$rref" >/dev/null 2>&1; then no "$l" "ref $rref reached the remote"; return; fi
  if grep -qE -- "$re" <<<"$out"; then ok "$l"; else no "$l" "blocked for another reason: $(head -c 160 <<<"$out" | tr '\n' ' ')"; fi
}

# ── 11. the real installed pre-push hook sees stash, dangling and odd refs ────
R="$T/hook"; new_pushrepo "$R"
LEAKRE="leaks found|matches credential"
( cd "$R" && git checkout -q -b work && printf 'token=%s\n' "$K_ANT" > st.txt && git add st.txt && git stash -q )
expect_push_blocked "11 stash pushed as a branch (stash@{0}:refs/heads/st)" "$LEAKRE" "$R" refs/heads/st 'stash@{0}:refs/heads/st'
expect_push_blocked "11 stash pushed to refs/custom/x" "$LEAKRE" "$R" refs/custom/x 'stash@{0}:refs/custom/x'
( cd "$R" && printf 'token=%s\n' "$K_ANT" > d.txt && git add d.txt && git stash create > "$T/dang.sha"; git reset -q --hard )
expect_push_blocked "11 dangling stash-create commit pushed by sha" "$LEAKRE" "$R" refs/heads/dang "$(cat "$T/dang.sha")":refs/heads/dang
( cd "$R" && printf 'token=%s\n' "$K_ANT" | git hash-object -w --stdin > "$T/blob.sha" && git tag blobtag "$(cat "$T/blob.sha")" && git tag -a -m m blobtag2 "$(cat "$T/blob.sha")" )
expect_push_blocked "11 lightweight tag of a blob" "points at a blob" "$R" refs/tags/blobtag blobtag
expect_push_blocked "11 annotated tag of a blob" "points at a blob" "$R" refs/tags/blobtag2 blobtag2
( cd "$R" && git tag treetag "$(git rev-parse 'HEAD^{tree}')" )
expect_push_blocked "11 tag of a tree" "points at a tree" "$R" refs/tags/treetag treetag
# 2 (second review): a forged remote-tracking ref used to switch the scan off
( cd "$R" && git checkout -q -b leak main && printf 'token=%s\n' "$K_ANT" > l.txt && git add l.txt && git commit -q --no-verify -m leak )
expect_push_blocked "11 leak branch is blocked" "$LEAKRE" "$R" refs/heads/leak leak
( cd "$R" && git update-ref refs/remotes/origin/leak HEAD )
expect_push_blocked "11 forged refs/remotes/origin/leak does not disable the scan" "$LEAKRE" "$R" refs/heads/leak leak
( cd "$R" && git checkout -q -b fine main && echo fine > fine.txt && git add fine.txt && git commit -q -m fine )
if ( cd "$R" && git push origin fine >/dev/null 2>&1 ); then ok "11 control: a clean branch pushes through the same hook"; else no "11 control: a clean branch pushes through the same hook" "wrongly blocked"; fi
# chain: prek's own pre-push hooks run after the guard
printf '#!/bin/sh\nexit 3\n' > "$R/.git/hooks/pre-push.prek"; chmod +x "$R/.git/hooks/pre-push.prek"
( cd "$R" && git checkout -q -b fine2 main && echo f2 > f2.txt && git add f2.txt && git commit -q -m fine2 )
expect_push_blocked "11 the chained pre-push.prek hook still runs (exit 3 stops the push)" "" "$R" refs/heads/fine2 fine2
rm -f "$R/.git/hooks/pre-push.prek"

# ── 12. attributes cannot hide content ────────────────────────────────────────
R="$T/attr"; new_repo "$R"
git init -q --bare -b main "$R-origin.git"
git -C "$R" remote add origin "$R-origin.git"
git -C "$R" push -q origin main 2>/dev/null
( cd "$R" && printf '*.dat -diff\n*.bin binary\n' > .gitattributes
  printf 'token=%s\n' "$K_ANT" > a.dat; cp a.dat a.bin; git add -A )
expect_blocked "12 staged: a key in a -diff / binary-attribute file" "a\.dat: content matches|leaks found" guard "$R" staged
expect_blocked "12 files: a.dat is checked as text too" "a\.dat: content matches credential" guard "$R" files a.dat a.bin
( cd "$R" && git commit -q --no-verify -m attr )
expect_blocked "12 range: the pushed commit is scanned despite .gitattributes" "a\.bin: content matches credential" guard "$R" range origin/main HEAD
( cd "$R" && git reset -q --hard HEAD~1 && printf '*.dat -diff\n' > .git/info/attributes && echo ok > ok.txt && git add ok.txt )
expect_blocked "12 .git/info/attributes with content is refused" "sets attributes" guard "$R" staged
rm -f "$R/.git/info/attributes"
( cd "$R" && printf 'token=%s\n' "$K_ANT" > b.dat && printf '*.dat -diff\n' > .git/info/attributes && git add b.dat )
expect_blocked "12 key in b.dat under .git/info/attributes is found" "b\.dat: content matches|leaks found|sets attributes" guard "$R" staged
rm -f "$R/.git/info/attributes"

# ── 13. commit messages, tag messages, ref names, author fields ───────────────
R="$T/msg"; new_pushrepo "$R"
( cd "$R" && git checkout -q -b w2 && git commit -q --allow-empty --no-verify -m "msg only $K_ANT" )
expect_blocked "13 range: key in a commit message" "message or author fields" guard "$R" range origin/main HEAD
expect_push_blocked "13 push: key in a commit message" "message or author fields" "$R" refs/heads/w2 w2
( cd "$R" && git checkout -q -b w3 main && GIT_AUTHOR_NAME="n $K_ANT" git commit -q --allow-empty --no-verify -m clean )
expect_push_blocked "13 push: key in the author name" "message or author fields" "$R" refs/heads/w3 w3
( cd "$R" && git checkout -q -b w4 main && git commit -q --allow-empty -m clean4 && git tag -a -m "tagmsg $K_ANT" tg2 w4 )
expect_push_blocked "13 push: key in an annotated tag message" "annotated tag: matches" "$R" refs/tags/tg2 tg2
( cd "$R" && git checkout -q -b w5 main && git commit -q --allow-empty -m clean5 )
expect_push_blocked "13 push: key in a branch name" "pushed ref name: matches" "$R" "refs/heads/br-$K_ANT" "w5:br-$K_ANT"
printf 'fix: thing %s\n' "$K_ANT" > "$T/cm.txt"
expect_blocked "13 commit-msg: key in the message file" "commit message: matches" guard "$R" msg "$T/cm.txt"
printf 'fix: a clean message\n' > "$T/cm2.txt"
expect_pass "13 commit-msg: a clean message passes" guard "$R" msg "$T/cm2.txt"
# the same in CI (scripts and config from the trusted copy)
R="$T/ci"
( cd "$R" && git checkout -q main && git checkout -q -b cimsg main && git commit -q --allow-empty --no-verify -m "ci msg $K_ANT" )
expect_blocked "13 CI: key in a commit message" "message or author fields" run_ci cimsg
( cd "$R" && git checkout -q main && git checkout -q -b ciattr main && printf '*.dat -diff\n' > .gitattributes && printf 'token=%s\n' "$K_ANT" > a.dat && git add -A && git commit -q --no-verify -m attr )
expect_blocked "13 CI: key in a -diff file" "a\.dat: content matches" run_ci ciattr
( cd "$R" && git checkout -q main && git branch -q -D cimsg ciattr )

# ── 14. the gitleaks binary is pinned by SHA-256, not by `version` output ─────
R="$T/pin"; new_repo "$R"
( cd "$R" && printf 'token=%s\n' "$K_ANT" > k.txt && git add k.txt )
FAKE="$T/fakebin"; mkdir -p "$FAKE"
printf '#!/bin/sh\n[ "$1" = version ] && echo %s\nexit 0\n' "$want" > "$FAKE/gitleaks"; chmod +x "$FAKE/gitleaks"
pinned_with_fake_on_path() { ( cd "$R" && PATH="$FAKE:/usr/bin:/bin" bash scripts/secret-guard.sh staged ); }
expect_blocked "14 a fake gitleaks that prints the pinned version is not used" "pinned SHA-256|no pinned" pinned_with_fake_on_path
expect_blocked "14 real gitleaks finds the key" "leaks found|matches credential" guard "$R" staged

# ── 15. binary allowlist pins the blob's SHA-256, never the path ──────────────
R="$T/allow"; new_repo "$R"
( cd "$R" && mkdir -p data && printf '\x00reviewed-binary\x00' > data/foods.bin )
SUM="$(sha256sum "$R/data/foods.bin" | cut -d' ' -f1)"
printf '%s\treviewed in the selftest\n' "$SUM" >> "$R/scripts/binary-allowlist.txt"
expect_pass "15 the reviewed binary (hash listed) passes" guard "$R" files data/foods.bin
( cd "$R" && printf '\x00swapped-binary\x00' > data/foods.bin )
expect_blocked "15 the same path with other bytes is rejected" "unknown binary" guard "$R" files data/foods.bin
( cd "$R" && printf '\x00reviewed-binary\x00' > data/copy.bin )
expect_pass "15 the reviewed bytes under another path are accepted (content pinned)" guard "$R" files data/copy.bin
printf 'data/foods.bin\tpath entry\n' >> "$R/scripts/binary-allowlist.txt"
expect_blocked "15 a path-style allowlist entry is refused" "first column must be the blob's SHA-256" guard "$R" files data/foods.bin

# ── 16. hook installation and the gate's hook check ───────────────────────────
if command -v prek >/dev/null 2>&1; then
  R="$T/inst"; new_repo "$R"
  expect_blocked "16 hooks check fails on a clone without hooks" "missing or not" guard "$R" hooks
  ( cd "$R" && bash scripts/install-hooks.sh >/dev/null 2>&1 && bash scripts/install-hooks.sh >/dev/null 2>&1 )
  expect_pass "16 install-hooks.sh (run twice) leaves hooks the check accepts" guard "$R" hooks
  if grep -q 'secret-guard pre-push wrapper' "$R/.git/hooks/pre-push" && grep -q prek "$R/.git/hooks/commit-msg" && grep -q prek "$R/.git/hooks/pre-commit"; then ok "16 pre-push is the wrapper, pre-commit and commit-msg are prek's"; else no "16 hook files" "wrong content"; fi
  ( cd "$R" && git checkout -q -b t && echo x > x.txt && git add x.txt )
  expect_blocked "16 a real git commit with a key in the message is blocked by commit-msg" "commit message credential scan|Failed|matches credential" bash -c "cd '$R' && git commit -m 'msg $K_ANT'"
  echo '# tampered' >> "$R/.git/hooks/pre-push"
  expect_blocked "16 a modified pre-push hook is detected" "pre-push is missing or not" guard "$R" hooks
fi

printf '%s\n' "${rows[@]}"
echo "secret-guard selftest: $pass passed, $failed failed in $((SECONDS - start)) s"
[ "$failed" -eq 0 ]
