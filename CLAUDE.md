# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Plantry

Self-hosted weekly meal planner. Single Go binary embeds React SPA, serves from one SQLite file. Must cross-compile to ARM (no CGO); the release image is `linux/amd64`, `linux/arm64` and `linux/arm/v7`. `plantry backup -out <file>` is the supported backup (see `README.md`).

- **Backend detail:** see [`backend/CLAUDE.md`](backend/CLAUDE.md)
- **Deployment context:** single-user, LAN-only — no auth required or expected.

## Commands

Backend and frontend commands live in [`backend/CLAUDE.md`](backend/CLAUDE.md) and [`frontend/CLAUDE.md`](frontend/CLAUDE.md).

### Docker

```bash
docker compose up --build             # build + run (port 8080)
```

### Merge gate (repo root)

```bash
E2E_LANE=N scripts/gate.sh --fast [specs]  # T1, per fix batch (~1 min)
E2E_LANE=N scripts/gate.sh [specs]         # T2, once per PR / hand-back (~2.5 min)
```

Both run the cheap checks first (`go vet`, `golangci-lint`, the `GOARCH=arm64` and `GOARCH=arm GOARM=7` cross builds with `CGO_ENABLED=0`, `check-sqlc-usage.sh`, `go tool deadcode -test ./...` (nothing outside `sqlcgen/`), `scripts/check-test-patterns.sh`, the timeout and budget checks, frontend lint/typecheck/knip), so a slip fails in seconds. **T1** then runs `go test ./...` without race and with the test cache on (only changed packages run), the unit tests of changed frontend files (`vitest related`) and only the named e2e specs. **T2** runs, in parallel: `go test ./...` then `go test -race -short` (the SQLite engine is not race-instrumented), vitest + build, and the full e2e suite; then checks the time budgets. Every step prints its time; logs go to a fresh `/tmp/plantry-gate-<lane>-*` directory; one T2 runs at a time on the machine (`flock`); a clean tree that passed a mode exits at once. Skip parts with `GATE_SKIP_BACKEND=1`, `GATE_SKIP_FRONTEND=1`, `GATE_SKIP_E2E=1`. `E2E_LANE` (1-9) gives each worktree its own ports and DBs so several can run e2e at once (see `frontend/playwright.config.ts`); never use lane 0 (port 8080/5173) from a worktree. Prefix every e2e run with it: `E2E_LANE=N bun run e2e -- <specs>`.

**Disk limits.** `scripts/disk-guard.sh` runs first in the gate: it fails when `/` or `/mnt/c` (the WSL vhdx) has under 30 GB free (`DISK_MIN_FREE_GB`), runs `go clean -cache` when the Go cache passes 20 GB (`GOCACHE_MAX_GB`), and fails when `~/.local/share/plantry-dev` plus the Go cache pass 60 GB (`DEV_DATA_MAX_GB`). The gate builds with `-trimpath`, so worktrees share cache entries. One worktree per live task, removed (`git worktree remove`, `git worktree prune`) when its PR merges. Paid-run data in `~/.local/share/plantry-dev/loop` can be packed for archiving with `python3 -I m3/dbpack.py pack <series>`; `run.py` rescore and replay read packed series directly through temporary copies and never unpack or re-pack them, and replay output stays raw (about 0.7 GB per dev replay), so delete or pack replay series you no longer need. `go clean -cache` is safe anytime.

### Test speed contract

Verification speed is iteration speed. These rules hold for every change and every agent; `scripts/gate.sh`, CI and the `test-hygiene` hook enforce them.

**Which tests when.**

| Tier | When | What |
|---|---|---|
| T0 | every edit | `go test ./<pkg> -run '<Test>'` (no race, cache on); `bunx vitest related --run <files>` |
| T1 | each fix batch / commit handed back | `scripts/gate.sh --fast [specs you touched]` |
| T2 | once per PR or milestone hand-back, on the final commit | `scripts/gate.sh` |

Never run `go test -count=1` (it switches off the test cache, which is what runs only the affected packages) or `go test -race` without `-run` outside the gate; only to chase a flake or a data race.

**Slow tests and timeouts.** When a test or CI step is slow or times out, never raise the timeout or a budget first: profile (`go test -run X -cpuprofile cpu.out ./pkg`, `go tool pprof -top`, or the per-test times in the gate log), find the cause, fix it. Timeouts are tripwires for hangs and are frozen in `test-budget.json`; a change needs `reasons["timeouts:<file>"]`.

**Time budgets (`test-budget.json`, `scripts/test-budget.py`).** Tests may get slower only in proportion to the tests added: per Go package (CPU seconds of its test binary, no race), the race tier, vitest and e2e, `allowed = budget x (1 + tolerance) + new tests x budget / tests`. Anything over fails T2 and CI. Past it only with a higher budget in the same PR and a reason under `reasons` saying why it costs this much and why it cannot be faster (CI checks the reason against `origin/main`, and the PR body lists it under "Test budget changes"). A leaf test over `single_test_max_s` fails unless it calls `testhelper.Slow(t, reason)` (skipped under `-short`, so the race tier leaves it out; the no-race pass runs it) and is listed under `slow_tests`. `--write` lowers beaten budgets (the ratchet) and never raises one; run it when a PR makes tests faster. CI (GitHub's 4-vCPU runner) is the reference machine: a test that is fast on 28 local cores can be 10x slower there, so budgets count CPU, not wall. The gate runs each test binary with `GOMAXPROCS=4` (CI's size; fewer idle scheduler threads to spin) and probes the machine speed during the run (`scripts/cpu-exec.py` with `CALIB_BIN`: a 0.5 CPU-s probe every 8 s from the 4th second, the best counts; a binary that ends sooner is probed once afterwards if it used over 1 CPU-s), so a package's CPU is scaled by the speed under the load it ran in, not the quieter one at the end of the gate. Binaries that use 1 CPU-s or less get no calibration (they use the end-of-run speed). The single-test check divides each leaf's elapsed time by that package's measured slowdown (never below 1), so a loaded machine does not fail a CPU-bound test but a test that waits (channel, I/O) still counts in full; with no calibration (CI) it is the raw wall time against 30 s.

**Anti-patterns** (each once cost minutes of gate time):
- A fixture rebuilt in every test or subtest: build it once per process and copy the database (`testhelper.SnapshotDB` / `NewTestDBFrom`; the agent week fixture does).
- An own SQLite DSN in a test: use `testhelper.NewTestDB`, `DSN` or `DSNWith` (fsync off; planning went from 50 s to 8 s).
- A scenario sweep in a unit test, or one run serially: run it once, in parallel (`eval.Config.Parallel`), and give a golden test the fewest scenarios that cover its format.
- A full re-evaluation with options inside a candidate loop: candidate checks use `evaluateWith(..., noOptions)`; a per-call reload of the plan state (the `Builder` memo covers it) or of the same SQL (the statement cache covers it).
- `time.Sleep` to wait for a clock: set the stored time instead.
- A serial Playwright project at the end of the run: give the specs their own server and data (global-settings has its own) instead of serializing them.
- Polling a background job in a loop: the background task notifies when it ends.

## Secrets

No key, token, password, private key, `secret.key`, `.env` or credential-carrying DB may land in any branch, commit, tag or PR of this public repo. Layers, so one bypass is not enough. **Preventive** (before anything leaves the machine): 1, 2, 5 and GitHub push protection. **Detective** (after the push): 4. CI runs after the push, so it cannot keep a secret out of the repo, only flag it.

1. **prek pre-commit:** `gitleaks` over the staged diff (pinned in `mise.toml`; `.gitleaks.toml` is the vendored default rules with the global path allowlist removed, plus ours), blocked file names (case-insensitive: `.env*` except `*.example`, `secret.key`, `*.pem`, `*.key`, ssh keys, `.claude/settings.local.json`, DB names incl. `.db3`/`-wal`/`-shm`), and `scripts/check-db-secrets.py` on every staged file: SQLite magic on any name, credential patterns in binaries (`scripts/secret-patterns.txt`, shared with `TestFixtureDBsCarryNoCredentials`), archives and unknown binaries rejected unless listed with a reason in `scripts/binary-allowlist.txt`.
2. **prek pre-push:** the same scans over every ref being pushed (merge commits included, `gitleaks -m`), plus a refusal of commit messages carrying a GitHub skip-ci marker.
3. **`scripts/gate.sh`:** fails when the prek hooks are not installed, scans `origin/main..HEAD` and every tracked file, runs the hook tests and `scripts/secret-guard-selftest.sh`, which replays every known bypass in a throwaway repo with fake keys.
4. **CI `secrets.yml`:** full-history gitleaks (pinned, SHA-256 checked) plus the file checks, on every PR (`pull_request_target`, so the workflow and all scripts and configs come from `main`, never from the branch under test), push and tag. A PR that changes a guard file (`scripts/secret-guard-files.txt`) fails unless the owner puts the label `secret-guard-change` on it. GitHub skips workflows when the head commit message contains `[skip ci]` (and similar markers); that cannot be switched off, so require the `scan` check on `main` with a ruleset (below) and treat the local layers as the real prevention.
5. **Claude Code hook** `block-hook-bypass.js`: a shell tokenizer that blocks `--no-verify` (and abbreviations) / `-n`, `SKIP=`, `GIT_DIR` and `GIT_CONFIG*` redirection, `git -c`, `core.hooksPath`, touching `.git/hooks` from any cwd, `git add -f`, and Bash writes to the guard files (Edit/Write are fine), also inside `bash -c`, `eval`, heredocs and `$(...)`.

**Owner setting (not done by agents):** Settings > Rules > Rulesets > New branch ruleset, target `main`, enable "Require status checks to pass" and add the check `scan` (the job of `secrets.yml`), plus "Require a pull request before merging". A required check that is skipped by `[skip ci]` stays pending and blocks the merge.

Install the hooks once per clone: `prek install --hook-type pre-commit --hook-type pre-push`. Never bypass a hook. If a scan fires: stop, remove the value from the commit (and from history if committed), rotate the credential, and never allowlist a real secret in `.gitleaks.toml` or `.gitleaksignore` (those are for reviewed false positives only). Fixture DBs must be built without real settings and `VACUUM`ed.

## Review

[`REVIEW.md`](REVIEW.md) holds the rules every reviewer follows: what counts as Important, what not to report (anything `scripts/gate.sh` enforces), the nit cap, how a finding is written and verified. Review every PR with the `pr-reviewer` agent (`.claude/agents/pr-reviewer.md`: find, then try to refute each candidate); `go-reviewer` and `silent-failure-hunter` are specialists beside it. `/code-review` reads this file but not `REVIEW.md`, so when you run it, read `REVIEW.md` first and apply it. Keep `REVIEW.md` to review behavior; project rules stay here and in the backend and frontend files. When a finding recurs in review, add a rule to `REVIEW.md`; when a check moves into the gate, add it to "Do not report".

## Required Skills

Load these before writing or modifying the relevant code:

- **`golang-patterns`** — Go code
- **`golang-testing`** — Go tests
- Frontend skills: see [`frontend/CLAUDE.md`](frontend/CLAUDE.md)

## Architecture

### Backend — Hexagonal-Lite (`transport → domain ← adapters`)

See [`backend/CLAUDE.md`](backend/CLAUDE.md) for the full directory breakdown, sqlc workflow, dependencies, and SQLite gotchas.

### Frontend — TanStack Router (file-based)

See [`frontend/CLAUDE.md`](frontend/CLAUDE.md) for the full directory breakdown, data layer, design system, and testing rules.

## Mandatory Guidelines

### 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:

- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

### 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

### 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:

- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:

- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

### 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:

- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:

```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```
