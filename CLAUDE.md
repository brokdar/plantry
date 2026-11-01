# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Plantry

Self-hosted weekly meal planner. Single Go binary embeds React SPA, serves from one SQLite file. Must cross-compile to ARM (no CGO).

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
E2E_LANE=N scripts/gate.sh [specs]    # backend + frontend checks in parallel, then e2e
```

Runs `go test -race`, `go vet`, `golangci-lint`, `check-sqlc-usage.sh`, then `bun run check`, `bun run build`, then the full e2e suite or only the named specs. Skip parts with `GATE_SKIP_BACKEND=1`, `GATE_SKIP_FRONTEND=1`, `GATE_SKIP_E2E=1`. `E2E_LANE` (1-9) gives each worktree its own backend/Vite ports and DB so several can run e2e at once (see `frontend/playwright.config.ts`); never use lane 0 (port 8080/5173) from a worktree. Prefix every e2e run with it: `E2E_LANE=N bun run e2e -- <specs>`.

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
