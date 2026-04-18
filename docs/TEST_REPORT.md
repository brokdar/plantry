# Plantry — Agent-Browser Test Report

**Date:** 2026-04-17
**Build:** `feat/variant-components`, image built from current branch
**Env:** Docker Compose, fresh volume, real OpenAI (`gpt-5.4-nano`) + FDC keys loaded
**Runner:** agent-browser (headless Chromium)

## TL;DR

| Area | Verdict |
|---|---|
| Empty states, navigation, theme | PASS — every route has a clean empty state with CTA |
| Time slots | PASS with UX concern (dev-facing labels) |
| FDC name lookup | PASS — returns USDA results, "Recommended" badge, auto-populated form |
| OFF barcode lookup | Backend PASS, UI **BUG** — text-field path never hits barcode endpoint |
| Component editor | PASS (after retest — initial failure was test-harness artifact) |
| Planner, add/remove plate, feedback buttons | PASS |
| Swap picker | PASS functionally, variant siblings not prioritized |
| Shopping list | PASS — aggregated grams match API |
| Nutrition panel | PASS conditional — silently hidden until profile `kcal_target` is set |
| Week copy | PASS |
| Delete-in-use 409 + i18n keys | PASS |
| Variant component creation | PASS backend; empty-starts UX concern |
| URL import (JSON-LD, real site) | PASS graceful fallback message; chefkoch blocks fetch |
| **AI agent with real OpenAI** | **FAIL — HARD BREAK** — tool schema rejected by OpenAI |

Playwright covers breadth; agent-browser against the real build surfaced **one P0 hard break, three P1/P2 UI bugs, and several UX concerns**.

---

## Scenario-by-scenario

### 1. Golden path (PRODUCT §11) — PASS after retest
Fresh volume → configured Dinner slot → added 2 ingredients (FDC chicken + manual rice) → composed a 2-ingredient component with edited 200 g amount → planted on Monday dinner → shopping list aggregates correctly → nutrition panel renders (once kcal_target is set). Completable without docs, modulo the UX concerns below.

### 2. OFF barcode (manual entry) — FAIL
Backend: `GET /api/ingredients/lookup?barcode=3017620422003` returns Nutella (539 kcal, 6.3 P, 30.9 F, 57.5 C). Works.
Frontend: typing the same barcode into the "Search by name or barcode…" field returns *No matches found*. The text input forces `?query=`, never `?barcode=`. Only the camera-scan button triggers a barcode call, which agent-browser cannot reach headless.

### 3. FDC name search — PASS (with minor nit)
`chicken breast` returns five USDA hits, inline macros, source badge. First-pick "Recommended" is a breaded item over raw — heuristic worth reviewing but not broken.

### 4. URL import JSON-LD — PARTIAL PASS
Real chefkoch URL returns 502 (outbound fetch blocked, likely Cloudflare). UI shows a localized "Could not load... try pasting the page source" with a visible fallback link. Graceful. The paste-HTML path (already covered by Playwright) presumably still works.

### 5. URL import LLM fallback — SKIPPED
Upstream fetch fails before LLM path engages.

### 6. AI agent — HARD FAIL (Bug #1)
First prompt to real OpenAI returns:
```
openai: 400 invalid_request_error: Invalid schema for function 'get_profile':
In context=(), object schema missing properties.
```
Root cause: `backend/internal/domain/agent/tools.go:336`
```go
schema := json.RawMessage(`{"type":"object","additionalProperties":false}`)
```
OpenAI's strict JSON-schema validation requires a `properties` field, even if empty. All other tools in `tools.go` define `properties`. Fix:
```go
schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
```
Entire AI feature is unusable with real OpenAI until this is fixed. Playwright missed it because it runs against the `fake` adapter.

### 7. Feedback → preferences → next chat — SKIPPED
Depends on (6).

### 8. Variants — PASS with UX concerns
`POST /api/components/{id}/variant` creates a sibling in the same variant_group_id. Confirmed via API. Editor opens with name pre-filled `"(variant)"` and an empty ingredients list. Swap picker treats siblings like any other component; no grouping or priority surface. PRODUCT's promise that swapping is "one click" is undermined if the sibling isn't visually close by.

### 9. Week copy + shopping — PASS
`POST /api/weeks/1/copy` → Week 17 populated with the same plate. Shopping list aggregates correctly against the planted plate.

### 10. Error edges — PASS
409 returned with correct message keys for `ingredient.in_use`, `component.in_use`, `slot.in_use`.

### 11. Empty + zero states — PASS with one cosmetic
Every primary route (`/`, `/components`, `/ingredients`, `/templates`, `/archive`) has a non-blank empty state. Archive shows the *current* week as an entry — PRODUCT describes archive as "past weeks," so the current-week row here is minor UX drift.

### 12. Visual polish — PASS
Dark theme active (browser preference); sage-green primary, amber accents, no raw blacks, focus rings on buttons. Matches "botanical, warm, calm."

---

## Concrete bugs (ranked by severity)

### Bug 1 — AI agent completely broken with real OpenAI [P0]
**File:** `backend/internal/domain/agent/tools.go:336`
**Fix:** add `"properties":{}` to the `get_profile` schema.
**Test gap:** all agent tests use the `fake` adapter. Add a unit test that each tool's `Schema` is valid under OpenAI's strict mode (at minimum: `type==object` requires `properties`).

### Bug 2 — Barcode entry in search field never triggers barcode lookup [P1]
`frontend/src/components/ingredients/LookupPanel.tsx:34` — `handleSearchChange` always clears `barcode`. Placeholder promises "by name or barcode" but numeric input routes to the name path, which never returns matches for a bare EAN.
**Fix options:** (a) remove "or barcode" from the placeholder, (b) detect pure-digit input (≥8 digits) and auto-switch to `?barcode=`, (c) add a distinct "Enter barcode" text affordance next to the scan button.

### Bug 3 — Nutrition panel silently hidden until profile target is set [P2, undocumented gate]
`Nutrition` button highlights but renders nothing when `user_profile.kcal_target` is null. After `PUT /api/profile` with a target, the panel renders correctly. No hint in the UI that the user needs to set a target first; first-hour users will think the button is broken.
**Fix:** either render a placeholder ("Set a daily target in Settings to see nutrition") or compute absolute macros even without a target.

### Bug 4 — Sodium unit mismatch [P2]
Form label says `Sodium (mg)`, but stored values are grams per 100 g (API) and displayed verbatim. A 0.0658 g/100 g ingredient shows "0.0658" under mg (should be 65.8 mg); the component detail shows "0.1 mg" on the same value. Either relabel as `Sodium (g/100g)` or multiply by 1000 at the UI boundary.

### Bug 5 — FDC ingredients with `kcal_100g: null` silently default to 0 [P2]
`boneless skinless raw chicken` returned from FDC has `kcal_100g: null`. The form pre-fills the `Calories (kcal)` field with `0` without any visual hint. Saving without noticing produces a zero-kcal ingredient that breaks nutrition math downstream.
**Fix:** compute `4P + 4C + 9F` as a server-side fallback, or render a "missing kcal — please fill" warning in the form.

### Bug 6 — Component detail ingredient list omits ingredient name [P2]
`/components/2` shows `100 g (100g)` as the ingredient line — just the amount twice. Ingredient name is missing entirely. Fix in the detail renderer — probably a missing join or field map.

---

## Retracted claims

During the walkthrough I initially reported that the Component Editor silently dropped ingredient-amount edits and additional ingredient rows. On re-test with cleaner event handling (Tab after fill, re-snapshotting refs after each DOM change), both work correctly:

```
POST /api/components {"name":"Multi-row test", ...,
  "ingredients":[
    {"ingredient_id":1, "amount":100, "grams":100, ...},
    {"ingredient_id":2, "amount":100, "grams":100, ...}]}
```

Root cause of the initial false positive: react-hook-form's `Controller`-wrapped fields need a synthetic change or blur event to commit into form state; `agent-browser fill` alone occasionally left the value uncommitted, and stale element refs after `useFieldArray` inserts caused the wrong button to be clicked. This is a **test-harness artifact, not an app bug.**

Worth noting for the regression suite: Playwright's `fill` + explicit `blur`/`Tab` is the correct pattern, and any agent-browser-style e2e addition needs the same discipline.

---

## Observations (not bugs)

- **Time-slot settings use developer-facing labels.** "Translation key: `slot.breakfast`", "Icon: `Coffee`". First-hour users will not guess these. Consider a selector of predefined slots + icons, with a "custom" escape hatch for power users.
- **Variant form does not pre-populate ingredients.** Variants share a group; users will most often want to clone-and-swap. Either offer a "Copy ingredients from parent" toggle, or clone by default and let the user remove.
- **Variant siblings not surfaced in swap.** PRODUCT sells swap as "one click"; siblings should lead the sorted list or appear in a dedicated "Variants" section.
- **Plate chip text aggressively truncated** ("Chi…") even with space available.
- **Archive lists the current week**, not just past weeks.
- **Plate feedback buttons render well** (✓ ✗ ❤ 👎 + save-as-template); the full PRODUCT §"I want the AI to learn what I like" loop could not be validated end-to-end because AI is down (Bug 1).
- **Double-POST on one Save click observed once** during the verification retest — the hooked fetch captured two identical `/api/components` POSTs. Only one record was written (the second likely 409'd on unique-name). Probably benign, but worth a look at the submit handler's idempotency.

---

## Coverage-gap recap (why agent-browser found what Playwright didn't)

| Gap | What Playwright does | What agent-browser exposed |
|---|---|---|
| LLM | uses `PLANTRY_AI_PROVIDER=fake` + scripted replies | real OpenAI schema validation rejects `get_profile` |
| Data seeding | every spec seeds via direct backend API POSTs | profile target never set before clicking Nutrition → undocumented empty panel |
| Barcode lookup | not tested with real OFF at all | typing a barcode in the text field silently fails |
| Empty states | fixtures seed before navigation | first-run UX validated |
| Real URL import | pastes HTML fixture | reveals outbound-fetch 502 behaviour (graceful) |

## Recommendations

1. **Add a schema-validity test per LLM tool.** Assert every tool's `Schema` is valid under OpenAI's strict mode (parseable; for `type: object`, `properties` key must exist). This single test would have caught Bug 1.
2. **Add one UI-driven golden-path Playwright spec** that runs against a fresh database without API seeding: slot → ingredient (via search) → component (with 2 ingredients, one via OFF, one via FDC) → plate → shopping totals match. Covers the flows that agent-browser exposed gaps in.
3. **Sweep `LookupPanel.tsx` + `IngredientEditor.tsx` for unit/label/null bugs together** — sodium label, kcal-null default, and barcode-in-search are neighbours.
4. **Decide the variant UX** (clone vs empty, sibling surface in swap). This is the current feature branch and behaviour is ambiguous against PRODUCT's "one-click swap" promise.
5. **Render an explicit gate for Nutrition panel** when no kcal target is set.

---

## State of the Docker container

Container still running at `localhost:8080` with test data (ingredient ids 1–2; components 1–4 incl. variant group 1; weeks 16–17 with plates; 1 AI conversation holding the failed message; profile now has a 2200 kcal target). Tear down with `docker compose down -v` when done.
