---
name: frontend-review
description: Comprehensive production-readiness review of a single Plantry frontend page. Combines static code analysis (React composition, i18n coverage, accessibility in source) with live browser inspection via agent-browser (real interactions, visual layout, tooltip and hover feedback, keyboard flow, toast notifications, error states). Produces a structured markdown report with per-area 1–5 ratings and a prioritized fix list at the repo root. Use whenever the user asks to "review a page", "audit this page", "check UX on /somewhere", "is this page production-ready", "QA this screen", or names a route/component and wants a quality pass. Prefer this skill over ad-hoc review when the target is a concrete page/route — it is the canonical way to evaluate a page against Plantry's Botanical Atelier design system, UX expectations, and the Vercel React / composition best practices.
---

# Frontend Review

Produce one markdown review file per page. Every section gets a 1–5 rating. The report lives at the **repo root** so the user can open it immediately — not inside `.claude/` and not inside `frontend/`.

## When to use this skill

Use whenever the user points at a single page/route and asks for a quality pass, UX audit, or production-readiness check. Examples: "review the ingredient creation page", "is `/planner` ready to ship?", "audit `ComponentEditor`", "check the weekly planner UX".

One page per invocation. If the user mentions multiple pages, ask which to do first — the review is thorough, not scannable.

## Companion skills

Before writing the review, load context from these skills — the ratings and recommendations must be grounded in their principles, not personal taste:

- **`frontend-design`** — visual quality heuristics, distinctive-vs-generic cues
- **`vercel-react-best-practices`** — React patterns, `use client`, hydration, data fetching
- **`vercel-composition-patterns`** — compound components, render props, prop-proliferation smell
- **`playwright-best-practices`** — only if verifying e2e coverage of the page
- **`agent-browser`** — live interaction; full API usage pattern

You do not need to re-read these every invocation — just when a section's findings depend on them.

## Workflow

### 1. Identify the target

The user names a page by route, component, or feature. Resolve to:

- **Route file**: `frontend/src/routes/<path>.tsx`
- **Primary component(s)**: the files the route renders
- **URL path**: what the user types in the browser (`/ingredients/new`, `/components/new`, `/` for planner)

If the target is ambiguous (e.g., "review the ingredient page" — there's `/ingredients`, `/ingredients/new`, `/ingredients/$id/edit`), ask which one before continuing. Don't guess — the review is expensive and the user's mental model matters.

### 2. Auto-discover related files

Every page pulls in a network of supporting code. You must read enough of it to judge the whole experience, not just the top-level component. Discover in this order:

1. **Route file** — params, `validateSearch`, loaders, what component it renders
2. **Primary component** — usually `frontend/src/components/<area>/<Name>.tsx`
3. **Child components it imports** from `@/components/**` (follow one level deep; deeper only if a child is clearly central to the workflow)
4. **API wrappers** at `@/lib/api/**` used by this page
5. **Query hooks** at `@/lib/queries/**` used by this page
6. **Zod schemas** at `@/lib/schemas/**` that validate the form
7. **i18n keys** — grep `frontend/src/lib/i18n/en.json` and `de.json` for every `t("…")` key referenced, verify both locales have it
8. **Existing tests** — `*.test.tsx` next to the component and any Playwright `frontend/e2e/*.spec.ts` that cover this page (so the review flags coverage gaps)

Use `Grep` and `Read`. Don't spend budget exhaustively mapping every transitive import — stop at the point where you've seen enough to rate a section with confidence.

### 3. Start the dev servers

The live inspection needs both backend (`:8080`) and frontend (`:5173`) running. Check first — don't kill a running server the user has open.

```bash
# Check what's already running
lsof -i :8080 -i :5173 | grep LISTEN
```

If **neither** is running, start both in the background:

```bash
cd backend && go run ./cmd/plantry &   # backend on :8080
cd frontend && bun run dev &           # frontend on :5173 (proxies /api → :8080)
```

Wait until both respond:

```bash
until curl -sf http://localhost:5173 > /dev/null && curl -sf http://localhost:8080/api/health > /dev/null 2>&1; do sleep 1; done
```

If only one is running, start the missing one the same way. Always use `http://localhost:5173/...` for the review — the proxy handles `/api` correctly and exposes the real user experience. Never review against `:8080` directly; the user never sees it that way.

If you started either server, kill them when done:

```bash
kill %1 %2 2>/dev/null
```

### 4. Live inspection via agent-browser

The static pass tells you what the code *says* it does. The live pass tells you what the user actually *experiences*. Both are required — do not skip live inspection because the code looks clean.

Follow this loop for every material area of the page:

```bash
agent-browser open http://localhost:5173/<route>
agent-browser snapshot -i                       # element refs @e1, @e2, …
agent-browser screenshot --output <workspace>/<slug>-initial.png
```

Then probe:

- **Affordance check** — for every `@e` ref that looks interactive, confirm the snapshot shows a cursor/role/aria that matches. A `<div>` acting as a button is a finding.
- **Hover feedback** — `agent-browser hover @eN && agent-browser screenshot` on primary CTAs, icon buttons, cards. If nothing visibly changes, that's a finding — users need to know what's clickable.
- **Tooltip coverage** — every icon-only button without a visible label must have a tooltip. Hover each and capture.
- **Keyboard path** — `agent-browser press Tab` repeatedly, screenshot between presses. The focus ring must be visible and the order must match visual order.
- **Form errors** — submit empty / invalid. Screenshot the error state. The error must say what to fix, not just "invalid".
- **Loading state** — throttle if possible, or submit and capture immediately. A spinner with no context is a finding.
- **Success state** — complete the happy path. Confirm a toast appears, the destination is correct, the user isn't left wondering "did it save?".
- **Responsive** — `agent-browser set viewport 375 812 && agent-browser reload` (the `set viewport` command must be followed by reload for the layout to recompute; a raw screenshot after resize shows the old layout). Overlap, overflow, touch targets under 44px are findings.
- **Empty state** — if the page has lists, test with zero items. Empty must teach, not just say "No items".
- **Motion capture** — open every modal, dropdown, popover, tooltip, and dismiss them. Submit a form and watch for the enter/exit animation. If nothing animates, that is a rating in section 10.
- **Navigation trace** — for every link/button that navigates, click it and note: where did I land, did focus move, did the URL make sense, does the back button return me with state intact?
- **Locale swap** — switch the language once via `/settings` or the language toggle and re-snapshot the page. German strings blow past button widths regularly; English-only review misses these.

Save every screenshot under the review workspace path for the report to link to.

#### Mandatory probes

Beyond the loop above, **every review must run the probes in `references/rubric.md § Workflow probes you must run`**. They exist because section-by-section rating keeps missing a specific class of bug:

1. **Crossed-workflow probe** — run every *pair* of workflows the page supports in non-default order. "Stage image → apply lookup" is not the same path as "apply lookup → stage image" and the two can silently clobber each other.
2. **Input modality completeness** — for every piece of content the page accepts (image, file, URL, text), enumerate the modalities a reasonable user would try: upload, URL, paste, drag-drop, camera on mobile, picker from a library. Missing modalities on a page that asks for that content are findings.
3. **Workflow dead-end probe** — for every concept the form references (units, categories, portions, tags, variants), confirm the user can configure it here. "Save first, then come back and edit" is a dead end; it is a P0 when the concept gates the happy path.
4. **Semantic search / translation probe** — in each locale, search with a real domain-language term that should return a specific well-known item. If "Paprika" (German: bell pepper) returns something else because the query was translated to English "paprika" (spice), the mismatch lives in data plumbing but surfaces as a page bug — flag it.
5. **Multi-locale skim** — switch locale once, skim the page for overflow, truncation, awkward line breaks. Costs 2 minutes, routinely finds P2s.
6. **Entry-point discovery probe** — on a create/new page in its blank state, enumerate every alternative path to the same goal (import, lookup, copy-from-existing, templates, camera/scan) and verify each one is visible *on this page*, not just "reachable from the list". A user who bypassed the list and deep-linked to `/x/new` sees only what this page shows.
7. **Shared-component page-specific probe** — when a component (ImageField, shared combobox, reusable form widget) is used across pages, test it in the current page's workflows; never defer with "same as page X". The same code can misbehave in different contexts.

These probes are not optional. A review that skips them keeps landing on cosmetic findings while state-loss bugs sit unmentioned.

### 5. Write the review

The review file is **always** written to the repo root:

```
/Users/jaltszeimer/Developer/apps/plantry/review-<page-slug>.md
```

Example slugs: `review-ingredients-new.md`, `review-components-new.md`, `review-planner.md`. Overwrite if it exists — the file is a snapshot, not a log.

Use the template in `assets/review-template.md`. Fill every section. If a section doesn't apply (e.g., "Search & Filter" on a form-only page), rate it **N/A** and explain why in one line — don't delete it.

The detailed rubric for each 1–5 scale lives in `references/rubric.md`. Read it the first time you use this skill; skim it after. The rubric exists so ratings are repeatable across pages and reviewers, not improvised.

### 6. Prioritize recommendations

At the end of the report, give a **prioritized fix list**. This is the highest-value part of the review — the ratings help the user see the shape, but the fix list is what they act on.

Every item follows this exact shape:

```
### P<0|1|2>: <short title>

**What**: one sentence describing the change.
**Why**: the user impact — what breaks, confuses, or slows them down today.
**Benefit if applied**: concrete improvement the user will feel.
**Cost of ignoring**: what the page stays like — be specific, not "worse UX".
**Effort**: S / M / L.
```

Priority bands:

- **P0** — users get stuck, data loss risk, accessibility blockers, hardcoded strings missing from i18n. Ship-blockers.
- **P1** — clarity or consistency issues that don't stop the user but erode trust. Fix before next release.
- **P2** — polish. Would make the page feel premium. Fine to backlog.

Do not invent P0s. If the page is genuinely solid, say so — a review that finds nothing critical is a legitimate outcome and more useful than padded concerns.

## Critical rules

- **Repo root, not anywhere else.** The report path is always `<repo>/review-<slug>.md`. Not `.claude/`, not `frontend/`, not a workspace subdirectory.
- **One page per invocation.** Do not review multiple pages in one pass — the depth collapses.
- **Rate every section.** Missing ratings make the scorecard unreadable. Use N/A with a reason if needed.
- **Ground every finding.** Every rating and recommendation must cite specific code (`path:line`) or a screenshot. "Feels cluttered" without evidence is noise.
- **i18n is non-negotiable.** Any user-visible string not passed through `t()` is a P0. Any key missing from `en.json` or `de.json` is a P0. The app must work in both locales.
- **Do not rewrite the code.** The review proposes; the user disposes. If the user asks for fixes afterwards, that's a separate pass.
- **Preserve server state.** If the user had the dev server running before the review, do not kill it when done.

## Why this shape

The user needs two things from this review: **a map** (where is the page strong and weak?) and **a plan** (what should I fix first?). The ratings give the map. The prioritized list gives the plan. Everything else — the workflow walk-through, screenshots, i18n checks — exists to make those two outputs trustworthy.

When you're tempted to write a paragraph of general observations, ask: does this change a rating or add to the fix list? If not, cut it. The review is read by someone who already knows the page — they need evidence and actions, not narration.
