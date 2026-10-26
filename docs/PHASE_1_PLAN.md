# Phase 1 implementation plan: Preferences and availability

Status: ready to execute. Written 2026-10-26 against `main` at 45171ed (Phase 0 complete, PRs #19–#24).

Sources of truth:
- `docs/PLANNING_ASSISTANT_SPEC.md`: §14 Phase 1, plus §3.1, §3.2, §3.5, §4, §5, §7.1, §7.2, §8.1, §8.2, §11, §12, §13, §16 and §17.
- `docs/UX_DESIGN_SPEC.md`: §2, §3, §5.1 (setup wizard), §5.8 (Recipes, guided state), §5.11 (Settings), §6–§9 and §11.
- `docs/PHASE_0_PLAN.md` §5 ("Out of Phase 0") and the Phase 0 follow-ups carried over from the Phase 0 reviews.
- `DESIGN.md` for the visual system.

All file and line references were checked against the code on 2026-10-26. Where the specs cite stale locations, this plan gives the current one.

---

## 0. Definition of done

1. **Fresh install flow.** On an empty database, `/` redirects to `/setup`. Clicking Next through every step and then Finish yields a working configuration: three meal slots, the maintain nutrition preset, every §4 default, and an all-`plan` availability grid. Finish lands on the Recipes page in its guided state (0 / 10). Once 10 recipes exist, the guided state collapses, "Generate week" is enabled, and the planner opens normally.
2. **"No plate needed" is a cell state.**
   - `plates.skipped`, `POST /api/plates/{id}/skip`, `planner-skip.ts` and every "skipped"/"Eating out" UI are gone. `grep -rn "skipped" backend/internal frontend/src` finds only unrelated uses (preset-apply `skipped_occupied`/`skipped_no_slot`, importer, etc.).
   - Existing skipped plates are migrated to `none` overrides.
   - The planner renders `none` and `routine` cells from `GET /api/availability`.
   - No plate can be created in or moved into a `none` cell.
3. **One plate per cell.** Duplicate plates per (date, slot) are merged by the migration, a unique index enforces it, and the API answers a second plate with 409.
4. **Preferences are editable.** Every Phase 1 profile field and setting (D4) can be edited in Settings, which uses the UX §5.11 sections, including the default availability grid. The wizard can be re-run from Settings → Data & system.
5. **AI calls can be replayed.** Tests show that a `lookup` run and an `import_fallback` run can be fully reconstructed from the database (system prompt, messages, parameters, tool schemas and responses). `ai_runs` carries `conversation_id` and `request_id`. Step bodies older than the retention setting are pruned.
6. **Phase 0 follow-ups** are fixed in the step that rebuilds the affected screen, or listed in §5 with a reason.
7. These all pass:
   - `cd backend && go test -race ./... && go vet ./... && golangci-lint run`
   - `cd frontend && bun run check && bun run build && bun run e2e`

---

## 1. Decisions taken for Phase 1

The user may override any of these.

| # | Decision | Reason |
|---|---|---|
| D1 | **No route renames.** The guided state lives on `/components` and the planner stays at `/`. `/recipes`, `/today` and `/plan` arrive with the rebuilt screens in Phases 2–3 (as in Phase 0 D1). Nav is already restructured (Phase 0 D1a). | Keeps Phase 1 about preferences and availability. |
| D2 | **Setup redirect only from `/`.** `routes/index.tsx` redirects to `/setup` while `setup.completed` is false (spec §7.1). Deep links keep working. | Spec wording; avoids trapping deep links. |
| D3 | **Existing installs skip the wizard.** Migration 00023 writes `setup.completed = true` when `time_slots` already has rows. Fresh databases start with `false`. | Upgrading users already have a working setup. They can re-run the wizard from Settings. |
| D4 | **Phase 1 settings keys.** New: `setup.completed` (bool, false), `plan.max_cook_occasions_per_day` (int 1–3, 1), `plan.batch_cooking` (bool, true), `plan.default_keep_days` (int 1–7, 3), `plan.freezing_enabled` (bool, false), `plan.freezer_keep_days` (int 1–365, 90), `plan.variety_weeks` (int 0–8, 3), `plan.explore_per_week` (int 0–3, 0), `discovery.daily_enabled` (bool, true), `ai.run_retention_days` (int 7–3650, 180). Changed: the `plan.anchor` default becomes `next_shopping_day`. Removed: `plan.window_days` (registry entry, key constant and DB row). **Deferred:** `ai.price_overrides` (Phase 3, as Phase 0 D3) and `discovery.cuisines` (Phase 5; it needs a list kind and has no consumer yet). | Spec §4.2, minus keys that have no consumer in Phase 1. |
| D5 | **Profile fields.** `diet_type` TEXT NOT NULL DEFAULT `'omnivore'` CHECK in (vegan, vegetarian, pescetarian, omnivore). `meat_days_per_week_max` INTEGER NULL CHECK 0–7. `household_portions` REAL NOT NULL DEFAULT 1.0 CHECK > 0 (the service caps it at 20). `PUT /api/profile` stays a full replace; a missing `diet_type` or `household_portions` in the body keeps the stored value rather than resetting it, so older clients don't wipe it. | Spec §4.1. |
| D6 | **Nutrition goal presets** (wizard step 3, settings): cut = 1800 kcal at 35/30/35 (P/F/C); maintain = 2200 at 30/30/40; bulk = 2700 at 25/25/50; custom = free entry. The presets live in the frontend only, and the profile stores just the numbers. The DB defaults stay NULL; the wizard writes maintain. | The spec names maintain only. These are common reference splits. |
| D7 | **Availability shape.** As spec §5.1, plus `note TEXT NULL` on `availability_overrides` (UX §5.5: "No plate" shows its reason, and migrated skipped plates carry their note). `preset_id` uses `ON DELETE CASCADE` on both tables, so deleting a routine preset drops its rows and the cell falls back to the next level (override → default → `plan`). CHECK: `preset_id IS NOT NULL` iff `mode = 'routine'`. Weekday 0 = Monday. Only active slots are resolved. | A routine without a preset is meaningless. Cascading keeps resolution simple. |
| D8 | **Routine in Phase 1 is a label.** A `routine` cell shows "Routine · \<preset name\>" in the planner and grids. Applying the preset to the cell is the generator's job (Phase 3). The user can still add a plate by hand in a routine cell. | The generator owns routine application (§3.2). |
| D9 | **Marking a planned cell "No plate".** `PUT /api/availability/{date}/{slotId}` with `mode: none` on a cell that has a plate returns 409 `error.availability.cell_has_plate`, unless the body has `remove_plate: true`. In that case the override is written and the plate deleted in one transaction, and the response carries `removed_plate` (the full plate JSON). The planner always sends `remove_plate: true` and offers a 6 s Undo that restores the previous override state and recreates the plate from `removed_plate` (the same copy path as the existing ctrl-drag copy). | UX §6: undo over confirm. The old skip silently cleared components. |
| D10 | **Cell rules in the plate service.** Creating a plate, or moving one (`PUT /api/plates/{id}` with a new date or slot), into a cell that resolves to `none` returns 409 `error.plate.cell_unavailable`. Into an occupied cell it returns 409 `error.plate.cell_occupied`. The planner rejects such drops client-side with a toast before calling the API, extending the existing `reject_skipped` path at `PlannerGrid.tsx:634-637`. There is no swap. | H1 and UNIQUE (date, slot) as one rule set. Swap would need a two-step update against the unique index. |
| D11 | **Skipped-plate migration** (spec §11), in order, in one Go migration: (1) each skipped plate becomes a `none` override for its cell, carrying the plate's `note`, then the plate is deleted. If the same cell also holds a non-skipped plate with components, that plate wins: no override is created and the skipped plate is just deleted. (2) Duplicate plates per (date, slot) merge into the lowest id: components are re-parented with `sort_order` appended after the survivor's, the survivor keeps its own feedback, and the others are deleted. (3) `ALTER TABLE plates DROP COLUMN skipped` and `CREATE UNIQUE INDEX ux_plates_date_slot ON plates(date, slot_id)`. No table rebuild is needed: the column has only its own CHECK. | Spec §11. Dropping a column plus a unique index avoids rebuilding a table with FK children. |
| D12 | **Copy week and preset apply respect availability.** Cells resolving to `none` in the target get no plate and are reported in a new `skipped_unavailable` list, next to `skipped_occupied`. Copy week also copies the source window's `none` overrides onto the matching target dates, so "copy last week" keeps last week's No-plate cells (the bug noted in spec §2.2, where skipped plates were dropped). Routine overrides are not copied. | §11: "the HTTP copy-week endpoint … stay[s] for manual use, now preserving availability". |
| D13 | **Agent interim.** The chat prompt lists the window's `none` and `routine` cells instead of skipped plates. The plate tools drop `skipped`. `get_plates_range` gains an `availability` array of `{date, slot_id, mode, preset_name?}` for non-`plan` cells. The fill-mode hints say "never create plates in No-plate cells" instead of "respect skipped". `Shopping day:` prints the weekday name. Agent `PromptVersion` becomes `"v3"`. | The chat stays usable until Phase 4. The plate service enforces D10 anyway. |
| D14 | **Setup API.** `GET /api/setup` → `{completed, recipe_count, recipe_target}`, where `recipe_count` is the number of composed foods and `recipe_target` is 10. `POST /api/setup/complete` sets `setup.completed = true` and returns the same shape. The spec's `steps_done` is dropped: the wizard saves each step on Next and always reopens at step 1, prefilled from the current values. | One request serves the redirect, the guided state and the Generate gate. Resumable steps add nothing a prefilled wizard doesn't give. |
| D15 | **Wizard slots.** Step 4 shows the existing active slots. On an install with none, it shows Breakfast, Lunch and Dinner (`slot.breakfast`/`slot.lunch`/`slot.dinner`, icons as in `TimeSlotsEditor`) and creates them on Next through the existing slots API. The slots can be renamed, reordered and toggled in the step. The availability grid in the same step uses the resulting slots. | Spec §7.1 step 4: "pre-created, editable". |
| D16 | **"Skip, use defaults"** (from step 2 on) saves the current step's shown values, writes the defaults of every later step (step 4 still creates the default slots when there are none), and goes to Finish. | UX §5.1: Next, Next, Finish must give a working setup, and skipping must too. |
| D17 | **Wizard AI step.** It reuses the existing AI settings controls (provider, model, key) and `GET /api/settings/ai/models` for inline validation. Real Anthropic key validation (that endpoint returns the static list for any non-empty key, `handlers/settings.go:173-182`) moves to Phase 5 together with capability detection (§9.4). The copy therefore says "Key saved. N models available" for Anthropic and "Key works, N models available" for OpenAI. | Avoids claiming a check that isn't made. |
| D18 | **Guided state.** It shows while `recipe_count < 10`. Cards: **Import from a link** (→ `/import`) and **Create manually** (→ `/components/new`, with a line about variants). The **Discover** card is not shown in Phase 1, because discovery is Phase 5. The milestone toast "10 recipes. You're ready to plan your first week." fires once, when the page sees the count reach 10 after having shown the guided state (`localStorage` key `plantry.recipeGuideShown`, read and written inside try/catch). | UX §5.8. Without the flag, an upgrading user with 50 recipes would get the toast. |
| D19 | **Generate gate.** `GeneratePlanButton` stays visible and is disabled while `recipe_count < 10`, with a tooltip and a link "Add N more recipes" to `/components`. It keeps opening the chat with the generate prompt; navigating to `/plan` is Phase 3. The phone centre FAB is removed (UX §2.1: the tab bar has exactly four items), and on phones the button moves to the TopBar next to the Settings gear. | Spec §7.2 and the Phase 0 follow-up. |
| D20 | **Settings restructure** (UX §5.11). The route becomes `/settings/$section` with sections `household`, `meals`, `cooking`, `assistant` and `system`. `/settings` redirects to `/settings/household`, and the legacy `?tab=` values map onto the sections (general/nutrition→household, plan→cooking, meal_slots→meals, ai→assistant, system→system). Theme and language (today's General tab) go to Data & system. Existing panels (`ProfileEditor`, `TimeSlotsEditor`, `AITab`, `PlanTab`, `SystemTab`) are moved and extended, not rewritten. Presets keep their own page in Phase 1; "Routines" in Settings → Meals is a link to `/presets`. | UX §5.11. Moving keeps the diff reviewable. |
| D21 | **Planner window.** With `plan.anchor` defaulting to `next_shopping_day`, the e2e setup project sets `plan.anchor = today`, so the existing planner specs (which seed plates on today's dates) stay valid. `planner-anchor-settings.spec.ts` keeps covering the other modes. | Avoids rewriting about 15 planner specs that Phase 2/3 replaces anyway. |
| D22 | **E2E global state.** A new Playwright project `setup` runs `setup-wizard.spec.ts` serially against the fresh e2e DB (no slots, so `setup.completed` is false), walks the wizard to Finish, then sets `plan.anchor = today`. `chromium` and `ai-serial` declare `dependencies: ["setup"]`, so a filtered run still completes setup first. Specs that need a "below 10 recipes" state stub `GET /api/setup` with `page.route`, because the shared e2e DB accumulates recipes. | Setup state is global, and parallel specs would otherwise be redirected. |
| D23 | **AI trace storage.** Each `llm_call` step stores `request_json` and `response_json` in new columns, separate from the 8 KB `args_json`/`result_json` used by tool calls.<br>**`request_json`** holds `{message_offset, messages, temperature, max_tokens, tools_hash, truncated}`:<br>- `messages` holds only the messages not already recorded by an earlier `llm_call` of the same run (`req.Messages[offset:]`). The first call, or any call whose history is shorter than what has already been recorded, records from offset 0.<br>- `tools_hash` is the sha256 of the canonical JSON of `req.Tools`, stored once in a new `ai_tool_schemas(hash, json)` table, the same pattern as `ai_prompts`.<br>**`response_json`** holds `{content: [blocks], stop_reason}`.<br>Each body is capped at 256 KiB. A cut sets `truncated: true` and stays valid JSON: the over-long text is shortened, never the JSON syntax. | Linear storage for multi-iteration chats. The import fallback sends up to 60 000 chars (`importer/llm_prompt.go:21`), so a lookup or import run fits uncapped. |
| D24 | **Run identity and retention.** `ai_runs` gains `conversation_id INTEGER NULL` (set by chat runs; no FK, so a run outlives its conversation) and `request_id TEXT NULL` (from chi `middleware.GetReqID`, already captured in `Run.requestID`, `airun/recorder.go:74`). At startup, and after each finished run at most once per hour, steps of runs that started more than `ai.run_retention_days` ago are deleted. The `ai_runs` rows and prompts stay (spec §13). | Spec §13 storage rules, with no background scheduler (§15). |
| D25 | **Outcome labels and the plate audit trail stay out.** `ai_runs.outcome*` remains reserved for Phase 3, and `plate_events` (§8.2) arrives in Phase 2. | User instruction. |
| D26 | **Impeccable without the helper binary** (Phase 0 D9 still applies). Never run `.claude/skills/impeccable/scripts/impeccable`. Read `docs/PRODUCT.md`, `DESIGN.md` and the skill's `reference/*.md` directly. | User instruction. |

---

## 2. Repository facts the steps rely on

- **Migrations** are in `backend/db/migrations`; the latest is `00022_food_source_description.sql`.
  - The numbers are fixed: **00023 = P1**, **00024 = A1**, **00025 = A2** (Go), **00026 = T1**.
  - sqlc reads `db/migrations` and `db/schema_patches` (`backend/sqlc.yaml`). A Go migration needs a matching `db/schema_patches/000NN_*.sql` so sqlc sees the schema change; see `schema_patches/00016_drop_week_aggregate.sql`.
  - **Generated-file conflicts:** take either side and re-run `sqlc generate` on the merged tree. Never hand-merge `sqlcgen/`.
  - Both DSNs enable `foreign_keys(1)` (`cmd/plantry/main.go:282`, `testhelper/db.go:22`).
- **Profile:**
  - The table is `00007_profile.sql`, with all targets nullable.
  - Domain `domain/profile/profile.go` and validation `profile/service.go:33-63`.
  - The handler `handlers/profile.go:80-118` is a full replace that defaults `locale` to `en`.
- **Settings:**
  - Registry `domain/settings/specs.go:5-73`; key constants `settings.go:88-101`.
  - Kinds are string, int, bool and enum; bools parse with `strconv.ParseBool` (`settings/service.go:225`).
  - `plan.anchor` defaults to `today` (`specs.go:51-56`). `plan.window_days` is unused (`specs.go:65-72`, with no reader outside the registry).
  - `PUT /api/settings/{key}` and `DELETE /api/settings/{key}` are in `router.go:166-175`.
- **Plates:**
  - The current table is `id, slot_id, note, created_at, skipped, date`, last rebuilt in `00016_drop_week_aggregate.go:27-34`. Indexes are `ix_plates_date` and `ix_plates_date_slot`; there is no uniqueness.
  - `plate_feedback.plate_id` is `PRIMARY KEY REFERENCES plates(id) ON DELETE CASCADE` (`00009_plate_feedback.sql:3`).
  - `Plate.Skipped` is at `domain/plate/plate.go:13`. The repo port has `SetSkipped` (`repository.go:30`), the service `SetSkipped` (`service.go:301-305`) and `Update` (`service.go:176-191`, date/slot moves). The query is `SetPlateSkipped` (`queries/planner.sql:43`).
  - The handler `SetSkipped` is at `handlers/plates.go:355-380`, the route at `router.go:110`, and the `skipped` JSON field at `plate_response.go:29,63`.
- **Skipped in other backend code:**
  - Agent: `prompt.go:34` (`PromptPlate.Skipped`), `:219-220` (plate line), `:179` (`Shopping day: %s` prints the raw int), `service.go:336` and `:357-362` (mode hints), `tools.go:314` (tool description) and `:761` (`plateSummary`).
  - Presets: `handlers/presets.go:487` and `:602`. Copy week is at `preset/apply.go:134`, with `SkippedOccupied`/`SkippedNoSlot` at `:54-55`.
- **Presets** (routine source): `presets` → `preset_plates(slot_id)` → `preset_components` (`00019_presets.sql`).
- **Transactions:** per-domain `RunIn…Tx` helpers on `adapters/sqlite/tx.go:15` (`RunInPresetTx`, `RunInFeedbackTx`), with matching `TxRunner` ports in each domain (`preset/repository.go:79`, `feedback/repository.go:21`).
- **Slots:** there is no seeding migration. `time_slots(name_key, icon, sort_order, active)`. The frontend labels slots with `slotLabel` (`lib/slot-label.ts:11`), which translates keys like `slot.breakfast`.
- **Foods:** `GET /api/foods` returns `{items, total}` (`handlers/foods.go:194,568`). Count composed foods with a dedicated query; don't page through the list.
- **AI run log:**
  - Migration `00021_ai_runs.sql`; port `domain/airun/store.go:90-95`.
  - The meter `airun/meter.go:28-50` (Stream) and `:82-128` (`llmCall`) records only the model, tokens and duration. Request messages, parameters and the response are not stored.
  - The repo truncates step JSON at 8192 bytes (`ai_run_repo.go:16-18,85-86,125`).
  - `llm.Request` (`domain/llm/client.go:63-70`) has `Model, System, Messages, Tools, Temperature, MaxTokens`. `llm.Response` (`:92-96`) has `Message, StopReason, Usage`.
  - Lookup calls: `food/ai.go:133` (translate, max 200 tokens) and `:199` (pick-best, max 50). Import fallback: `importer/llm_prompt.go:58-64` (max 4096) and the retry at `:79-83`.
- **Frontend skip surface:**
  - `lib/planner-skip.ts` and its test; `lib/api/plates.ts:46,161-170,206`; `lib/queries/plates.ts` (`useSetPlateSkipped`).
  - `SlotCell.tsx:94` (`SkippedSlot`), `SlotSheet.tsx:247-267,404-416,965-1003`, `DndCellWrapper.tsx:48-74`, `PlannerGrid.tsx:633-637`, `MobilePlannerGrid.tsx:30,42`, `MobileSlotRow.tsx:47,217-225`.
  - `ReadOnlyPlannerGrid.tsx`, `calendar/MonthCell.tsx` and `test/fixtures.ts`.
  - The i18n `skip.*` keys.
  - E2E: `planner-skip-favorite`, `planner-clear`, `planner-mobile`, `planner-cross-week-drag`.
- **Settings UI:**
  - `routes/settings/index.tsx` uses `?tab=` with six tabs.
  - `SettingsShell.tsx` uses `PageHeader eyebrow` and the legacy `border-outline` and `text-on-surface-variant` classes.
  - The dead secret-key banners are at `AITab.tsx:51-56,78-95` and `SystemTab.tsx:93-99`. The cipher always exists since Phase 0 D1 (`main.go:83-90` always builds it).
- **Recipes page:**
  - `routes/components/index.tsx` renders `ComponentList`.
  - Its header buttons use legacy `gradient-primary editorial-shadow` (`ComponentList.tsx:156,417`).
- **Shell:**
  - `GeneratePlanButton.tsx:23-40` has shimmer, gradient, hover lift and sage shadows in all three variants, and `MobileBottomNav.tsx:18-28` places it as a centre FAB.
  - `components/ui/button.tsx:13-21` has the hover lift and sage shadow.
  - `frontend/index.html:2` hardcodes `lang="en"`, and `lib/i18n/index.ts` never updates it.
- **IngredientEditor** sends missing macros as 0 (`ingredients/IngredientEditor.tsx:123-128`; defaults at `:93-94`).
- **E2E harness:**
  - `playwright.config.ts` has the projects `chromium` and `ai-serial` and spawns a fresh backend on `/tmp/plantry-e2e.db`.
  - `e2e/global-setup.ts` deletes the DB files.
  - Seed through `API = http://localhost:8080` (`e2e/helpers.ts:10`).

---

## 3. Steps

Every step:
- Load the required skills first. Go: `golang-patterns` and `golang-testing`. Frontend: `frontend-design`, `vercel-react-best-practices`, `vercel-composition-patterns`, `shadcn`, `vitest` and `playwright-best-practices`. UI work adds `impeccable` (D26) and `frontend-design`.
- Write the listed tests first and watch them fail, then implement, then run the step's verification.
- Make surgical changes (root `CLAUDE.md` §3). Touch only the listed files; ask before going outside them.
- User-visible strings go through i18n in both `en.json` and `de.json`. The German is written natively (UX §7), and the vocabulary is "No plate / Kein Essen", "Routine", "Assistant / Assistent".
- New and rebuilt UI uses the Cookbook token names only (DESIGN.md): no legacy aliases, no gradients or shadows on cards, focus rings at 2 px accent with a 2 px offset, touch targets ≥ 44 px, and `kcal`/`g` through i18n unit keys (`units.kcal`, `units.g`), never hardcoded.
- Inside a worktree, prefix `go`, `golangci-lint`, `sqlc`, `git commit` and `bun run e2e` with `GOWORK=off`, and run `bun install` before any frontend command. Use `go test -p 4`. Run at most one e2e or dev server at a time, on non-default ports if 8080/5173 are busy, and stop it afterwards.

### Track P: Preferences

#### P1. Profile fields and settings keys (backend)

- **Files**
  - new `backend/db/migrations/00023_profile_planning_prefs.sql` and `adapters/sqlite/migration_00023_test.go`
  - `adapters/sqlite/queries/profile.sql`, regenerated `sqlcgen/`, `adapters/sqlite/profile_repo.go` and its test
  - `domain/profile/profile.go`, `service.go` and `service_test.go`
  - `domain/settings/specs.go`, `settings.go` and `specs_test.go`
  - `transport/http/handlers/profile.go` and `profile_test.go`
- **Behaviour**
  - **Migration 00023:**
    - `ALTER TABLE user_profile ADD COLUMN diet_type TEXT NOT NULL DEFAULT 'omnivore' CHECK (diet_type IN ('vegan','vegetarian','pescetarian','omnivore'))`
    - `ADD COLUMN meat_days_per_week_max INTEGER CHECK (meat_days_per_week_max IS NULL OR meat_days_per_week_max BETWEEN 0 AND 7)`
    - `ADD COLUMN household_portions REAL NOT NULL DEFAULT 1.0 CHECK (household_portions > 0)`
    - `DELETE FROM app_settings WHERE key = 'plan.window_days'`
    - `INSERT INTO app_settings (key, value) SELECT 'setup.completed', 'true' WHERE EXISTS (SELECT 1 FROM time_slots)` (D3)
    - Down reverses everything except the deleted row.
  - **Profile domain:** the three fields. `DietType` is a typed string with `Valid()`.
  - **Validation** (domain errors through the existing error map; add keys as needed):
    - an unknown diet type gives `error.profile.invalid_diet_type`
    - `meat_days_per_week_max` outside 0–7 gives `error.profile.invalid_meat_days`
    - `household_portions` ≤ 0 or > 20 gives `error.profile.invalid_household_portions`
  - **Handler:** request and response gain `diet_type`, `meat_days_per_week_max` and `household_portions`. Per D5, an absent `diet_type`/`household_portions` keeps the stored value (decode into pointers and merge with `Get`). `meat_days_per_week_max: null` clears it.
  - **Settings registry:** apply D4 exactly (keys, kinds, ranges, defaults). Add the key constants. Remove `KeyPlanWindowDays` and its spec.
- **Tests first**
  - The migration adds the columns with their defaults. A fresh DB has no `setup.completed` row. A DB with a slot gets `setup.completed = true`. A `plan.window_days` row is deleted. An invalid `diet_type` insert is rejected by the CHECK.
  - Repo round-trip of the three fields, including `meat_days_per_week_max` null → 3 → null.
  - Service table tests for each validation boundary (household 0, 0.5, 20, 20.5; meat −1, 0, 7, 8; diet "keto").
  - Handler: a PUT without `diet_type` keeps it; malformed JSON gives 400; an invalid value gives 400 with the key.
  - Settings: every D4 key resolves its default; out-of-range ints and a bad bool are rejected; `plan.window_days` is now an unknown key; `plan.anchor` defaults to `next_shopping_day`.
- **Verify:** `cd backend && sqlc generate && go test ./internal/adapters/sqlite/ ./internal/domain/profile/ ./internal/domain/settings/ ./internal/transport/http/handlers/ -run 'Profile|Settings|00023' && go vet ./...`

### Track A: Availability

#### A1. Availability tables, domain and API

- **Files**
  - new `db/migrations/00024_availability.sql` and `adapters/sqlite/migration_00024_test.go`
  - new `adapters/sqlite/queries/availability.sql`, regenerated `sqlcgen/`, new `adapters/sqlite/availability_repo.go` and its test
  - `adapters/sqlite/tx.go`: new `RunInAvailabilityTx`
  - new `domain/availability/availability.go`, `repository.go`, `service.go` and `service_test.go`
  - new `transport/http/handlers/availability.go` and its test; `handlers/errormap.go`
  - `transport/http/router.go` and `cmd/plantry/main.go` (wiring only)
- **Schema** (D7):
  ```sql
  CREATE TABLE availability_defaults (
    weekday   INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    slot_id   INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE CASCADE,
    mode      TEXT NOT NULL CHECK (mode IN ('plan','routine','none')),
    preset_id INTEGER REFERENCES presets(id) ON DELETE CASCADE,
    CHECK ((mode = 'routine') = (preset_id IS NOT NULL)),
    PRIMARY KEY (weekday, slot_id)
  );
  CREATE TABLE availability_overrides (
    date      TEXT NOT NULL,             -- YYYY-MM-DD
    slot_id   INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE CASCADE,
    mode      TEXT NOT NULL CHECK (mode IN ('plan','routine','none')),
    preset_id INTEGER REFERENCES presets(id) ON DELETE CASCADE,
    note      TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK ((mode = 'routine') = (preset_id IS NOT NULL)),
    PRIMARY KEY (date, slot_id)
  );
  ```
  A `plan` default is never stored: writing the full grid deletes rows that are `plan`, so "no row" means `plan`.
- **Domain** (`domain/availability`):
  - `Mode` (`plan`/`routine`/`none`) and `Source` (`default`/`override`/`fallback`).
  - `Cell{Date, SlotID, Mode, Source, PresetID *int64, PresetName *string, Note *string}`.
  - **Service:**
    - `Defaults(ctx) ([]Default, error)`: the full weekday × active-slot grid, with missing rows as `plan`.
    - `SetDefaults(ctx, []Default) error`: replaces all rows in one tx and validates every row.
    - `Resolve(ctx, from, to) ([]Cell, error)`: every date × active slot, with override > default > `plan` (§3.2). It costs one query per table, no N+1.
    - `ModeAt(ctx, date, slotID) (Mode, error)`
    - `SetOverride(ctx, OverrideInput{Date, SlotID, Mode, PresetID, Note, RemovePlate}) (Cell, *plate.Plate /*removed*/, error)`
    - `ClearOverride(ctx, date, slotID) error`
  - **Validation:**
    - the slot exists and is active
    - a routine needs an existing preset, and a non-routine has no preset (`error.availability.preset_required`, `error.availability.preset_not_allowed`)
    - a valid date
    - a note of at most 200 chars, kept only for `none`
    - `SetDefaults` rejects a weekday outside 0–6 and duplicates
  - **D9:** `SetOverride` with `none` reads the cell's plate inside the tx. If one exists: without `RemovePlate` it returns `ErrCellHasPlate`; with it, it deletes the plate (cascading its components and feedback) and returns the deleted plate, loaded before the delete.
- **API** (spec §12; JSON dates `YYYY-MM-DD`):
  - `GET /api/availability/defaults` → `{items: [{weekday, slot_id, mode, preset_id}]}`. It is a full grid over active slots.
  - `PUT /api/availability/defaults` takes `{items: [...]}`, the full grid, and returns it.
  - `GET /api/availability?from&to` → `{items: [Cell JSON: date, slot_id, mode, source, preset_id, preset_name, note]}`. A range over 62 days, a missing date or `to < from` returns 400.
  - `PUT /api/availability/{date}/{slotId}` takes `{mode, preset_id?, note?, remove_plate?}` → `{cell, removed_plate?}`. `removed_plate` uses the existing plate JSON (`plate_response.go`).
  - `DELETE /api/availability/{date}/{slotId}` → 204, idempotent.
  - Errors: 400 validation; 404 unknown slot or preset; 409 `error.availability.cell_has_plate`.
- **Tests first**
  - Migration: tables exist; CHECK rejects a routine without a preset and `none` with a preset; deleting a preset cascades; deleting a slot cascades.
  - Service (table-driven with fakes): resolution precedence for every combination of default {no row, none, routine} × override {no row, plan, none, routine}; an inactive slot is excluded; validation cases; `SetDefaults` drops `plan` rows; D9 with and without `remove_plate`.
  - Repo against `testhelper.NewTestDB()`: round-trip; a `Resolve` over 7 days × 3 slots issues a bounded number of queries (assert on the result, not on query counting).
  - Handlers: each endpoint's happy path; malformed JSON; a non-numeric slot id; a bad date; range limits; 409 with `remove_plate` absent; 200 with `removed_plate` present.
- **Verify:** `cd backend && sqlc generate && go test ./internal/adapters/sqlite/ ./internal/domain/availability/ ./internal/transport/http/... -run 'Availability|00024' && go vet ./...`

#### A2. Skipped plates → overrides, one plate per cell, availability-aware plate rules

Needs A1. It replaces every backend use of `skipped`.

- **Files**
  - new `db/migrations/00025_plates_cell_unique.go` and its test (beside `00014_date_keyed_plates_test.go`, or `adapters/sqlite/migration_00025_test.go`, following whichever pattern the 00014/00016 tests use)
  - new `db/schema_patches/00025_plates_cell_unique.sql` (`ALTER TABLE plates DROP COLUMN skipped;` plus the index)
  - `queries/planner.sql` (remove `SetPlateSkipped` and `skipped` from selects; add a cell lookup, e.g. `GetPlateByCell`), regenerated `sqlcgen/`, `plate_repo.go` and its test
  - `domain/plate/plate.go`, `repository.go`, `service.go` and `service_test.go`: drop `Skipped`/`SetSkipped`; add a `CellChecker` port (`ModeAt`) and the D10 rules in `Create` and `Update`
  - `domain/preset/apply.go` and `apply_test.go`: D12
  - `domain/agent/prompt.go`, `service.go`, `tools.go`, their tests and `tools_payload_test.go`: D13
  - `handlers/plates.go` (remove `SetSkipped`), `plate_response.go`, `presets.go`, `errormap.go` and their tests
  - `router.go` (remove `/skip`) and `cmd/plantry/main.go` (pass the availability service to the plate and preset services and to the agent `WindowSources`)
- **Behaviour**
  - **Migration 00025** follows D11, in one transaction, and logs counts (overrides created, plates deleted, duplicates merged) through the goose logger or `slog`. Its Down re-adds `skipped` (default 0) and drops the unique index. It does not recreate skipped plates.
  - **Plate service:**
    - D10 on `Create` and on `Update` when date or slot changes. The checks run in the service; the unique index is the safety net.
    - A raw constraint failure from the repo (a race) still maps to `ErrCellOccupied`.
    - New domain errors `ErrCellUnavailable` and `ErrCellOccupied` map to 409 with the keys from D10.
  - **Preset apply and copy week:** D12. `skipped_unavailable` is added to the apply result JSON next to `skipped_occupied`. The undo snapshot also restores overrides that copy week created.
  - **Agent:** D13. The prompt gets a "No-plate and routine cells" section within `promptCharBudget`. `get_plates_range` returns the `availability` array. Remove `skipped` from `PromptPlate`, the plate line, the tool payload and the descriptions. Bump `PromptVersion` to `"v3"`.
- **Tests first**
  - Migration on a seeded DB:
    - a skipped plate with a note becomes a `none` override with that note
    - a cell with a skipped plate and a real plate keeps the real plate and gets no override
    - three duplicates merge into the lowest id, with all components kept in order and the survivor's feedback kept
    - the column is gone and a duplicate insert fails
    - Down/Up round-trips
  - Plate service: a create into a `none` cell gives `ErrCellUnavailable`; into an occupied cell `ErrCellOccupied`; a move into either is rejected; a move within the same cell (a note-only update) is allowed; `routine` cells accept a manual plate.
  - Preset apply: a target `none` cell is reported in `skipped_unavailable`; copy week copies source `none` overrides and undo removes them.
  - Agent: the prompt lists `none`/`routine` cells with weekday names; `Shopping day: Saturday` (not `5`); the tool payload has no `skipped` and includes `availability`; `create_plate` into a `none` cell returns the tool error carrying the i18n key.
  - Handlers: 409 bodies for both conflict keys; `/skip` is 404/405; plate JSON has no `skipped`.
- **Verify:**
  ```bash
  cd backend && sqlc generate && go test -race ./internal/... -p 4 && golangci-lint run
  grep -rn "skipped\|Skipped" internal cmd --include=*.go | grep -v _test.go | grep -v sqlcgen   # only preset-apply skip lists remain
  ```

### Track S: Setup

#### S1. Setup API (backend)

Needs P1.

- **Files**
  - new `domain/setup/service.go` and `service_test.go`
  - `queries/foods.sql` (+ `CountComposedFoods :one`), regenerated `sqlcgen/`, `food_repo.go` and its test
  - new `handlers/setup.go` and its test; `router.go`; `cmd/plantry/main.go`
- **Behaviour** (D14): the service reads `setup.completed` through the settings service and `recipe_count` from `CountComposedFoods`; `RecipeTarget = 10` is a domain constant. `Complete` sets the setting. Routes: `GET /api/setup` and `POST /api/setup/complete`.
- **Tests first**
  - The service with fakes: fresh gives completed false and count 0; after `Complete` it is true; the count reflects composed foods only (leaves excluded).
  - Handler JSON shape for both routes; POST is idempotent.
- **Verify:** `cd backend && sqlc generate && go test ./internal/domain/setup/ ./internal/transport/http/handlers/ ./internal/adapters/sqlite/ -run 'Setup|CountComposed'`

#### S2. Setup wizard (`/setup`, frontend)

Needs S1, P1, A1, F1 and F3 (which provides the setup client hooks and the extracted settings controls).

- **Files**
  - new `src/routes/setup/index.tsx`
  - new `src/components/setup/` (`SetupWizard.tsx`, one file per step, `StepIndicator.tsx`, `nutrition-presets.ts` for D6) and tests
  - `src/routes/index.tsx`: `beforeLoad` redirect (D2) using `queryClient.ensureQueryData(setupKeys.status())`
  - `src/routes/__root.tsx`: only if the wizard must render without the app shell (decide by reading it; UX §5.1 wants a centred single column)
  - i18n files
  - E2E: new `e2e/setup-wizard.spec.ts`; `playwright.config.ts` (the D22 `setup` project); `e2e/global-setup.ts` only if needed
- **Behaviour** (spec §7.1, UX §5.1, D14–D17):
  - **Layout:** a centred column, max 560 px, on every size. The step indicator is numbered "Step 2 of 7". The footer has Back, **Next** (primary) and, from step 2, the text button "Skip, use defaults" (D16). Each step leads with a one-line explanation of why it matters for planning.
  - **Steps**, all prefilled from current values or the defaults:
    1. **Welcome:** language (en/de, browser locale preselected). Next sets `profile.locale` and switches i18n.
    2. **You & household:** diet type (4 cards); household helper (adults stepper × 1.0 + children stepper × 0.5, showing "= 1.5 portions per plate", storing only `household_portions`); allergies as chips (`dietary_restrictions`); optional meat-days limit.
    3. **Nutrition goal:** cut, maintain and bulk cards (D6) plus custom (kcal and P/F/C %, with the existing ≤ 100 % validation). Maintain is preselected.
    4. **Meals & week:** slot list (D15) plus `AvailabilityGrid` over weekday × slots. Next writes `PUT /api/availability/defaults`.
    5. **Cooking style:** cooking occasions per day (1–3), batch cooking toggle, default keep days (1–7, with a food-safety note above 4), freezing toggle.
    6. **Variety & shopping:** variety weeks (0–8), shopping day (Saturday preselected), new recipes per week (0–3).
    7. **Assistant (optional):** provider, model and key (D17); notes to your assistant (`profile.system_prompt`); daily suggestion toggle (`discovery.daily_enabled`). Without a key it shows the calm note "You can add this later. Planning works without it."
  - **Finish:** "You're set up. Add 10 recipes and Plantry can plan your weeks." with **Add recipes**. It calls `POST /api/setup/complete`, invalidates the setup query and navigates to `/components`.
  - **Saving:** each step persists on Next with the existing mutations (profile, settings, slots, availability). A failure keeps the user on the step, with the mutation's toast.
  - **Re-run:** opening `/setup` on a completed install works the same way, prefilled.
- **Tests first**
  - Vitest: the index redirects when `completed:false` and not when true; every step renders its defaults; the household helper computes 2 adults + 1 child = 2.5; "Skip, use defaults" on step 2 writes the step 3–6 defaults and reaches Finish; Finish calls complete then navigates.
  - E2E `setup-wizard.spec.ts` (project `setup`, D22):
    - Fresh DB: `/` lands on `/setup`. Next through every step, Finish.
    - Assert: three slots exist, the profile is maintain 2200/30/30/40, `GET /api/availability/defaults` is all `plan`, `setup.completed` is true.
    - The page is `/components` showing the guided state (the count is real here: a fresh DB has 0 recipes).
    - The spec ends by setting `plan.anchor = today` (D21).
- **Verify:** `cd frontend && bun run check && GOWORK=off bun run e2e -- setup-wizard shell planner`

### Track R: Recipe onboarding

#### R1. Guided Recipes state and the Generate gate

Needs S1 and F3 (setup client hooks).

- **Files**
  - `src/components/components/ComponentList.tsx` and its test
  - new `src/components/components/RecipeOnboarding.tsx` (progress row plus option cards) and its test
  - `src/components/shell/GeneratePlanButton.tsx`, `MobileBottomNav.tsx` and `TopBar.tsx`, plus their tests
  - `src/components/ingredients/IngredientEditor.tsx` and its test (Phase 0 follow-up)
  - i18n files
  - new `e2e/recipes-guided.spec.ts`; update `e2e/shell.spec.ts` (the FAB is gone)
- **Behaviour**
  - **Guided state** (D18, UX §5.8):
    - Above the grid: "4 of 10 recipes. Plantry needs 10 to plan a varied week." with a progress bar (`role="progressbar"`, with value text).
    - Two option cards, Import from a link and Create manually, each with a two-sentence explanation and its action. Cookbook tokens, no shadow, button radius.
    - An empty library with 0 recipes shows the same cards in place of the empty grid.
  - **Collapse at 10:** the cards disappear, and the milestone toast fires once (D18).
  - **ComponentList header:** replace the legacy `gradient-primary editorial-shadow` buttons (`:156,:417`) with the default primary button, since this step rebuilds that header.
  - **GeneratePlanButton** (D19):
    - Restyle every variant to Cookbook: solid accent, no shimmer, gradient, hover lift, sage shadow or rotate.
    - Disabled with `aria-disabled` while `recipe_count < 10`. A tooltip, plus a visible link for the default variant: "Add 6 more recipes to start planning" → `/components`.
    - Remove the `fab` variant and the centre slot in `MobileBottomNav` (a four-column grid). The phone TopBar gets a compact icon variant next to the gear.
  - **IngredientEditor:** empty macro fields are sent as `null`, not 0. A zero typed by the user stays 0. This keeps nutrition completeness (Phase 0 C2) honest for manually created ingredients.
- **Tests first**
  - Vitest: the guided state at count 4 shows the progress text and both cards; at 10 there are no cards; the toast fires once across re-renders when the flag is set; the Generate button is disabled below 10 with the right count text and enabled at 10; MobileBottomNav renders exactly four links and no FAB; IngredientEditor submits `kcal_100g: null` for an empty field.
  - E2E `recipes-guided.spec.ts` (parallel project): stub `GET /api/setup` (D22) at counts 3 and 10 and assert cards, progress text and the Generate state; the import card navigates to `/import`.
- **Verify:** `cd frontend && bun run check && GOWORK=off bun run e2e -- recipes-guided shell components ingredients`

### Track F: Frontend screens and shared pieces

#### F1. Availability client, hooks and `AvailabilityGrid`

Codes against the A1 API shapes above; A1 runs in the same wave.

- **Files**
  - new `src/lib/api/availability.ts`, `src/lib/queries/availability.ts` (and tests), `src/lib/queries/keys.ts` (`availabilityKeys`)
  - new `src/components/availability/AvailabilityGrid.tsx`, `RoutinePicker.tsx` and tests
  - `src/test/fixtures.ts` (availability fixtures)
  - i18n files
- **Behaviour**
  - **API functions** for every A1 endpoint. Hooks: `useAvailabilityDefaults`, `useSetAvailabilityDefaults`, `useAvailabilityRange(from, to)`, `useSetAvailabilityOverride` and `useClearAvailabilityOverride`. Each mutation owns its invalidation (availability keys plus `plateKeys.range` when `removed_plate` is returned) and its `toastError`.
  - **`AvailabilityGrid`** is a presentational component used by setup, settings and later the plan wizard (UX §9):
    - Props: `columns: {key: string, label: string, sublabel?: string}[]` (weekdays now, dates in Phase 3), `slots`, `value: Record<"<colKey>:<slotId>", {mode, presetId?}>`, `presets`, `onChange(colKey, slotId, next)` and an optional `disabled`. It owns no server state.
    - **Cell tap** cycles Plan → No plate → Routine. Choosing Routine opens `RoutinePicker`, a popover listing presets, with an empty-state link to `/presets`. Cancelling the picker leaves the cell unchanged.
    - **Rendering:**
      - Plan is a plain panel cell.
      - No plate is transparent with italic "No plate" (UX §5.5).
      - Routine is "Routine" plus the preset name, truncated.
      - The text always names the state; colour is never the only signal.
    - **Accessibility:** a real `role="grid"` with row and column headers. Each cell is a button whose name reads in full, e.g. "Monday lunch, No plate". Arrow keys move between cells; Enter and Space cycle.
    - **Layout:** 7 columns plus a header column, `rounded-cell` cells, at least 44 px targets on touch. Below 768 px the grid scrolls horizontally inside its own container, with a sticky slot column.
  - **Bulk actions** come as optional props, rendered only when given: a per-column menu with "No plate all day" / "Plan all day".
- **Tests first** (Vitest and Testing Library): the cycle order; routine opens the picker, picking sets the preset, cancelling reverts; keyboard navigation and accessible names; `onChange` payloads; a bulk action sets every slot of a column; hooks call the right endpoints, and a `removed_plate` response invalidates plates.
- **Verify:** `cd frontend && bun run check`

#### F2. Planner: "No plate" is a cell state

Needs F1 and A2 (API shape: no `skipped` and no `/skip`; the 409 keys from D10). It runs beside A2 and codes against this plan.

- **Files**
  - planner: `PlannerGrid.tsx`, `MobilePlannerGrid.tsx`, `SlotCell.tsx`, `SlotSheet.tsx`, `DndCellWrapper.tsx`, `MobileSlotRow.tsx`, `ReadOnlyPlannerGrid.tsx` and their tests
  - `calendar/MonthCell.tsx` and its test
  - `src/routes/index.tsx` (load the availability range for the window)
  - `lib/api/plates.ts`, `lib/queries/plates.ts` and tests; delete `lib/planner-skip.ts` and its test; `lib/planner-keynav.ts` if it references skipped
  - `test/fixtures.ts`, i18n (remove the `skip.*` keys; add `availability.*` planner keys)
  - E2E: rename `planner-skip-favorite.spec.ts` to `planner-no-plate-favorite.spec.ts`; update `planner-clear`, `planner-mobile` and `planner-cross-week-drag` where they use skip
- **Behaviour**
  - The planner fetches `useAvailabilityRange(window)` next to the plates and passes a cell-mode lookup to both grids.
  - **`none` cell:** italic "No plate", plus the note if one is set. Its actions are "Plan this meal" (`DELETE` the override if its source is `override`, otherwise `PUT` mode `plan`) and "Edit note".
  - **`routine` cell without a plate:** "Routine · Oats", plus the normal add action (D8).
  - **"Mark as No plate"** in the empty-cell actions, `SlotSheet` and the mobile swipe drawer: `PUT` override `none` with `remove_plate: true`, then a 6 s Undo toast (D9). Undo restores the previous override state (clear, or re-put the previous mode/preset/note) and, when a plate was removed, recreates it with its components.
  - **DnD:** `DndCellWrapper` exposes `mode` and `existingPlateId`. Drops onto `none` cells (`planner.dnd.reject_no_plate`) or occupied cells (`planner.dnd.reject_occupied`) are rejected with a toast and no API call. A 409 from the server, from a race, shows its message key.
  - **Copy/apply toasts** report `skipped_unavailable` (`preset-apply-toast.ts`).
  - `ReadOnlyPlannerGrid` and `MonthCell` drop the skipped hatch. They show `none` only where availability data is already at hand; the calendar views go away in Phase 2, so don't add fetching there.
  - Keyboard shortcut: the existing skip shortcut becomes "Mark as No plate", and the cheatsheet is updated.
- **Tests first**
  - Vitest: a `none` cell renders "No plate" plus the note and offers "Plan this meal"; Mark as No plate calls the override with `remove_plate`; Undo restores the plate; DnD onto `none` or occupied cells makes no mutation and shows a toast.
  - E2E `planner-no-plate-favorite.spec.ts`: mark a planned cell No plate → the plate is gone and the cell says No plate → Undo brings it back → mark again, reload, still No plate → "Plan this meal" clears it. A plate cannot be dragged onto a No-plate cell. Seed through the API with unique names, and clean up in `finally` (including overrides).
- **Verify:** `cd frontend && bun run check && GOWORK=off bun run e2e -- planner-no-plate-favorite planner-clear planner-mobile planner-cross-week-drag planner-dnd planner`

#### F3. Settings restructure (UX §5.11) with the new fields

Needs P1, A1 and F1.

- **Files**
  - routes: `src/routes/settings/index.tsx` (redirect plus legacy `?tab=` mapping, D20) and new `src/routes/settings/$section.tsx`
  - `components/settings/SettingsShell.tsx`; new section components `components/settings/sections/{Household,Meals,Cooking,Assistant,System}Section.tsx`
  - existing panels moved or extended: `ProfileEditor.tsx`, `TimeSlotsEditor.tsx`, `tabs/PlanTab.tsx`, `tabs/AITab.tsx`, `tabs/SystemTab.tsx`, `tabs/GeneralTab.tsx`, `DisplayPreferences.tsx`, `LanguagePreference.tsx` and their tests. Delete files that end up empty.
  - `lib/api/profile.ts`, `lib/api/settings.ts`, related queries and tests
  - new `lib/api/setup.ts` and `lib/queries/setup.ts` (`setupKeys`, `useSetupStatus`, `useCompleteSetup`), consumed by S2 and R1
  - `test/fixtures.ts`, i18n
  - E2E: `profile.spec.ts`, `settings-ai.spec.ts`, `settings-slots.spec.ts`, `planner-anchor-settings.spec.ts`, plus any spec navigating `/settings?tab=` (grep)
- **Behaviour**
  - **Layout:** desktop and tablet show a section list on the left with content on the right. The phone shows the list, which drills into a section with a back link. Use Cookbook tokens, no `PageHeader` eyebrow, and the sentence-case h1 "Settings".
  - **Sections:**
    - **You & household:** diet type, the household helper (as in the wizard, from a shared component under `components/setup/` or `components/settings/`; pick one place and import it from the other), allergies, the meat-days limit, and the nutrition goal (D6 cards plus custom macro targets from `ProfileEditor`).
    - **Meals & week:** `TimeSlotsEditor`; the default availability grid (`AvailabilityGrid` + `useAvailabilityDefaults`); Routines as a link to `/presets` (D20).
    - **Cooking & variety:** cooking occasions per day, batch cooking, default keep days (food-safety warning above 4), freezing, freezer keep days (shown when freezing is on), variety weeks, new recipes per week, shopping day, plan anchor and week start (from `PlanTab`).
    - **Assistant:** provider, model, key and rate limit (from `AITab`); notes to your assistant (`system_prompt`, labelled as UX §7); daily suggestion; run retention days (`ai.run_retention_days`).
    - **Data & system:** language, theme, the FDC key, "Re-run setup wizard" (→ `/setup`), about/version (from `SystemTab`), and the developer preferences.
  - **Effect lines:** every setting shows its effect in one line, e.g. "Plantry plans at most 1 cooking session per day".
  - **Saving:** on blur or change, with a quiet "Saved" note. There is no global Save button. Reuse `SettingRow`'s source badge.
  - **Remove the dead secret-key banners** in `AITab` and `SystemTab`, and their i18n keys (Phase 0 follow-up). The backend `cipher_available` field stays; it is still true and harmless.
- **Tests first**
  - Vitest: `/settings` redirects to household; `?tab=ai` maps to assistant; each section renders its fields with the defaults; changing keep days to 5 shows the warning; the household helper writes `household_portions`; the availability grid saves through `PUT /defaults`; there is no secret-key banner.
  - E2E updates for the moved panels. In a new `settings-preferences.spec.ts`, set diet type, household portions and two settings, reload, and assert persistence. Restore the values in `finally`, because settings are global; if a setting change can affect parallel specs, put the spec in `ai-serial`.
- **Verify:** `cd frontend && bun run check && GOWORK=off bun run e2e -- settings profile planner-anchor-settings`

### Track Z: Phase 0 follow-ups (foundations)

#### Z1. Motion, focus, buttons and document language

- **Files:** `src/index.css`, `src/test/tokens.test.ts`, `src/components/ui/button.tsx` and its test (if one exists), `src/lib/i18n/index.ts` and a new `src/lib/i18n/index.test.ts`, `frontend/index.html` (only if needed).
- **Behaviour**
  - **Reduced motion:** a global `@media (prefers-reduced-motion: reduce)` rule sets transition and animation durations to near zero and removes transforms used for slides, so sheets fade instead of sliding (UX §3.4, §8).
  - **Focus:** the base layer gives every focusable element `:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px }`. Remove the conflicting `focus-visible:ring-3 ring-ring/50` in `button.tsx` so buttons show the same outline (UX §6).
  - **Buttons:** drop the hover lift (`hover:-translate-y-*`), the sage `rgba(74,101,77,…)` shadows and `hover:shadow-sm` from every variant (`button.tsx:13-21`). Keep the colour changes on hover and `active:translate-y-px`. The shadcn file is already hand-modified, so editing it back is in line with DESIGN.md.
  - **`<html lang>`:** `i18n/index.ts` sets `document.documentElement.lang` on init and on every `languageChanged`.
- **Tests first:** `tokens.test.ts` asserts that the reduced-motion block and the focus outline rule exist; a button test asserts no `translate-y` hover class and no `rgba(74,101,77` in the variant classes; the i18n test switches to `de` and asserts `document.documentElement.lang === "de"`.
- **Verify:** `cd frontend && bun run check && bun run build`

### Track T: AI trace completeness

#### T1. Replayable LLM calls, run identity and retention

Runs after A2 (both touch `agent/service.go`).

- **Files**
  - new `db/migrations/00026_ai_trace.sql` and `adapters/sqlite/migration_00026_test.go`
  - `queries/ai_runs.sql`, regenerated `sqlcgen/`, `adapters/sqlite/ai_run_repo.go` and its test
  - `domain/airun/store.go`, `recorder.go`, `meter.go` and `meter_test.go`; new `domain/airun/replay_test.go`
  - `domain/agent/service.go` and its test (pass `conversation_id`)
  - `domain/food/resolver.go` and `importer/llm_prompt.go`, only if a test shows a gap
  - `cmd/plantry/main.go` (startup prune)
- **Schema:**
  ```sql
  ALTER TABLE ai_runs ADD COLUMN conversation_id INTEGER;
  ALTER TABLE ai_runs ADD COLUMN request_id TEXT;
  ALTER TABLE ai_run_steps ADD COLUMN request_json TEXT;
  ALTER TABLE ai_run_steps ADD COLUMN response_json TEXT;
  ALTER TABLE ai_run_steps ADD COLUMN stop_reason TEXT;
  CREATE TABLE ai_tool_schemas (hash TEXT PRIMARY KEY, json TEXT NOT NULL);
  CREATE INDEX ix_ai_runs_conversation ON ai_runs(conversation_id);
  ```
- **Behaviour** (D23, D24):
  - **Start:** `airun.Start` gains `ConversationID *int64`. `RunStart` carries it plus the request id that `Recorder.Start` already reads.
  - **`llmCall` records, per call:**
    - the request delta (tracked per run as `recordedMsgs int`)
    - temperature, `max_tokens` and `tools_hash` (upserting the schema once per distinct hash per run, the way `UpsertPrompt` is used)
    - the response content blocks (text and tool_use) and `stop_reason`; on error, `response_json` is null and `is_error` is set as today
  - **Caps:** each body ≤ 256 KiB, cut with valid JSON and `truncated: true`. The 8 KB cap for tool-call `args_json`/`result_json` is unchanged.
  - **Store:** the `Store` port gains `UpsertToolSchema(ctx, hash, json)` and `PruneSteps(ctx, before time.Time) (int64, error)`. `Step` gains `RequestJSON`, `ResponseJSON` and `StopReason`.
  - **Retention:** at startup, `main.go` reads `ai.run_retention_days` and calls `PruneSteps(now − days)`. The recorder runs the same prune after a `Finish`, at most once per hour (mutex-guarded timestamp, injected clock). Prune failures are logged at warn.
  - **Replay helper** (test-only, `replay_test.go`): `reconstruct(db, runID) []llm.Request`. It loads the steps in `seq` order, concatenates the message deltas with the previous responses, and resolves the prompt hash and tools hash back to text.
- **Tests first**
  - Migration: columns and the table exist; old rows read back with nulls.
  - Meter:
    - a three-call chat-like run stores deltas of the expected lengths
    - a call with a shorter history resets the offset to 0
    - tools are stored once per hash
    - a 300 KiB message is truncated to valid JSON with `truncated:true`
    - an error call has a null response
  - **Reconstruction**, against a real SQLite test DB and the fake client:
    - a `lookup` run (translate plus pick-best through `food.Resolver`) reconstructs exactly the two `llm.Request`s the fake client recorded (`fake.Requests()`): system, messages, temperature, max tokens and tools, deep-equal
    - the same for an `import_fallback` run with the invalid-JSON retry (two calls, the second carrying the assistant reply and the correction)
    - each response content equals what the fake returned
  - Run identity: a chat run stores `conversation_id`; a run started inside an HTTP request with chi's RequestID middleware stores `request_id`.
  - Retention: steps of a run started 200 days ago are deleted with the default 180, the `ai_runs` row survives, and a 10-day-old run is untouched. The hourly throttle is tested with an injected clock.
- **Verify:** `cd backend && sqlc generate && go test -race ./internal/domain/airun/ ./internal/adapters/sqlite/ ./internal/domain/food/ ./internal/domain/importer/ ./internal/domain/agent/ -p 4`

### Track D: Docs

#### D1. Docs and design system

Last, after all other steps.

- **Files**
  - `backend/CLAUDE.md`: settings keys, availability, setup API, AI trace columns and retention, the migration list if one is mentioned
  - `frontend/CLAUDE.md`: route list (`/setup`, `/settings/$section`), remove `planner-skip.ts`, the components dirs (`availability/`, `setup/`), the e2e `setup` project and the stubbing rule (D22)
  - `DESIGN.md`:
    - `AvailabilityGrid` moves from *specified* to *implemented*
    - the guided-state cards
    - the reduced-motion and focus rules become *implemented*
    - the button legacy note is removed
    - GeneratePlanButton is no longer listed under Botanical leftovers
  - `docs/ARCHITECTURE.md`, where it describes skipped plates or settings keys
- **Verify:** `grep -rn "skipped\|planner-skip\|window_days\|Eating out" backend/CLAUDE.md frontend/CLAUDE.md DESIGN.md docs/ARCHITECTURE.md` returns nothing current (historical mentions in the specs are fine).

---

## 4. Waves and parallelism

The same hand-written file must never be edited by two parallel steps. Shared files:
- `sqlcgen/`: P1, A1, A2, S1, T1. Regenerate on conflict.
- `router.go` and `main.go`: A1 (wave 1); A2 and S1 (wave 2, different handlers and lines); T1 (wave 3, main only).
- `agent/service.go`: A2 (wave 2), then T1 (wave 3).
- `en.json`/`de.json`, `lib/queries/keys.ts` and `test/fixtures.ts`: most frontend steps. Each adds keys under its own namespace or fixture names, so merges are additive.
- `routes/index.tsx`: F2 (wave 2), then S2 (wave 3).
- `components/settings/*`: F3 (wave 2) only. Z1 touches none of them.
- `playwright.config.ts`: S2 only.

| Wave | Steps (parallel within a wave) | Notes |
|---|---|---|
| 0 | This plan | Docs PR |
| 1 | P1, A1, Z1, F1 | Four agents. P1 and A1 are both sqlc; regenerate when merging. F1 mocks the A1 API. |
| 2 | A2, S1, F2, F3 | A2 needs A1. S1 needs P1. F2 needs F1 and codes against A2's documented shape. F3 needs P1, A1 and F1. |
| 3 | T1, S2, R1 | T1 after A2 (agent service). S2 needs S1, F1 and F3. R1 needs S1 and F3. S2 and R1 share only i18n. |
| 4 | D1, then the definition-of-done check and the final reviews | |

**Final verification (DoD):**

```bash
cd backend && go test -race ./... && go vet ./... && golangci-lint run
cd frontend && bun run check && bun run build && bun run e2e
```

Then:
- A fresh-install walkthrough with an empty DB (backend on a spare port, `agent-browser`): `/` → `/setup` → Next × 7 → Finish → guided `/components` (0/10). Seed 10 composed recipes through the API, reload: the guided state collapses, the toast fires once, and Generate is enabled. Then the planner, with a No-plate cell set and reloaded.
- `go-reviewer` on the whole Phase 1 backend diff.
- `frontend-review` on `/setup`, `/components` (guided state) and Settings → Meals & week (availability), at 375, 820, 1180 and 1440 px in light and dark.

---

## 5. Out of Phase 1 (explicitly)

- **Phase 0 follow-ups not folded into a step:**
  - **Macro and chart palettes** (`index.css` `macro-*` and `chart-*`): their future is decided with the day meter and nutrition popover (UX §5.5) in Phase 2. No Phase 1 screen shows macros charts.
  - **Hardcoded `kcal`/`g` in existing screens** (about 25 files under `planner/`, `ingredients/`, `editorial/`, `import/`): new Phase 1 screens use i18n unit keys. Existing screens are fixed when Phases 2–3 rebuild them (board, Today, recipe library, editor).
  - **The `EmptyWeekCTA` radial-gradient wash and the planner's legacy aliases:** the planner board is rebuilt in Phase 2. F2 changes behaviour only.
- **Deferred from the Phase 0 list:**
  - **Diet class on foods** (§3.6; Phase 0 D6 pointed it at Phase 1) moves to **Phase 3**. Its consumers are the checker (H5), the candidate context and the library diet filter, and none exists in Phase 1. Phase 1 adds only `profile.diet_type`.
  - **Per-food count portions** (§10.4) move to **Phase 2**, with the scaled recipe view and the shopping list from sessions, which are their consumers.
- **Settings without a consumer yet:** `ai.price_overrides` (Phase 3) and `discovery.cuisines` (Phase 5). D4.
- **Real Anthropic key validation** and capability detection (Phase 5). D17.
- **The Discover onboarding card** (Phase 5). D18.
- **Applying routine presets to cells** (Phase 3 generator). D8.
- **`ai_runs` outcome labels** (Phase 3) and **`plate_events`** (Phase 2). D25.
- **Route renames** (`/recipes`, `/today`, `/plan`) and the Today, Shop and board redesigns (Phase 2). D1.
- **Routines editing inside Settings** (presets keep `/presets` until the Phase 2/3 board and plan wizard). D20.
- **A per-date availability view in the planner header**, bulk "Away all day" on the board, and "Save as my new default" (the Phase 3 plan wizard step 1). F1 ships the bulk-action props so Phase 3 only wires them.
