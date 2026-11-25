# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
bun run dev                           # dev server (port 5173, proxies /api → :8080)
bun run check                         # lint + typecheck + knip + legacy grep gate + unit tests
bun run check:legacy                  # scripts/check-legacy.sh: grep gate for removed Phase 2 patterns
bun run e2e                           # playwright (spawns its own backend + frontend; E2E_LANE=N for a parallel lane)
```

## Required Skills

Load these before writing or modifying the relevant code:

- **`frontend-design`** — creating/modifying frontend pages/components
- **`vercel-react-best-practices`** — React components
- **`vercel-composition-patterns`** — component API design / refactoring prop interfaces
- **`shadcn`** — shadcn/ui component usage and customization
- **`vitest`** — frontend unit tests
- **`playwright-best-practices`** — e2e tests

## Architecture

### Routing — TanStack Router (file-based)

Routes live in `src/routes/`. The router plugin auto-generates `src/routeTree.gen.ts` — **never edit it manually**. All routes are wrapped by `__root.tsx`, which mounts `AppShell` (rail + phone tab bar), renders the not-found page, and hosts the `CommandPalette` and `CopyFromWeekDialog`. `/setup` and cook mode (`/cook/…`, `/recipes/$id/cook`) render without the shell.

```
src/routes/
├── __root.tsx         # AppShell, CommandPalette, CopyFromWeekDialog
├── index.tsx          # `/` — redirects to /setup while `GET /api/setup` says not completed,
│                      #   else to /today (`lib/setup-redirect.ts`, shared with /today)
├── today/             # `/today?date=` — Today (components/today, view model lib/plan/today.ts)
├── setup/             # `/setup` — first-run SetupWizard
├── cook/              # `/cook/$sessionId` — cook mode for a planned batch (components/cook)
├── recipes/           # `/recipes?q&role&quick&stale&fav&new&sort&add` — the library (filters are search
│                      #   params, `lib/recipes/library.ts`; `add=url` opens the import sheet; `add=discover` the Discover sheet, and `add=discover&suggestion=<id>` the prefilled review of one suggestion); new.tsx,
│                      #   $id/ (detail, `?portions` starts its stepper), $id/edit.tsx, $id/cook.tsx
│                      #   (`?portions=`, cook mode without a plan, D30); ingredients/ — the
│                      #   Ingredients tab (table),
│                      #   ingredients/new.tsx, ingredients/$id/edit.tsx
├── plan/              # `/plan?start&cell&day&run` — the plan board (components/plan, view model
│                      #   lib/plan/board.ts); `day` focuses that day (Today's week strip); `run`
│                      #   is the generation the board follows (progress bar, D35); new.tsx:
│                      #   `/plan/new?start` — the plan wizard (components/plan-wizard), a dialog over
│                      #   the board from 768 px, a full page on phones; redirects to /recipes below
│                      #   the recipe target
├── shop/              # `/shop?from&to&drafts` — Shop (components/shop); without a range, the server's
│                      #   default for the anchored window; `drafts=1` includes draft plates
└── settings/          # index.tsx: `/settings` (section list on phone, else redirects);
                       #   $section.tsx: `/settings/$section` (household, meals, cooking, kitchen,
                       #   assistant, system — `settings-sections.ts`); assistant/insights.tsx:
                       #   `/settings/assistant/insights?run=` (static, beats $section; D37)
```

**Navigation** (`components/shell/primary-nav.ts`): four destinations — Today → `/today`, Plan → `/plan`, Recipes → `/recipes` (active on `/recipes*`), Shop → `/shop`. Settings sits at the bottom of the rail (≥768 px) and as a gear in the Today header on phones; the compact Generate button sits in the `/plan` header on phones (there is no top bar). ⌘K opens the `CommandPalette`: routines (apply to a day, with Undo), Copy a previous week, Manage routines (→ Settings → Meals & week) and the go-to entries (`command/palette-entries.ts`). Any other path renders the root not-found page with a link to Today.

### Data layer

**Server state — TanStack Query only.** Never mirror server data in a client store.

- `src/lib/api/` — one file per resource, all calls go through `apiFetch` in `lib/api/client.ts`. `ApiError` carries `status` + `messageKey` for i18n error display.
- `src/lib/queries/` — TanStack Query hooks + `keys.ts` (centralized key factories). Key hierarchies are intentional: invalidating a parent key also drops all child keys (e.g. invalidating `sessionKeys.detail(id)` also drops `sessionKeys.leftoverCells(id, count)`). **Each `useMutation` hook owns its own `invalidateQueries` (onSuccess) and `toastError` (onError) — call sites just call `mutate()` with no cache or error knowledge.**

- **Read models.** The planning screens read `usePlan(start?)` (`GET /api/plan`, key `planKeys.window`) and `useToday(date, today)` (`GET /api/today`, key `todayKeys.date`; `today` from `useTodayISO`, which rolls over at midnight and on becoming visible) — server-composed views, not plates assembled client-side. View models are pure functions of them (`lib/plan/board.ts` `buildBoard`, `lib/plan/today.ts`, `lib/plan/ledger.ts` `buildLedger`). The board's week ledger (`WeekLedger`, from `plan.week` + `plan.soft`) takes every in/out-of-range state from the server's soft issues (S3/S4/S5/S7), never from bands repeated in the client; it shows only what is off in the warning colour with an arrow and a word, and keeps hints on single days in a closed `<details>`. A day's light request (`light`, `light_slot`, `light_level`) shows as `LightMark` (a feather; a flame and "Hearty …" / "Deftig …" for `light_level: hearty`) and a `light: day` day is measured against its `target_kcal` (`dayMeterTargets`). `useShoppingList(range?)` reads `/api/shopping-list`.
- **Invalidation.** Every mutation that changes the plan returns `invalidatePlanViews(qc)` from `onSuccess` (`lib/queries/plan.ts`): it invalidates `planKeys.all`, `todayKeys.all` and `shoppingKeys.all` together, and returning the promise keeps the mutation pending until the active views refetch. Plate edits are not optimistic; the views refetch.
- **Undo.** Removals return `{removed}`; `useUndoableRemoval().offerUndo(res, verb)` shows the shared 6 s undo toast (`useUndoToast().offer(message, undo)` is the lower-level form) and Undo calls `POST /api/plan/restore` via `useRestorePlan`. Don't build per-screen undo toasts.
- **Meal actions and revert-token Undo (slice 5).** Every meal edit reaches `POST /api/plan/revert` through `useRevertUndo()` (`lib/queries/plan.ts`): `offerRevert(undo, message, description?)` shows the shared undo toast (the also-changed words as its description), Undo reverts once, and a 409 `error.plan.change_stale` shows its message; an empty token (`emptyUndo`) offers nothing, and `mergeUndo` spans a sequence of tokens (first to last event, every cell). It covers move, swap, replace, copy, move cook, away, regenerate, plate delete and the component add, change and remove calls. Hooks in `lib/queries/meal.ts` (one per endpoint, each owns its invalidation and `toastError`): `useMovePlate`, `useCopyPlate`, `useMoveCook`, `useSetAway`, `useAlternatives` (`{alternatives, empty_reason?}`, `mealKeys`) and `useApplyAlternative` (an alternative's ops as replace, add and remove calls in order, `alternativeCalls`, one Undo over the whole sequence; a `cook_at` row is exactly one replace that sends `cook_at` (an add sends it on the component add), a session row is exactly one replace that sends `session_id` (an add sends it as `cook_session_id`), so the position is kept; a call that fails part-way leaves the earlier ones committed, so the Undo covers that part under "Only part of the change was applied" and the error shows too; `usePlanDay` likewise reports "N of M meals set back to plan", with no Undo). Pure helpers in `lib/plan/meal-actions.ts`: `targetCells` (free or taken plan cells from today), `cookCells` (where a batch may cook: before its first meal, within keep days of its last), `lighterKcal`, `cellLabel`, `alsoChangedLines`, `emptyReasonText` (an empty list says why: the candidates and each rule that dropped them in plain words, `plan.alt.dropped.<rule>`, an unknown rule shows its name). **Dry-run preview.** `useAlsoChangedConfirm(plan, slotName)` (`components/plan/MealActions.tsx`) runs the call with `?dry_run=1` first (`movePlate(..., true)`, `setAway(..., true)`, `getRegeneratePreview`): an empty `also_changed` commits at once, else a dialog lists the other meals in plain words ("Lasagne will now be cooked Thu lunch") with Confirm and Cancel; a second `confirm` while a preview is in flight or the dialog is open is ignored. **Board actions.** `Board` owns drag and drop: a drop on a free cell moves with preview and Undo; on a taken cell it opens a menu at the target (Swap, Move X to… submenu of free cells, Replace X, Cancel); Ctrl/⌘-drag still copies through restore. The plate sheet has Move to… and Copy to… (`CellChoiceDialog`: a taken target asks Swap or Replace), "Something else", "Change main", "Change side" and "Add side, snack or dessert" (`AlternativesPanel`: ranked rows from `POST /api/plan/alternatives`, badges Adds a cooking, Eats a batch and "Cooked with Tue dinner" for `cook_at`, filter chips Quicker, Vegetarian, More protein, Lighter, one click applies). Keyboard R and S and the cell's hover quick actions open the same lists (`SheetState.alt`, `PlateSheet initialAlt`), never a planner run. `BatchSection` offers Cook on another day (`useMoveCook`; the backend's refusal is shown as a toast). The day header menu (`DayMenu`) has Away all day (preview, then a confirmed plate from the re-plan start on goes to `ReplanSheet` as before), Plan this day and Regenerate day (preview, then Undo once the run finishes, `undoRun`). No plate on one cell is away on that cell. Specs: `meal-move`, `meal-alternatives` and `meal-away` (all chromium: Away all day writes a cell for every slot, but in the spec's own `futureWindow()` band, and a revert skips a cell whose slot another spec deleted).
- **Review actions (drafts and confirmed).** Generated plates are drafts until confirmed (`lib/queries/plan.ts`). `useConfirm` / `useUnlock` act on a plate's or session's whole batch closure (Confirm offers Undo = unlock and closes the sheet; Unlock asks first through `UnlockDialog`, D34). `useAccept` confirms every draft of the window and its Undo calls `unaccept` with exactly the rows the accept returned; `useDiscard` deletes the window's drafts and its Undo is `restore` (the `{removed}` snapshot). Nothing accepted offers no Undo. `toastUndoError` turns a failed undo into a toast.
- **Where Undo does not exist.** A mid-week re-plan has **no Undo**: its removal, the No-plate overrides it writes and the new run cannot be reversed from `removed` alone. The plan wizard's No plate on an occupied cell offers one Undo (availability back, then `restorePlan`), but none when the removed plates share a cook session. Marking No plate on a confirmed plate from the re-plan start on opens the `ReplanSheet` (or `N`) instead of removing directly; drafts and earlier days keep the direct removal with Undo.
- **Generation runs.** `lib/queries/generation.ts`: `useGenerate({openBoard})` and `useRegenerate` start a run (202 `{run_id}`) and put `?run=<id>` on `/plan` (already on the board only `run` is added, so an anchored board keeps its URL; the wizard passes `openBoard: false` and follows the run itself). **`useRun(id)` polls** `GET /api/plan/runs/{id}` every 500 ms (`RUN_POLL_MS`) while `status` is `running`, stops on a final status and invalidates the plan views once when it first sees the run finished. There is no SSE (D9). `GenerationProgress` renders the stage; a reloaded board re-attaches through `running_run_id` on `GET /api/plan`. Assistant streaming (`assistantEvents`) is the only SSE left.
- **Plan wizard** (`/plan/new?start`, `components/plan-wizard`). Step 1 is the who-is-home grid (Away all day / Plan all day / Guests per day menu, which becomes `eaters_by_date`); "Make this my new default" writes the availability defaults and unticking reverts them. Step 2 is the optional instruction and "include a new recipe". It sends `chooser: heuristic` when settings show no AI provider. Below the recipe target it redirects to `/recipes` with a toast. `useApplyWindowAvailability` applies the grid; confirmed plates in cells set to No plate go through `ConfirmRemoval` first.

- **Recipe attributes** (`components/recipes/RecipeAttributes.tsx`, vocabularies in `lib/recipes/attributes.ts`). The recipe page shows every attribute as a chip (unknown ones dashed, user values with a dot); a chip's popover picks a value (heroes: up to three ingredients; taste: 0-3 per axis, saved together) or resets it to Automatic. `useSetFoodAttributes(id)` writes `PUT /api/foods/{id}/attributes`, puts the answer in the cache and offers the shared undo toast, whose Undo calls `POST /api/food-events/revert`. Shown for everyone (Chef switch off too, ON-1). Spec: `recipe-attributes`.
- **Discovery** (`components/discovery`, `lib/api/discovery.ts`, `lib/queries/discovery.ts`, `lib/discovery/available.ts`). `discoveryAvailable` is a plain boolean: true only when an AI provider resolves (false while settings load), and it gates every entry point (the Add menu's Discover item, the third onboarding card, the Today sheet, Settings cuisines). `useDailySuggestion(date, today)` yields `suggestion | running | none` (200 / 202 / 204) and `useDiscoveryRun(id)` polls `GET /api/discovery/runs/{id}` every 1 s. Components: `DailySuggestionSheet` (the daily idea over the real today only; closing remembers the day in `localStorage`, "Not for me" is the one thing that rejects; Add and reject dismiss first, with no refetch), `DiscoverSheet` (mode chips daily/explore/known, cuisine and focus inputs, results as `SuggestionCard`s), `SuggestionCard` (photo or typographic placeholder, facts, computed macros or "Nutrition incomplete", why, ingredients with a "New" mark and `NewLegend`, source link), `RejectReasons` (too long / ingredients / taste / no reason) and `SuggestionReview` (looks the suggestion up in the pending list, waits for the refetch, then opens `ImportWizard` with `initialDraft`, `initialRows` from `import/draft-rows.ts`, `extraTags`, `onSaved` → accept). The `page` fixture in `e2e/helpers.ts` answers `/api/discovery/daily*` with 204 by default; a real-backend spec calls `page.unroute("**/api/discovery/daily*")`.

**Client-only state — an external store.** Only the assistant one (no Zustand); everything else is local state, URL search params or `localStorage`.

- `src/lib/stores/assistant-stream.ts` — the assistant: streaming turn, status line, changes, unlock request, per-line undo state, conversation id and composer draft (open state lives in the URL).
- The assistant panel is `components/assistant/*` (`AssistantPanel`, `PanelShell`, `MessageList`, `Composer`, `ChangeLine`, `UnlockCard`, `StatusLine`, `SuggestionChips`, `LiveRegion`, history), mounted on `/plan`. **URL state:** `?assistant=1` opens it (a docked 380px aside from 1280px, a non-modal overlay sheet over the board from 768px, a bottom sheet below; a reload reopens the latest conversation); the `A` shortcut toggles it (`isAssistantShortcut`). Helpers in `lib/assistant/`: `change-text` (copy for `assistant.change.<text_key>`; an `apply_changes` line spells out each op, including the meal ops move/swap/drop/to, replace, away, move_cook and copy), `transcript`, `pulse` (`cellInChange`, which board cells flash: the change's plates and sessions plus exactly the cells its ops name: each op's `pulse` list (a move's source and target, away cells, copy targets, a cook's old and new cell) or its own `date` + `slot_id`; only a change whose ops name no cell, older rows, falls back to its dates), `chips`. `ChangeLine` shows the first three ops of a change and a "+N more" toggle (`aria-expanded`) that expands the full list. Hooks: `useAssistantStream` (the SSE turn, sends `start`), `useUndoChange`. A persisted change is matched to its tool call by the `change_id` in the call's tool_result (`transcript`; name matching only for results without one); a `change` with `recorded: false` (id 0) shows a "couldn't be saved" note and no Undo, and a 409 `change_not_undoable` ends the line in "Nothing to undo". The panel never shows raw tool names; it shows change lines with Undo, an unlock card for `unlock_request`, and the board cells of the latest change pulse (`data-pulse`). Specs: `assistant-panel.spec.ts` (ai-serial), `assistant-drafts.spec.ts` (global-settings).

### Forms

All forms use `react-hook-form` + `zod` via `@hookform/resolvers/zod`. Schema definitions live in `src/lib/schemas/`.

### Design system

Full reference: [`DESIGN.md`](../DESIGN.md); spec: `docs/UX_DESIGN_SPEC.md` §3. This section is the short summary. Tokens are hex CSS variables in `src/index.css` (`:root` + `.dark`), exposed to Tailwind via `@theme inline`. `src/test/tokens.test.ts` checks values and WCAG contrast.

Use these names in new and rebuilt code:

- Colour: `bg-ground` (page, green-neutral), `bg-panel` (cells, cards, sheets, the rail), `bg-panel-sunken` (second neutral layer, tags, hover fills), `text-ink`, `text-ink-muted`, `border-line`, `bg-accent` + `text-on-accent` (green: actions, selection, focus only), `bg-batch-1`…`bg-batch-6` and `bg-batch-N-tint` (batch lines, lane labels, pills and the cook-cell tint only), `text-success` / `text-warning` / `text-danger`.
- Type: `text-caption`, `text-small`, `text-body`, `text-h3`, `text-h2`, `text-h1`. Figtree is the only face (`font-sans`, upright and italic); `font-heading` is a role alias that also resolves to Figtree. Hierarchy comes from size and weight. Numbers use `tabular-nums`. Sentence case, no all-caps labels.
- Shape: `rounded-cell` (10 px: cells, cards, inputs), `rounded-xl` (panels above the board), `rounded-sheet` (16 px), `rounded-tag` (batch pill, full round); buttons keep shadcn's `--radius` (8 px). `shadow-card` is the 1 px whisper under cards and panels; `shadow-sheet` is for sheets and popovers.

**shadcn names.** `components/ui` reads the shadcn theme variables (`--background`, `--primary`, `--muted`, `--border`, …); `index.css` maps each onto a design token (table in its comment). App code uses the design token names. The Botanical Atelier aliases, macro and chart palettes and utilities are gone: `src/test/tokens.test.ts` allowlists every custom property `index.css` defines, and `scripts/check-legacy.sh` (in `bun run check`) fails on the Phase 2 legacy patterns in `src/` and `e2e/`. Nutrition reads as one line of text: `editorial/MacroLine` (kcal/P/F/C, `units.*` keys).

**Gotcha:** shadcn's `--accent` is the same variable as the design accent (solid green), so `bg-accent` (including shadcn menu/select highlights) needs `text-on-accent` / `text-accent-foreground` for text, never `text-ink`.

### Dead code and legacy gates

`bun run check` also runs `knip` (config `knip.json`; entry points are the route files, so an unused file, export or dependency fails the run; `components/ui/**` exports are exempt) and `scripts/check-legacy.sh`, which greps `src/` and `e2e/` for removed Phase 2 patterns (old routes, week/template names, Material and Botanical utilities). Remove the dead code instead of allowlisting it; if a legacy pattern is a legitimate new use, change the pattern list in the script deliberately.

### i18n

All user-visible strings go through `react-i18next`. Keys live in `src/lib/i18n/`. Use `useTranslation()` in components; never hardcode English strings (units too: `units.kcal`, `units.g`). `src/lib/i18n/keys.test.ts` fails when en/de key sets differ or a key is not referenced in `src/` (a string literal, a template-literal head such as `` `unit.${key}` ``, or a prefix in `DYNAMIC_PREFIXES`, the allowlist for keys composed from API data: `error.` for backend message keys and `slot.` for slot `name_key`s). Add a prefix there only for keys that really are built at runtime. `src/lib/hooks/useWakeLock` keeps the screen on (cook mode, in-store mode).

### Utility modules

`src/lib/domain/` — domain-logic modules shared across features (each has a sibling `*.test.ts`):

- `domain/nutrition.ts` — calorie/macro calculation from food portions
- `domain/units.ts` — unit conversion (g ↔ oz, ml ↔ fl oz, etc.)
- `domain/assistantEvents.ts` — SSE event types for the assistant stream

Loose `*.ts` files in `src/lib/` are planner-specific utilities with their own unit tests:

- `dates.ts` — `todayISO` (the client's date, sent as `today`, D25) and `shiftYMD`
- `slot-label.ts` — slot display names
- `availability-grid.ts` — AvailabilityGrid columns and value ↔ availability defaults mapping
- `nutrition-presets.ts` — cut / maintain / bulk nutrition target presets (frontend only)

## Testing

### Vitest (unit)

- `vite.config.ts` caps `maxWorkers: 4` (memory: the default jsdom worker per core exhausted WSL when runs overlapped). Don't raise it.
- Mock at `@/lib/api/*` module level — TanStack Query works normally with mocked fetch fns.
- Use `renderWithRouter` from `@/test/render` — wraps in `QueryClientProvider` (retry: false) + `RouterProvider` (createMemoryHistory).
- Use shared fixtures from `@/test/fixtures` — don't duplicate mock data per test file.
- Use `screen.findBy*` (async) — `RouterProvider` renders async.

### Playwright (e2e)

- `bun run e2e` is fully self-contained: playwright spawns the backend (`go run ./cmd/plantry` with test env vars, always fresh) and a Vite dev server. `reuseExistingServer` is false for both, so a leftover server on the port makes the run fail rather than be reused.
- **Lanes.** `E2E_LANE=N bun run e2e` (N = 0–9) gives a run its own backend port (8080+N), Vite port (5173+N), DB (`/tmp/plantry-e2e-N.db`) and image dir, so several worktrees can run e2e at once. Lane 0 (unset) uses 8080/5173. Parallel agents each take their own lane and never use lane 0.
- **Worktrees need their own `bun install`** (and `GOWORK=off` for the backend `go run`). The repo-root `node_modules` symlink points at the main checkout, so Vite refuses its font files (403) and `smoke` plus the German 375 px `ingredients-table` spec fail; because `global-settings` depends on `chromium`, its specs then show "did not run".
- The e2e backend runs with `PLANTRY_OFF_DISABLED=1` (no Open Food Facts calls) and the `fake` AI provider, so runs are hermetic.
- Local runs use `workers: 6` (chromium takes 4, the serial `ai-serial` and `global-settings` run beside it), CI `workers: 4` (its runner has 4 vCPUs); don't raise them.
- Self-contained seeding: call the backend API directly through `API` from `e2e/helpers.ts`, never a literal `localhost:8080`. `API` is a live binding the worker fixture `apiBinding` sets to the project's backend (`apiPort` option: lane-aware, and global-settings has its own server), so read it inside tests and helpers, never at a spec's top level. Cleanup in `finally`. Always bypass the Vite proxy for seeding.
- Unique data: append `crypto.randomUUID().slice(0,8)` to names.
- After form submit: `waitForResponse` on the specific API call. Never `waitForURL`, never `waitForTimeout`.
- For elements absent from DOM (server-filtered): use `toHaveCount(0)`, never `not.toBeVisible()` (causes timeouts).
- Projects: `setup` runs `setup-wizard.spec.ts` first on the fresh DB (completes the wizard, sets `plan.anchor = today`); `chromium` and `ai-serial` depend on it. `global-settings` (specs that write the profile or plan settings through the UI, such as `plan-anchor-settings.spec.ts`, and specs that run a real generation, which plans every active slot of its window from the shared recipes, such as `plan-wizard.spec.ts`) has its **own backend, Vite and DB** (ports `8280+N` / `5273+N`, `/tmp/plantry-e2e-global-N.db`): `setup-global` walks the wizard there, then its specs run one at a time beside `chromium` and `ai-serial` (it also depends on `fresh-install`: Playwright starts a project only after every project of the level before it). They used to run last, alone, as a serial 56 s. Never run with `--no-deps`.
- Plan board specs work in `futureWindow()` (`e2e/helpers.ts`): each parallel worker owns its own band of future windows, so one test's Clear this week or No plate overrides never touch another's plates.
- Settings are global: `setSetting` (`PUT /api/settings/{key}`) changes them for every running spec, so parallel specs override values page-scoped with `mockSettingValues` and restore anything they write.
- The shared DB accumulates recipes, so a spec that needs the library below or at the 10-recipe target stubs `GET /api/setup` with `page.route` (see `recipes-guided.spec.ts`), never relies on the real count.
- **Real generation goes in `global-settings`.** Any spec that runs an actual generation (heuristic or LLM) plans every active slot from the shared recipes and would race parallel specs, so it belongs in `GLOBAL_SETTINGS_SPECS` (`playwright.config.ts`): `plan-generate-heuristic`, `plan-review-llm`, `plan-wizard`, `plan-replan`, `assistant-drafts`, `settings-insights-generate`. **Always pin `chooser`** in the request: the e2e backend always has the fake provider, and a request without `chooser` defaults to `llm`. Seed enough recipes (about 10 mains; `seedRecipeLibrary` in `e2e/helpers.ts`) and work far ahead (`plan-replan` uses a window 88 weeks out with a pinned clock).
- **LLM e2e pattern** (`plan-review-llm.spec.ts`): the fake client replays a script. Read `e2e/fixtures/chat-scripts/generate-week.json` (a template; placeholders are described in its `_comment`), fill it for the window, write it to a **unique tmp path** (the fake client is cached per path), `PUT` the `ai.fake_script` setting to that path, run, then delete the `ai.fake_script` setting. Each generation replays turn 1 (an H2 violation) and then turn 2 (a valid plan), so it ends `ok` after one repair; add `delay_ms` to a turn to observe progress; `generate-week-error.json` is an empty script, so every call fails. Override the cells so exactly the slots you expect are Plan (the e2e DB keeps leftover slots). Count a run's LLM calls with `GET /api/ai/runs/{id}` steps (`repair_attempts` is not in any API). Heuristic-only specs set `ai.provider` to empty through `mockSettingValues` instead.
- Specs that mutate global AI state (`assistant-panel.spec.ts`, `settings-insights.spec.ts`, `discovery.spec.ts`, etc.) run under the `ai-serial` project with `workers: 1` (`AI_SERIAL_SPECS` in `playwright.config.ts`). Never add AI-touching assertions to a parallel spec — it will cause flaky failures. A discovery spec that also generates a plan (`discovery-explore-plan.spec.ts`) belongs in `GLOBAL_SETTINGS_SPECS` instead.
- **Discovery fixture site (no network).** `playwright.config.ts` starts a third `webServer`, `bun e2e/fixtures/discovery-site/serve.ts`, on port `9100 + E2E_LANE` (`E2E_SITE_PORT`). It serves the recipe pages and images (`fesenjan.html`, `pasta.html`, `img/`) the fake research client points at; `discovery.json` also lists a `broken.html` (404) and an unresolvable host, so the backend's real `httpfetch`, JSON-LD parser, image store and URL guard run; the e2e backend sets `PLANTRY_DEV_MODE=1` so the guard lets the loopback fixture through. `e2e/discovery-fixture.ts` fills the `{{SITE}}` and `{{RUN}}` placeholders in `e2e/fixtures/chat-scripts/discovery.json`, writes the script to a unique path and sets `ai.fake_script` (delete it afterwards). Discovery turns use `match_user` per mode ("persian", "thai", "chickpeas"), `search_results`, `web_search_requests`, `stop_reason: "pause_turn"` and `capabilities.web_search`. "No provider" is page-mocked with `mockSettingValues`, since the e2e backend always has the fake provider. Never call a real provider or the real web.
- **Assistant specs: never open the panel with `?assistant=1`.** A page that loads with the panel open restores the latest conversation of the shared DB (another spec's, or the previous `--repeat-each` run's), so its change lines count as the test's own and the switch can land mid-turn. Load `/plan?start=...` and click "Ask assistant" (a fresh conversation), and wait for the scripted closing reply to be visible (`assistant-move.spec.ts`); an absent abort button alone also holds before the stream starts and after it failed.
- Stable = `--repeat-each=10 --workers=4` with zero failures.

## Gotchas

### React / ESLint

- **No ref access during render** (`react-hooks/refs`). Reset derived state in event handlers instead.
- **No setState in effects** (`react-hooks/set-state-in-effect`). Move state resets to event handlers.
- **Prefer `useDeferredValue`** over `useState`+`useEffect` debounce.
- `Date.now()` / `new Date()` during render is flagged by the React compiler as impure — isolate into a `useState` initializer or a separate component.

### Units vocabulary

`domain/units.ts` is not self-initializing. It must be seeded at app boot via `initUnitsVocabulary(units)` in `__root.tsx` (called from a `useEffect` on the `useUnits()` query result). Any component that calls unit-conversion helpers before this resolves will get empty conversions.

### shadcn

- `FormControl` uses `Slot.Root` (radix-ui) so id/aria forward to the child input. `useFormField` reads id from `FormItemContext`.
- `src/components/ui/` is generated — prefer wrapping over editing primitives directly.
