# Feature: Presets

A full replacement for the existing Templates feature. This document is the source of truth for **what the feature must do**. Implementation strategy is out of scope here and will be planned separately.

---

## 1. Summary

Replace Plantry's three-scoped Template system (slot / day / week) with a single, unified artifact called a **Preset**: a named, tagged, calendar-agnostic bundle of one or more plates, each bound to a slot type. Presets are surfaced through three apply pathways (command palette, draggable drawer, and contextual slot picker) and are fully editable in place. Week-shaped reuse is handled by a separate **Copy from week** flow that operates on planner history rather than on saved artifacts.

The old Templates implementation — schema, tools, endpoints, frontend routes, components, and AI tool — is removed entirely. No backwards-compatibility shims, no dual code paths.

---

## 2. Why we're doing this

### Problems with the current system

- **Three scopes (`slot` / `day` / `week`)** confuse the user. Scope is auto-inferred from selection range at save time; users don't realize it's a decision and can't change it later.
- **Slot-scope templates are second-class.** The `TemplatePicker` is hardcoded to `day` / `week` only. There is no top-level apply UI for slot-scope templates — the most natural primitive ("save my favorite dinners") is unusable in practice.
- **Templates are read-only after save.** Only rename and delete are supported. Drift in the underlying meal forces delete-and-rebuild.
- **The library is invisible from the planner.** Templates live on a separate `/templates` page; the planner doesn't suggest, preview, or list them inline.
- **Vocabulary is inconsistent.** API serializes entries as `components`, the DB calls them `entries`, the frontend type calls them `components`. Two scopes for one concept and three names for the same field.
- **Application is multi-modal.** A 5+ step modal flow (open dialog → pick date → pick template → resolve conflicts → submit) for what should be a drag or a keystroke.
- **`/templates/new` creates an empty template that can never be populated** from the UI — a dead route.
- **The AI assistant's `apply_template` tool is effectively unusable** — it accepts only slot-scope templates and has no companion `list_templates` / `get_template` tools, so the agent can never discover an ID to pass it.

### What the new system delivers

- One artifact, one mental model, honest vocabulary.
- A real cookbook: searchable, filterable, taggable, editable in place.
- Three apply surfaces matched to three user modes: keyboard-first (command palette), visual (drag from drawer), contextual (click empty slot).
- Past-week reuse handled by a dedicated history-based flow, decoupled from the artifact library.
- A coherent, well-shaped AI tool surface that the assistant can actually use.

---

## 3. Core concept

### What a Preset is

A **Preset** is a reusable, named bundle of one or more plates. It is:

- **Calendar-agnostic.** A preset has no notion of date, day-of-week, or day offset. It is not tied to a week shape.
- **Slot-type-bound.** Each plate in a preset is bound to a slot **type** (e.g. "Breakfast", "Lunch", "Dinner", or any user-defined slot type), not to a specific slot ID.
- **Composed of plates.** A preset contains 1..N plates. A 1-plate preset is the common case ("my chicken bowl"). A multi-plate preset is a bundle ("meal-prep Sunday", "training-day combo").
- **Composed of components.** Each plate in a preset contains 1..N components, where a component is `(food, amount, unit, optional note)`. Component semantics mirror the planner's plate components exactly.
- **Tagged.** Each preset has 0..N freeform string tags (e.g. `"quick"`, `"high-protein"`, `"vegan"`).
- **Editable.** All fields are mutable in place at any time. Edits do not affect previously-applied plates (no link, see §3.3).
- **Independent of the foods it references.** If a food is deleted, the FK prevents deletion (current behavior preserved) — the preset remains usable until edited.

### What a Preset is not

- It is not scoped (`slot` / `day` / `week` are retired terms).
- It does not carry calendar metadata (no `day_offset`, no anchor date).
- It is not linked to any plates it was created from or applied to.
- It is not versioned (no history of past states).
- It is not shared, exported, or published outside the single-user app.

### Plate ↔ Preset relationship after apply

Applying a preset is a **one-way copy**. The resulting plates on the planner are fully independent of the preset that created them. There is no link, no "revert to preset," and no "update preset from this plate." Plates can be freely edited after apply with no risk of touching the preset.

"Update preset from a current plate" is a deliberate, explicit action initiated from the preset editor (see §6.4), not an implicit consequence of editing a plate.

---

## 4. Conceptual data model

Implementation will translate this into SQL/sqlc. This is the conceptual shape, not the migration plan.

```
Preset
  id
  name (required, non-empty, trimmed)
  created_at
  updated_at
  last_used_at (nullable, updated whenever the preset is applied)

PresetPlate (1..N per Preset)
  id
  preset_id  -> Preset.id          (cascade delete)
  slot_type_id -> SlotType.id      (FK to existing slot definition)
  sort_order

PresetComponent (1..N per PresetPlate)
  id
  preset_plate_id -> PresetPlate.id (cascade delete)
  food_id -> Food.id                (RESTRICT delete; mirror current Templates behavior)
  amount, unit, note
  sort_order

PresetTag (0..N per Preset)
  preset_id -> Preset.id            (cascade delete)
  tag (string)
  PRIMARY KEY (preset_id, tag)
```

Notes:

- A `PresetPlate` with zero components is allowed during editing (intermediate state) but invalid for apply.
- Two `PresetPlate` rows in the same preset MAY share a `slot_type_id` (e.g. a preset with two breakfasts). Apply behavior with duplicates is specified in §6.5.
- Component amount/unit shape must match whatever the planner's current plate-component model uses. Presets must not introduce a parallel/legacy portion model. (The existing legacy `portions` float on `template_entries` is dropped — presets use the modern unit-aware shape from day one.)

---

## 5. Vocabulary

The UI uses **"Preset"** as the single user-facing term for the artifact.

- Library page: "Presets"
- Save action: "Save as preset"
- Apply action: "Apply preset"
- Cmd-K palette section: "Presets"
- Drawer: "Presets"

Internally, the field name for a preset's plate list is `plates` and for a plate's component list is `components`. The legacy `entries`/`components` API aliasing is eliminated — only `components` for component-level data, and `plates` for plate-level data.

The terms "template," "slot scope," "day scope," "week scope," "components" (when referring to plates), and "day offset" are retired from the codebase, the API, the UI, and the i18n strings.

---

## 6. Functional requirements

### 6.1 Creating a preset

Two entry points, both originating from real plates already on the planner. There is no empty-creation flow and no `/presets/new` route.

**6.1.1 From a single plate**

- Plate context menu / hover action: **"Save plate as preset"**.
- Opens a save dialog seeded with:
  - A suggested name derived from the plate's contents (e.g. dominant food name; same heuristic as today is acceptable).
  - Empty tags.
- The resulting preset has exactly one `PresetPlate`, bound to the source plate's slot type, with components copied verbatim (food, amount, unit, note, sort_order).

**6.1.2 From a multi-plate selection**

- The planner supports multi-selecting plates via lasso, shift-click, or an equivalent multi-select gesture. (If the planner does not currently support multi-select, this capability is part of the feature scope.)
- Selection may span multiple days and multiple slot types.
- Toolbar / context action: **"Save selection as preset"**.
- Opens the same save dialog, with the name seeded from the selection summary (e.g. "Bundle of 3").
- The resulting preset has one `PresetPlate` per selected plate, slot-type-bound, components copied verbatim.

**6.1.3 Save dialog requirements**

- Name field (required, trimmed, non-empty).
- Tag input with autocomplete from existing tags (see §6.3).
- Preview showing slot types and food badges so the user sees what's being saved.
- Cancel and Save buttons. Save is disabled when name is empty.
- Save success: toast `"Preset \"{name}\" saved."`; the user remains in the planner (no navigation).

**6.1.4 Disallowed flows**

- **No "create empty preset from scratch."** The library and routes do not expose any "+ New preset" affordance that opens an empty editor. Presets are born from real plates only. (The preset editor in §6.4 is reachable only by opening an existing preset.)
- **No auto-promotion.** The system does not silently track frequently-used plates and propose them as presets. The user is the only author.

### 6.2 Library page (`/presets`)

Replaces `/templates`. Optimized for 30+ entries.

**Layout requirements:**

- **Search bar** (top, sticky): free-text filter matching preset name (case-insensitive substring).
- **Slot-type filter chips**: one per active slot type (Breakfast, Lunch, Dinner, plus any user-defined slot types). Toggling chips filters to presets that contain at least one plate of that type. Multiple chips selectable (OR semantics).
- **Tag chips**: shows all distinct tags used across the user's presets, sortable by frequency. Toggling filters to presets carrying that tag (AND semantics across multiple tags).
- **Result grid**: card layout, responsive.
- **Empty state**: when no presets exist, an empty illustration + message explaining that presets are saved from real plates on the planner (with a link/button that focuses the planner).
- **No-result state** (filters yield nothing): a "clear filters" affordance.

**Each preset card shows:**

- Preset name
- Food badges (up to 4 distinct foods across all plates)
- Slot-type chips (which types are covered)
- Plate count badge (`"1 plate"` / `"3 plates"`)
- Tag chips
- Last-used timestamp (relative, e.g. "Used 2 days ago"; absent if never used)
- Click target: opens the **Preset Editor** drawer (§6.4)
- Card menu: Rename, Duplicate, Delete

**Card actions:**

- **Rename**: inline edit on the card or a small dialog.
- **Duplicate**: copies the preset with a name suffixed (e.g. "Chicken Bowl (copy)"); new preset gets a fresh `id`, `created_at`, no `last_used_at`.
- **Delete**: confirmation dialog ("Delete preset 'X'? This cannot be undone."). Cascades to plates, components, and tags. Does not affect any plates previously applied to the planner.

### 6.3 Tags

- Tags are **freeform strings** per preset, normalized to lowercase on write, trimmed, deduplicated within a preset.
- Tag input shows autocomplete from `SELECT DISTINCT tag` across all presets, ranked by usage frequency.
- No central tag management screen, no tag rename, no tag colors, no predefined tag set.
- The library page and Cmd-K palette responses always include the top-N existing tags so users (and the AI assistant) prefer reuse over coining duplicates.

### 6.4 Preset editor

Opens as a drawer/panel from the library page when a preset card is clicked.

**Editable fields:**

- Name (rename in place)
- Tags (add / remove via chip input)
- Plates list: add a plate, remove a plate, change a plate's slot type, reorder plates
- Per plate: components list — swap a food, change amount/unit/note, add a component, remove a component, reorder components

**Behavior:**

- **Autosave on blur.** No explicit save button. A small saved/saving indicator confirms persistence.
- The editor uses the same component-input UI primitives as the planner so the user's interaction model is shared.
- The editor surfaces no concept of date, day-of-week, day offset, or calendar — only slot types.
- **Update preset from plate** action (entry from preset editor): a "Pull plate from planner…" button opens a target picker (date + slot type), reads that plate's components, and offers to replace a preset plate's components with them. Optional in v1 — not strictly required but called out so implementation knows where it would live.

### 6.5 Applying a preset

Apply is a one-way copy that materializes the preset's plates onto the planner. Three surfaces — all use the same underlying apply behavior.

**6.5.1 Apply semantics**

Given:
- `preset` with 1..N plates, each bound to a slot type
- `target_date` (calendar date) for the apply
- `on_conflict` ∈ `{skip, overwrite}` (default `skip`)
- Optional `slot_types_filter` (apply only plates whose slot type is in this list)

For each plate in the preset (filtered by `slot_types_filter` if provided), compute the target slot on `target_date`:

1. **Slot resolution**: find the slot on `target_date` whose `slot_type_id` matches the preset plate's `slot_type_id`. If multiple slots share that type, the one with the **lowest `sort_order` on that date** wins (deterministic, no prompt).
2. **No matching slot**: the preset plate is silently skipped from materialization; it appears in the apply result's `skipped_no_slot` list.
3. **Target slot empty**: a new plate is created, components copied verbatim.
4. **Target slot occupied + `on_conflict=skip`**: the preset plate is skipped; appears in `skipped_occupied`.
5. **Target slot occupied + `on_conflict=overwrite`**: the existing plate is replaced with the preset plate's components. The pre-replace plate state is captured for undo (server- or client-side snapshot — implementation chooses).

After a successful apply, the preset's `last_used_at` is updated.

**6.5.2 Apply result shape**

The apply operation returns:
- `created`: list of `{plate_id, date, slot_id, slot_type_id}` for new plates
- `replaced`: list of `{plate_id, date, slot_id, slot_type_id}` for overwritten plates (only when `on_conflict=overwrite`)
- `skipped_occupied`: list of `{date, slot_type_id}` for plates that didn't land because the slot was filled
- `skipped_no_slot`: list of `{slot_type_id}` for preset plates whose slot type doesn't exist on the target date

All three surfaces consume this result to produce user-facing toasts.

**6.5.3 Apply surface: Command palette (Cmd-K)**

Global hotkey (`Cmd+K` / `Ctrl+K`) opens a unified command palette with three sections:

- **Presets** — ranked by recency × frequency; filterable by typing. Selecting a preset reveals a target picker (date + optional slot-type filter), then submits.
- **Recent plates** — last 30 distinct applied plates from planner history (dedup by component signature), each acting as an ad-hoc "apply this composition again." Selecting one reveals a target picker.
- **Actions** — at minimum: "Copy from week…", "Go to Presets library." (Other navigation/actions optional.)

Apply behavior:
- `Enter` = apply with `on_conflict=skip` (default).
- `Shift+Enter` = apply with `on_conflict=overwrite`.
- Result toast describes what landed and what didn't; overwrite toast includes an Undo affordance.

**6.5.4 Apply surface: Drawer**

A toggleable right-rail drawer in the planner:

- Hidden by default. Opened by hotkey (e.g. `t`) and/or a planner toolbar button.
- Contents: the preset library (same search / slot-type / tag filters as `/presets`), in a compact list form.
- **Drag-and-drop**: drag a preset card onto a slot, a day column, or a week to apply.
  - Drop on a **slot**: applies the preset filtered to that slot's type only (single-plate apply).
  - Drop on a **day**: applies the preset to that day (all plates, each to its matching slot type).
  - Drop on a **week** (e.g. week header): applies the preset to the week start date (only meaningful if presets had day offsets — which they don't — so this case applies to the first day of the week only).
- **Shift-drop** = `on_conflict=overwrite`. Plain drop = `on_conflict=skip`.
- Result toast shape identical to Cmd-K.

**6.5.5 Apply surface: Empty slot contextual picker**

Clicking an empty slot on the planner opens a picker pre-filtered to that slot's type:

- Shows recents first (presets with that slot type most recently applied), then all matching presets.
- Search bar inside the picker for the same filtering as the drawer.
- Clicking a preset applies it directly to the clicked slot (single-plate apply with `slot_types_filter` set to that slot's type).
- Apply uses `on_conflict=skip` by definition (the slot was empty).

**Out of scope:** inline auto-suggestion chips inside empty slots ("your top 3 breakfasts") are explicitly **not** part of v1 — only the click-to-open picker.

### 6.6 Conflict resolution rules summary

| Surface | Default | Overwrite gesture |
|---|---|---|
| Cmd-K | skip | Shift+Enter |
| Drawer | skip | Shift+drop |
| Slot picker | skip (slot is empty by definition) | n/a |
| Past-week copy (§6.7) | skip | Toggle in confirmation step |
| AI agent tool | skip (explicit parameter, default `skip`) | `on_conflict: "overwrite"` |

In all surfaces, overwrite operations:
- Show a confirmation visual cue before commit where possible (e.g. drop preview highlights replaced plates in a warning color).
- Always produce a toast with an Undo affordance.
- The Undo restores the pre-apply state of all replaced plates.

### 6.7 Past-week copy

A **separate flow** from presets. The user's planner history is the source — no artifact is created or required.

**Entry point:** A button on the week toolbar: **"Copy from week…"**

**Flow:**

1. User clicks "Copy from week…" while viewing a target week.
2. A date picker (or week picker) opens, defaulting to the previous week. Recently-active weeks may be surfaced as suggestions (optional).
3. After selecting a source week, a preview shows which plates will be copied (foods, slot types, dates).
4. The preview includes an `on_conflict` toggle (skip / overwrite), defaulting to skip.
5. **Confirm** triggers the copy: every plate in the source week's date range is materialized onto the corresponding day of the target week (Mon→Mon, Tue→Tue, etc.).
6. Conflict semantics match preset apply: skip occupied or overwrite, with the same result toast shape (`created` / `replaced` / `skipped_occupied`).
7. Slot collision rules match preset apply: lowest sort_order wins on duplicate slot types; missing slot types skipped silently.

**Important:** past-week copy does **not** produce or consume a preset. It is purely a planner-to-planner duplication.

### 6.8 AI assistant integration

The preset feature must be fully usable by the existing AI agent. The agent's tool surface is updated to match.

**Tools to add:**

- `list_presets(search?, slot_type?, tag?, limit?, offset?)` — returns id, name, plate count, slot types, tags, last_used_at. Response also includes a `known_tags` array (top tags by frequency) to discourage tag duplication.
- `get_preset(preset_id)` — full preset detail including plates and components.
- `create_preset_from_plates(name, plate_ids[], tags?[])` — single shape covering both single-plate and multi-plate creation. The UI builds the right `plate_ids`.
- `update_preset(preset_id, name?, add_tags?[], remove_tags?[])` — top-level edits. (Plate/component edits inside a preset are deferred; see §10.)
- `delete_preset(preset_id)` — cascade delete.
- `apply_preset(preset_id, target_date, on_conflict?, slot_types_filter?[])` — replaces `apply_template`. Default `on_conflict=skip`. Returns the full result shape from §6.5.2.
- `copy_week(source_start, target_start, on_conflict?)` — exposes the past-week-copy flow to the agent. Returns the same result shape as apply.

**Tools to remove:**

- `apply_template` — gone with the rest of the Templates feature.

**System prompt additions:**

Add a Presets section to the agent's system prompt (`backend/internal/domain/agent/prompt.go`) covering:

- "Presets are reusable, named bundles of one or more plates. Each plate is bound to a slot type (breakfast/lunch/dinner)."
- "When the user references 'my X' or 'the X preset', call `list_presets` with `search=X` first. Never invent preset IDs."
- "Default `on_conflict` is `skip`. Use `overwrite` only when the user explicitly asks to replace, rebuild, overwrite, or wipe."
- "Prefer existing tags returned in `known_tags` over coining near-duplicates."
- "For 'apply my standard week,' use `copy_week` from a representative past week, or ask the user which week to use."
- "If a preset has multiple plates and the user's request is slot-specific, use `slot_types_filter` to apply only the relevant plate."

**Tool result behavior:**

- All apply tool results explicitly enumerate `created`, `replaced`, `skipped_occupied`, `skipped_no_slot`. The agent must surface skips to the user when non-empty.
- `list_presets` response is paginated and includes total count so the agent can avoid asking for more when the user is clearly referencing a single preset by name.

---

## 7. Non-functional requirements

- **No CGO** in any new backend code (cross-compile to ARM constraint).
- **Single binary, single SQLite file** — no new data stores.
- **Hexagonal-Lite architecture** preserved: `domain/preset/` for aggregates + service + repo interface; `adapters/sqlite/` for queries; `transport/http/handlers/presets.go` for thin translation.
- **sqlc-generated functions** for all DB access (no raw SQL outside `queries/`).
- **Domain validation in the service**, not the DB. DB constraints are a safety net only.
- **FTS5 search**: if preset name search needs FTS5, user input must pass through the existing `sanitizeFTS5()` helper.
- **`errors.Is()`** for sentinel errors, never `==`.
- **Tests cover happy path + edge cases** at every layer (unit, adapter, HTTP) per CLAUDE.md coverage requirements. Validation, conflicts, duplicate names, FK behavior on food delete, unicode, FTS operators, malformed JSON, non-numeric IDs.
- **e2e tests** cover at minimum: save from plate, save from selection, apply via picker, apply via drawer drag, apply via Cmd-K, overwrite with undo, past-week copy.
- **Frontend test fixtures and `renderWithRouter`** reused; no per-file mock duplication.
- **i18n**: every user-facing string in both `en.json` and `de.json` (matching the existing template.* coverage pattern, in a new `preset.*` namespace).
- **Self-hosted, single-user, LAN-only context** — no auth, no sharing, no multi-tenant concerns.

---

## 8. What is being removed

This is a full replacement. The following must be gone after the feature lands:

**Backend:**

- Schema: `templates`, `template_entries` tables, `scope` column, `day_offset` column, legacy `portions` float on template entries.
- Migrations 00006, 00015, 00017 (template-related) supersedes by a migration that drops the tables. (Implementation may choose to preserve them as historical migrations and add a drop migration on top.)
- Domain: `backend/internal/domain/template/` (entire package).
- Adapters: all template-related sqlc queries and generated code (`queries/templates*.sql`, generated equivalents).
- Transport: `backend/internal/transport/http/handlers/templates.go`, route registrations.
- Agent tool: `apply_template` and its registration.
- System prompt: any template-related rules.

**Frontend:**

- Routes: `frontend/src/routes/templates/index.tsx`, `frontend/src/routes/templates/new.tsx`, and any other `templates/*` routes.
- Components: `SaveAsTemplateDialog`, `TemplatePicker`, `TemplateList`, `TemplateForm`, and any other `frontend/src/components/templates/*`.
- API client: `frontend/src/lib/api/templates.ts`.
- Queries: `frontend/src/lib/queries/templates.ts` and any related hooks.
- Schemas: `frontend/src/lib/schemas/template.ts`.
- i18n: every `template.*` key in `en.json` and `de.json`.

**Tests:**

- All template-specific Go tests (`*_test.go` in the template package and template handler tests).
- All template-related Vitest specs.
- All template-related Playwright specs.

**Migration considerations:**

The user has explicitly chosen "burn it down." Existing template data in user databases is **not migrated**. The implementation must:

- Document this clearly in any release notes / changelog.
- Provide a one-shot migration that drops the template tables cleanly.
- Surface no template UI, no template API endpoints, and no template-derived behavior anywhere in the running system after release.

(If a future need for one-time data preservation arises, that is a separate task. v1 does not migrate.)

---

## 9. Out of scope (v1)

These were considered and explicitly deferred:

- **Portion scaling at apply time** (`×2` / `×0.5` dial). User chose "apply 1:1, edit afterwards." No scale parameter on apply.
- **Plate ↔ preset link** (loose or strong). Apply is one-way copy. No origin tracking, no "revert to preset," no sync.
- **Auto-promotion** of frequently-used plates into presets.
- **Inline auto-suggestion chips** inside empty slot cells. Empty slots open a picker on click; they do not proactively display preset chips.
- **Build-from-scratch preset editor.** Presets are born from real plates only.
- **Variants / portion-size duplicates.** If the user wants a "double" version, they save a second preset.
- **Tag rename / merge / delete** as a central operation. Tags are freeform; removing the last reference effectively removes the tag.
- **Tag colors, predefined tag set, tag groups.**
- **Preset sharing, export, import.**
- **Preset versioning / history.**
- **Preset-internal edit tools for the AI** (swap food inside a preset via agent). The agent can read presets with `get_preset` and create new presets via `create_preset_from_plates`. In-place preset editing through the agent is deferred. The user can always edit via the UI; if real prompts demand it, add later.
- **"Update preset from plate"** action in the editor (§6.4) is **flagged as optional** for v1 — implementation may include or defer it.
- **Named / labeled past weeks** ("my standard week"). The past-week-copy flow operates on raw calendar dates only. If "apply my standard week" becomes a top prompt, revisit (see §10).

---

## 10. Open questions and future considerations

These are not blocking v1 but should be re-evaluated after the feature lands and real usage data exists.

- **Routine-week reuse.** v1 expects the user to keep a representative past week clean and use Copy from week. If "apply my usual routine" becomes a recurring top-level need, candidates include:
  - Reintroducing an optional `day_offset` on `PresetPlate` (the artifact gains a multi-day shape).
  - A lightweight "starred weeks" concept (label a past week so it's findable / namable).
- **Tag fragmentation.** Even with autocomplete and `known_tags` in agent responses, tag duplicates may accumulate ("quick" / "Quick" / "quickly"). Mitigation if observed: lowercase-normalize on write (already specified), add a tag merge tool in a maintenance UI.
- **Preset-internal edit tools for the AI.** Hold until real prompts demand it.
- **Inline empty-slot suggestions.** Reconsidered if the click-to-open picker proves heavier than expected for the common "fill an empty breakfast" case.
- **Tool count.** Adding ~6 preset tools brings the agent toolset to ~19. Watch for tool-selection confusion in observed agent behavior; consolidate via richer single tools if needed.

---

## 11. Acceptance criteria

The feature is complete when:

1. **No template code exists** in the codebase (backend, frontend, i18n, migrations except a drop migration, tests). A grep for "template" returns only incidental matches (string templates, Go text/template stdlib, build templates).
2. A user can save **a single plate as a preset** from the planner via plate context menu.
3. A user can save **multiple selected plates as a preset** from the planner via multi-select.
4. A user can browse the **`/presets` library page** with search, slot-type filter chips, and tag filter chips, all working on a database with 30+ presets.
5. A user can open a preset in the **editor**, rename it, add/remove tags, add/remove plates, swap/add/remove components, change amounts, change a plate's slot type — all autosaving on blur.
6. A user can **apply a preset** via:
   - Cmd-K palette (with `Shift+Enter` for overwrite)
   - Toggleable drawer (drag, with `Shift+drop` for overwrite)
   - Empty-slot contextual picker
7. **Apply respects slot-type collision rules** (§6.5.1): lowest sort_order wins on duplicates, missing slot types skip silently with a toast note.
8. **Overwrite is always undo-able** via the result toast.
9. A user can **copy from a past week** via the week toolbar, with the same skip/overwrite semantics.
10. The **AI assistant** can list, get, create, update, delete, and apply presets, and copy weeks, via the new tools. The system prompt instructs it to default to `skip` and prefer existing tags.
11. **All existing test coverage requirements** (validation edges, conflicts, FK behavior, FTS sanitization, HTTP malformed input, e2e flake-free `--repeat-each=10 --workers=4`) are met for the new feature.
12. **i18n parity** — every new user-facing string exists in both `en.json` and `de.json`.
13. **The feature works on the deployment target** (ARM cross-compile, single binary, single SQLite file, no CGO).

---

## 12. Glossary

- **Preset**: the new artifact. Named, tagged bundle of 1..N plates, each slot-type-bound. Calendar-agnostic.
- **PresetPlate**: a single plate inside a preset, bound to a slot type, containing components.
- **PresetComponent**: a single food entry inside a PresetPlate (food, amount, unit, note).
- **Slot type**: an existing concept in the planner (e.g. Breakfast). The preset binds to the type, not to a specific dated slot.
- **Apply**: materializing a preset's plates onto the planner at a target date. One-way copy.
- **Skip-by-default**: the conflict resolution rule where occupied target slots are left untouched.
- **Overwrite-on-modifier**: the opt-in destructive variant, gated by Shift in UI surfaces and by `on_conflict="overwrite"` in the agent tool.
- **Past-week copy**: a separate, history-driven flow for week-shaped reuse. Not a preset.
- **Recent plates** (Cmd-K section): the user's last-applied plates surfaced as ad-hoc apply sources alongside saved presets.
