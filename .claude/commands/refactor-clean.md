---
description: Safely identify and remove dead code with verification after each change.
---

# Refactor Clean

Safely identify and remove dead code with test verification at every step.

**Project root:** `/Users/jaltszeimer/Developer/apps/plantry`
**Stacks:** Go backend (`backend/`) · TypeScript/Svelte frontend (`frontend/`)
**Agent:** invoke `refactor-cleaner` to run this workflow autonomously.

## Step 1: Detect Dead Code

Run the appropriate tool for each stack:

| Stack | Tool | Command | Run from |
|-------|------|---------|----------|
| Go backend | deadcode | `deadcode ./...` | `backend/` |
| Svelte/TS frontend | knip | `bunx knip` | `frontend/` |

Run both in parallel. If a tool is unavailable, use Grep to find exports with zero imports.

## Step 2: Categorize Findings

Sort findings into safety tiers:

| Tier | Examples | Action |
|------|----------|--------|
| **SAFE** | Unused variables, unreachable code, imports used nowhere | Delete with confidence |
| **CAREFUL** | Exported functions/types with no apparent external callers | Verify — may be consumed by sqlc-generated code or the domain/adapter boundary |
| **RISKY** | Anything touching sqlc-generated types, the domain/adapter boundary, or entry points | Investigate thoroughly before touching |

## Step 3: Safe Deletion Loop

For each SAFE item:

1. **Run full test suite** — Establish baseline (all green)
   - Backend: `go test ./...` (from `backend/`)
   - Frontend: `bun run check` (from `frontend/`)
2. **Delete the dead code** — Use Edit tool for surgical removal
3. **Re-run test suite** — Verify nothing broke
4. **If tests fail** — Immediately revert with `git checkout -- <file>` and skip this item
5. **If tests pass** — Move to next item

## Step 4: Handle CAREFUL Items

Before deleting CAREFUL items:

- Grep for all references, including string-based lookups
- Check if the symbol is referenced from sqlc-generated files in `backend/db/`
- Check if the symbol sits on the domain/adapter boundary (`backend/domain/`, `backend/adapters/`)
- Verify no dynamic usage patterns

## Step 5: Skip RISKY Items

Do not delete RISKY items without explicit user instruction. Report them separately so the user can decide.

## Step 6: Consolidate Duplicates

After removing dead code, look for:
- Near-duplicate functions (>80% similar) — merge into one
- Redundant type definitions — consolidate
- Wrapper functions that add no value — inline them
- Re-exports that serve no purpose — remove indirection

## Step 7: Summary

Report results:

```
Dead Code Cleanup — Plantry
──────────────────────────────────────────
Backend (Go):
  Deleted:   N unused functions / variables
  Skipped:   N items (tests failed or RISKY)

Frontend (Svelte/TS):
  Deleted:   N unused exports / dependencies
  Skipped:   N items (tests failed or RISKY)

Lines removed: ~N
──────────────────────────────────────────
All tests passing ✓
```

## Rules

- **Never delete without running tests first**
- **One deletion at a time** — Atomic changes make rollback easy
- **Skip if uncertain** — Better to keep dead code than break production
- **Don't refactor while cleaning** — Separate concerns (clean first, refactor later)
- **never remove during active feature development**
