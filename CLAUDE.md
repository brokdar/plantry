# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# Plantry

Self-hosted weekly meal planner. Single Go binary embeds React SPA, serves from one SQLite file. Must cross-compile to ARM (no CGO).

- **Backend detail:** see [`backend/CLAUDE.md`](backend/CLAUDE.md)
- **Deployment context:** single-user, LAN-only — no auth required or expected.

## Commands

### Backend (`cd backend`)

```bash
go build ./...                        # compile
go test ./...                         # all tests
go test -run TestName ./path/...      # single test
go vet ./...                          # vet
golangci-lint run                     # lint
sqlc generate                         # regenerate from queries + migrations
go test -race ./...                   # race detector — run before feature complete
```

### Frontend (`cd frontend`)

```bash
bun run dev     # dev server (port 5173, proxies /api → :8080)
bun run check   # lint + typecheck + unit tests
bun run e2e     # playwright (auto-spawns backend + frontend via webServer config)
```

See [`frontend/CLAUDE.md`](frontend/CLAUDE.md) for full frontend commands, architecture, and testing rules.

### Docker

```bash
docker compose up --build             # build + run (port 8080)
```

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
