# Review instructions

Review-only rules for every reviewer of a Plantry change: the `pr-reviewer` agent, the specialist agents, `/code-review` and Anthropic's Code Review. General project context lives in `CLAUDE.md`, `backend/CLAUDE.md` and `frontend/CLAUDE.md`; a violation of a rule stated there is a finding.

Plantry is a single-user, LAN-only, self-hosted app: one Go binary, one SQLite file, no auth by design. A reviewer's job is to find what breaks the household's data or plan, or what the merge gate cannot see.

## What to read

1. The diff and the PR body (the claim: what it says it does, and the gate output it pastes).
2. For each changed file, the code around the change: callers, the tests that cover it, the section of `backend/CLAUDE.md` or `frontend/CLAUDE.md` that describes the area.
3. Never run the gate, `go test -race` or e2e (the author did, and the output is in the PR body). A targeted `go test ./pkg -run X` or `grep` to confirm a finding is fine.

## What Important means here

Important is a bug or rule break that should be fixed before merging. Everything else is Nit at most.

**Data and integrity**
- A migration that can lose or corrupt data: dropping a table with cascading foreign keys instead of the safe rebuild in `backend/CLAUDE.md`, a non-backward-compatible schema change, a wrong goose number or no migration test.
- A plate, component or cook session written outside `plate.Service`; a multi-write operation outside one `InTx`; a result that ignores that a nested `InTx` has no savepoints.
- Undo that does not restore what the change removed (`PlanSnapshot`, revert token), or a change that cannot be undone and says nothing.
- A broken invariant: a cell holds at most one plate, a plate at most one main, the hard rules H1-H9. The H9 safety floor follows the normal target, never a light, hearty or shifted one.
- SQLite misuse: a bare `_` on `db.Exec`, `tx.Commit`, `rows.Scan` or `rows.Err()`; a missing `defer tx.Rollback()`; unsanitized FTS5 input; raw SQL outside `queries/*.sql`.

**Build and architecture**
- CGO or an architecture-specific dependency (the image is amd64, arm64 and arm/v7; `mattn/go-sqlite3` is never allowed).
- A layer break: `transport → domain ← adapters`; a service that takes a concrete type instead of a repo interface.
- An unchecked secret path: API keys stored unencrypted, `secret.key` or a key in a log, debug endpoints outside `DEV_MODE`, a fetch that skips the URL guard.

**Planner and chat agent**
- Code owns the facts, the model owns the language: a number, date, rule or refusal that a prompt now asks the model to compute or remember instead of code.
- A prompt or tool-schema change that makes a prompt longer without removing something (prompts must not grow), or one that changes behavior without bumping its version (planner `pNN`, chat `vNN`, heuristic `hNN`).
- A user-facing string that uses internal words (band, floor, cap, soft, session, draft, ISO dates); the notes pass through the kitchen-words rewrite and a test pins the word list.
- A tool that reports a partial or refused change as done, saved as unsaved, or hides a rule conflict from the model.
- An eval or lint change that bends the check to make a result pass (evaluator integrity: whoever optimizes the product never edits the checks).

**Tests and the gate**
- A test that cannot fail for the bug it names (asserts the mock, the constant it just set, or nothing about the changed behavior), or a deleted or weakened assertion without a reason.
- A breach of the test speed contract: an own SQLite DSN, `-count=1`, `time.Sleep` for a clock, a rebuilt fixture per test, a raised timeout or time budget without a `reasons` entry in `test-budget.json`.
- A claim in the PR body that the diff does not support ("done", "all callers updated", "no behavior change"), or no gate output for a change that needs it.

**Frontend**
- Server data mirrored in client state (TanStack Query only); a `useMutation` whose call sites own invalidation or `toastError` instead of the hook; a plan mutation that skips `invalidatePlanViews`; an Undo built per screen instead of the shared hooks.
- A hardcoded user-visible string (en and de keys both, units included), a hand-edited `routeTree.gen.ts`, or a bypass of the Plan or Today read models.
- An e2e spec that uses `waitForTimeout`, `waitForURL` after a submit, a literal `localhost:8080`, an unpinned `chooser`, or the `?assistant=1` start; real generation or AI state outside its project.

**Stray artifacts** (any of these in the diff is Important)
- Screenshots, scratch docs, plan or loop files (`plan.md`, `*-report.md`; plans live in `~/.claude/plans/plantry/`), `.claude/settings.local.json`, `*.db-shm`, `*.db-wal`, paid-run data, built `webui/files` assets, secrets.

## Nit

At most: naming, comments, a stale sentence in a `CLAUDE.md` that the diff made wrong, a missing edge-case test for a path that already has tests, an avoidable allocation off the hot path, structure that works but is harder to read than it needs to be.

## Pre-existing

A bug in code the diff only touches or sits next to, not introduced by it: report it as Pre-existing, at most three per review, and only when it would corrupt data or mislead the user. Never block on it.

## Cap the nits

Report at most five Nits. When more exist, keep the five that change what a future author does and drop the rest without mentioning them.

## Do not report

- Anything the gate already enforces: `go vet`, `golangci-lint`, `deadcode`, `check-sqlc-usage.sh`, `check-test-patterns.sh`, the budget and timeout checks, frontend eslint, typecheck, knip, `check-legacy.sh`, the i18n key-parity and token tests, formatting.
- Generated or vendored files: `internal/adapters/sqlite/sqlcgen/`, `routeTree.gen.ts`, `components/ui/**`, lockfiles.
- Missing auth, HTTPS, CSRF or multi-tenant scoping: Plantry is single-user and LAN-only on purpose.
- Style the author could reasonably choose either way: function length, nesting, "interface pollution", table-driven vs. not.
- Speculation: "might", "could", "consider". A finding needs a concrete input or state that goes wrong.
- Anything already settled in an earlier review round of the same PR unless the fix is wrong.

## How to write a finding

- One issue per finding, anchored to the `file:line` it breaks.
- Three parts, in order, each one sentence: **what is wrong**; **how it fails** (a concrete input or state and the wrong output or damage); **the fix** (the smallest change; a code snippet only when the words are ambiguous).
- Behavior claims need a `file:line` citation in the source, not an inference from a name.
- No praise, no restating the diff, no questions to the author unless the answer decides the finding.
- A finding that cannot name its failing case is dropped, not downgraded to a Nit.

## Verify before posting

Every candidate is tried against these before it is posted; one yes drops it:
1. Does a caller, a guard, a transaction or a type already prevent it? Read the code that would.
2. Does an existing test already cover and pass this path?
3. Is it one of the Do-not-report items?
4. Is it introduced by the diff? If not, it is Pre-existing or dropped.

## Summary

Start with one verdict line: `No Important findings` or `N Important, M Nit`. Then the findings, Important first. Then one line on what was checked and not (for example "migration 00054 and the plate write path read; frontend not touched"). No more than ten lines besides the findings.
