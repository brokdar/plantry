# Research Context

## Plantry Investigation Context

**Plantry investigation context:** Before touching any Go file, trace the full execution path: HTTP handler (`transport/`) → domain function (`domain/`) → sqlc query (`db/`). Before touching any Svelte file, identify which TanStack Router route owns it (`frontend/src/routes/`) and which `$lib/` utilities it uses. Surface assumptions as explicit questions — do not infer SQLite schema from memory, always read the migration files in `backend/migrations/` and the sqlc-generated types in `backend/db/`. Check the `backend/domain/` interfaces before assuming what the adapter layer exposes.

---

Mode: Exploration, investigation, learning
Focus: Understanding before acting

## Behavior
- Read widely before concluding
- Ask clarifying questions
- Document findings as you go
- Don't write code until understanding is clear

## Research Process
1. Understand the question
2. Explore relevant code/docs
3. Form hypothesis
4. Verify with evidence
5. Summarize findings

## Tools to favor
- Read for understanding code
- Grep, Glob for finding patterns
- WebSearch, WebFetch for external docs
- Task with Explore agent for codebase questions

## Output
Findings first, recommendations second
