# Plantry — Agent-Browser Test Spec

Empirical validation of PRODUCT.md against the running Docker build (`docker compose up`, http://localhost:8080). Scope is deliberately narrow: only flows that existing Playwright tests **do not** cover end-to-end. Playwright seeds most fixtures via direct backend POSTs, stubs the LLM with a fake adapter, and never clicks through the real first-run UI. Those are the gaps this spec targets.

## Preconditions

- Fresh volume: `docker compose down -v && docker compose up -d --build`.
- Image built from current branch (`feat/variant-components`).
- `PLANTRY_AI_PROVIDER`, `PLANTRY_AI_MODEL`, `PLANTRY_AI_API_KEY`, `PLANTRY_FDC_API_KEY` exported in shell before compose.
- `/api/health` returns `{status: "ok"}`.
- Browser: agent-browser (Chromium headless). Camera-dependent flows are marked SKIPPED.

## Report shape — per scenario

| Field | Content |
|---|---|
| Scenario | Short name |
| PRODUCT claim | Line/section the feature maps to |
| Steps | What was clicked/typed |
| Expected | What PRODUCT.md promises |
| Observed | What actually happened |
| Result | PASS / FAIL / SKIPPED (reason) |

---

## Scenario 1 — Golden path (PRODUCT §11, Success Criteria)

A first-hour user with an empty catalog reaches a planned week. This is the single most important claim in the product doc.

Steps:
1. Open `/` on a fresh volume. Expect empty planner, empty component library, welcome or empty-state affordance.
2. Go to Settings → add at least one time slot (`Dinner`) — default slots are NOT seeded (ARCH §4.4).
3. Ingredients → add 2–3 ingredients manually (no barcode, no FDC) with kcal/P/F/C.
4. Components → create one `main` and one `side_starch` using those ingredients.
5. Planner → click the `Dinner` cell on Monday → pick the `main` → add the side.
6. Open the shopping panel → verify aggregated grams match the ingredients × portions.
7. Open the nutrition panel → verify day + week totals are non-zero and align with per-portion macros.

Pass criteria: all seven steps completable without docs, empty states guide the user, no console errors.

## Scenario 2 — Real OFF barcode lookup

Playwright never calls real Open Food Facts.

Steps:
1. Ingredients → New → enter a real barcode via the manual input (since the agent-browser cannot activate the camera).
   - Suggested barcode: `3017620422003` (Nutella 400 g).
2. Expect the form to populate with name + macros from OFF.
3. Save. Verify the `source` column reads `off` and macros are plausible.

Pass criteria: lookup succeeds, fields auto-fill, duplicate-on-retry is prevented by name unique constraint.

Failure to note: PRODUCT promises "local catalog checked first to avoid duplicates" — verify by repeating the scan/entry.

## Scenario 3 — Real FDC name search

Requires `PLANTRY_FDC_API_KEY`.

Steps:
1. Ingredients → New → switch to name-search lookup → type `chicken breast`.
2. Expect auto-selected best match with 100 g macros.
3. Save. Confirm `source=fdc` and `fdc_id` populated.

Pass criteria: non-empty result list, one-click add.
Skip reason (if key missing): mark SKIPPED-NO-KEY.

## Scenario 4 — URL recipe import (JSON-LD path)

Playwright tests the pasted-HTML path. This tests the live fetch.

Steps:
1. Import → paste a public URL with JSON-LD Recipe schema. Suggested: a Chefkoch or AllRecipes page.
2. Expect draft name/portions/instructions/image populated, ingredient rows listed.
3. Resolve each ingredient against the local catalog (which is nearly empty — most will fall back to FDC/OFF or require manual match).
4. Save as a component.

Pass criteria: draft appears without LLM usage (JSON-LD path hits no AI cost), resolver shows candidates per row, final component is first-class.

## Scenario 5 — URL import LLM fallback

Requires `PLANTRY_AI_API_KEY`.

Steps:
1. Import a URL with no JSON-LD (a blog-style recipe page). Candidates: serious-eats post, a substack recipe, etc.
2. Expect the LLM extractor fills the draft.

Pass criteria: draft populated with plausible fields, no 500. SKIPPED-NO-KEY if absent.

## Scenario 6 — AI agent tool loop (real LLM)

Requires `PLANTRY_AI_API_KEY`. Playwright uses a scripted fake; this validates the real adapter.

Steps:
1. Planner → open chat panel → "fill my empty week with three quick-weekday dinners under 40 minutes".
2. Watch SSE events: streamed text, tool-call cards, `plate_changed` events, plates updating live.
3. Verify at least one `create_plate` + `add_component_to_plate` tool call landed.
4. Verify no runaway loop (cap at 20 iterations per ARCH §7.2).
5. Verify the agent respects `cook_minutes ≤ 40` on chosen components (if any exist).

Pass criteria: streaming works, grid re-renders without a full reload, `ai_conversations` row created. SKIPPED-NO-KEY if absent.

## Scenario 7 — Feedback → profile.preferences → next generation (cross-feature loop)

PRODUCT §"I want the AI to learn what I like" claims a feedback loop that influences future picks. No existing e2e test couples the three steps.

Steps:
1. Create plate with a component (say "Mushroom risotto").
2. Mark the plate `disliked ✗` with note "too earthy".
3. Inspect `/api/profile` JSON — expect `preferences` to reflect the dislike (a `dislikes` array or similar, per `record_preference` tool schema).
4. Start a fresh AI chat, ask for a recommendation; verify the system prompt or output reasoning avoids the disliked component/ingredient.

Pass criteria: preferences persist; AI picks reference them. If the app does not auto-update preferences from feedback (it may only do so when the AI calls `record_preference`), note the gap.

## Scenario 8 — Variant component creation + planner swap

Current branch is `feat/variant-components`; this is the hot path.

Steps:
1. Create component "Chicken curry" (role=main).
2. From its detail page, create a variant "Tofu curry" in the same variant group.
3. From another variant "Chickpea curry".
4. Verify the siblings list on any of the three shows the other two.
5. Plan the Monday dinner with Chicken curry. On the plate chip, use swap → expect variant siblings to appear as suggestions.
6. Swap to Tofu curry.

Pass criteria: variant group reachable from each sibling, swap surfaces siblings prominently, swap succeeds without a page reload.

## Scenario 9 — Week copy + shopping list aggregation

Steps:
1. With Monday/Tuesday/Wednesday plated, use "Copy week" → target next week.
2. Navigate next week; expect same plates present.
3. Shopping panel should aggregate ingredients by name in grams.
4. Edit one plate's portions from 1 → 2 on the copy; shopping totals should update.

Pass criteria: copied plates independent (editing one does not change the other), shopping math matches `portions × component grams`.

## Scenario 10 — Destructive / error edges

Steps:
1. Delete an ingredient referenced by a component → expect 409 + `error.ingredient.in_use`.
2. Delete a component used on a plate → expect 409 + `error.component.in_use`.
3. Delete a time slot used by a plate → expect 409.
4. POST a plate with an unknown `slot_id` → expect 4xx with `error.plate.slot_unknown` (verify via UI or network tab).
5. Disconnect backend (stop container) → UI shows readable error toast, not a blank crash.

Pass criteria: every failure surfaces an i18n key the user can read, no stack traces leak to the UI.

## Scenario 11 — Empty + zero-state UX

PRODUCT promises "empty catalogs are OK". Most Playwright specs seed fixtures before rendering.

Steps (on fresh volume):
1. `/` — empty planner. Does it tell the user they need to add slots/components, or just show a blank grid?
2. `/components` — empty list. Is there a visible CTA to create?
3. `/ingredients` — same.
4. `/templates` — same.
5. `/archive` — no past weeks. Does it show an empty state rather than an error?

Pass criteria: every primary route has a non-blank empty state with a CTA or explanation. Count any blank page as a FAIL.

## Scenario 12 — Visual polish (§"Botanical, warm, calm")

Agent-browser can take screenshots; subjective but recordable.

Steps:
1. Screenshot `/`, `/components`, `/settings`.
2. Check dominant palette is sage/amber (OKLch tokens from ARCH §2).
3. Confirm no raw 0 0 0 black; buttons have focus rings.
4. Check dark mode toggle (if present) mirrors cleanly.

Pass criteria: matches "botanical, warm, calm" claim. Color pipette unnecessary; eyeball + screenshot evidence.

---

## Coverage notes (findings BEFORE the walkthrough)

- **Pantry tracking:** out of scope per §"Not a pantry tracker." Do not test.
- **Barcode camera path:** cannot be tested headless. Mark as skipped with reason.
- **Offline mode / local-first:** §10.1 "offline-tolerant" — we can validate by stopping the container mid-interaction and checking graceful degradation.
- **AI provider switch at runtime:** ARCH §6.2 notes `PUT /api/settings/ai/*` is deferred. Config comes from env vars. Don't test a UI that doesn't exist.
- **i18n DE locale:** locale switcher should be in Settings. One quick flip + spot-check translated string.
