# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
bun install                           # install deps
bun run dev                           # dev server (port 5173, proxies /api → :8080)
bun run build                         # production build → dist/
bun run typecheck                     # tsc --noEmit
bun run lint                          # eslint
bun run format                        # prettier (ts/tsx)
bun run test                          # vitest (unit, run once)
bun run test:watch                    # vitest watch mode
bun run e2e                           # playwright (spawns its own backend + frontend; E2E_LANE=N for a parallel lane)
bun run check                         # lint + typecheck + knip + legacy grep gate + unit tests
bun run knip                          # unused files, exports, dependencies (knip.json)
bun run check:legacy                  # scripts/check-legacy.sh: grep gate for removed Phase 2 patterns
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
│                      #   params, `lib/recipes/library.ts`; `add=url` opens the import sheet); new.tsx,
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
                       #   $section.tsx: `/settings/$section` (household, meals, cooking,
                       #   assistant, system — `settings-sections.ts`); assistant/insights.tsx:
                       #   `/settings/assistant/insights?run=` (static, beats $section; D37)
```

**Navigation** (`components/shell/primary-nav.ts`): four destinations — Today → `/today`, Plan → `/plan`, Recipes → `/recipes` (active on `/recipes*`), Shop → `/shop`. Settings sits at the bottom of the rail (≥768 px) and as a gear in the Today header on phones; the compact Generate button sits in the `/plan` header on phones (there is no top bar). ⌘K opens the `CommandPalette`: routines (apply to a day, with Undo), Copy a previous week, Manage routines (→ Settings → Meals & week) and the go-to entries (`command/palette-entries.ts`). Any other path renders the root not-found page with a link to Today.

### Components

```
src/components/
├── ui/                # shadcn primitives (never hand-edit generated files)
├── shell/             # AppShell, SideNav, MobileBottomNav, GeneratePlanButton, primary-nav.ts
├── today/             # TodayPage, Verdict (header + headline), CookCard/LeftoverCard, MealRow,
│                      #   Reminders, RateQuestion, PlanBanner, WeekStrip
├── shop/              # ShopPage, RangePicker, SectionList, ShoppingRow, CopyAsText (clipboard text);
│                      #   ticks in localStorage `plantry:purchased:range:{from}:{to}`
├── cook/              # CookMode (header, tabs; side by side ≥768 px), ScaledIngredients, StepPager,
│                      #   TimerPill, useSessionPortions (debounced session write);
│                      #   CookPages (session and recipe containers); timer detection in lib/cook/timers.ts
├── plan/              # the board: PlanPage, Board (BoardGrid ≥768 px, DayList phone), PlateCell,
│                      #   PlanToolbar, EmptyWindow, BoardSkeleton, ShortcutsDialog, CopyFromWeekDialog; keynav in lib/plan/keynav.ts;
│                      #   PlanSheet (one sheet per cell: PlateSheet, CellSheet or FoodPicker), BatchSection,
│                      #   QuantityStepper; review (D33–D35): UnlockDialog, DiscardDialog, DraftBar (phone),
│                      #   GenerationProgress; ReplanSheet (mid-week re-plan, UX §5.10: overflow
│                      #   "Re-plan from today" on the current window, and Mark as No plate on a
│                      #   confirmed plate from today on)
├── batch/             # BatchBadge, BatchHighlightProvider (hooks in batch-highlight.ts), CookStrip,
│                      #   DayMeter, NutritionPopover; pure helpers in lib/plan/ (batch-colours, format)
├── plan-wizard/       # PlanWizard (`/plan/new`): WhoIsHome (window grid, day menu Away all day /
│                      #   Guests, "Make this my new default"), AnythingSpecial, Generating (run
│                      #   stages, failure + Plan without the assistant), ConfirmRemoval (No plate on
│                      #   confirmed plates asks first); GeneratePlanButton opens it
├── availability/      # AvailabilityGrid (weekday/date × slot modes), RoutinePicker
├── setup/             # SetupWizard and its steps; wizard-draft.ts, use-wizard-save.ts
├── component/         # shared input widget: QuantityUnitInput (leaf amount + unit)
├── recipes/           # RecipeLibrary, RecipeCard, RecipesHeader (tabs), AddRecipeMenu, ImportSheet,
│                      #   RecipeOnboarding, RecipeEditor, RecipeDetail, NutritionCompletenessWarning
│                      #   (shared by editor, detail and import)
├── import/            # ImportWizard (hosted by ImportSheet), IngredientResolveRow, ImportNutritionSummary
├── ingredients/       # ingredient CRUD + lookup: IngredientEditor, IngredientTable, LookupPanel
├── editorial/         # design system editorial primitives
├── chat/              # AI chat panel
├── command/           # CommandPalette (cmdk)
├── images/            # image crop/upload UI
└── settings/          # SettingsShell, sections/ (one per section), controls.tsx (Stepper, Switch,
                       #   NumberField, OptionCards, SettingsGroup, SavedNote), HouseholdHelper,
                       #   AIProviderControls (shared with the setup wizard), SourceBadge;
                       #   routines/ (RoutinesEditor in Meals & week: RoutineCard, RoutineSheet, RoutineRenameDialog);
                       #   PricesEditor (Assistant → Prices, `ai.price_overrides`); insights/ (InsightsPage,
                       #   RunsTable, RunDetailSheet, Sparkline — hand-written SVG, VersionTable)
```

### Data layer

**Server state — TanStack Query only.** Never mirror server data in Zustand.

- `src/lib/api/` — one file per resource, all calls go through `apiFetch` in `lib/api/client.ts`. `ApiError` carries `status` + `messageKey` for i18n error display.
- `src/lib/queries/` — TanStack Query hooks + `keys.ts` (centralized key factories). Key hierarchies are intentional: invalidating a parent key also drops all child keys (e.g. invalidating `sessionKeys.detail(id)` also drops `sessionKeys.leftoverCells(id, count)`). **Each `useMutation` hook owns its own `invalidateQueries` (onSuccess) and `toastError` (onError) — call sites just call `mutate()` with no cache or error knowledge.**

- **Read models.** The planning screens read `usePlan(start?)` (`GET /api/plan`, key `planKeys.window`) and `useToday(date, today)` (`GET /api/today`, key `todayKeys.date`; `today` from `useTodayISO`, which rolls over at midnight and on becoming visible) — server-composed views, not plates assembled client-side. View models are pure functions of them (`lib/plan/board.ts` `buildBoard`, `lib/plan/today.ts`). `useShoppingList(range?)` reads `/api/shopping-list`.
- **Invalidation.** Every mutation that changes the plan returns `invalidatePlanViews(qc)` from `onSuccess` (`lib/queries/plan.ts`): it invalidates `planKeys.all`, `todayKeys.all` and `shoppingKeys.all` together, and returning the promise keeps the mutation pending until the active views refetch. Plate edits are not optimistic; the views refetch.
- **Undo.** Removals return `{removed}`; `useUndoableRemoval().offerUndo(res, verb)` shows the shared 6 s undo toast (`useUndoToast().offer(message, undo)` is the lower-level form) and Undo calls `POST /api/plan/restore` via `useRestorePlan`. Don't build per-screen undo toasts.
- **Review actions (drafts and confirmed).** Generated plates are drafts until confirmed (`lib/queries/plan.ts`). `useConfirm` / `useUnlock` act on a plate's or session's whole batch closure (Confirm offers Undo = unlock and closes the sheet; Unlock asks first through `UnlockDialog`, D34). `useAccept` confirms every draft of the window and its Undo calls `unaccept` with exactly the rows the accept returned; `useDiscard` deletes the window's drafts and its Undo is `restore` (the `{removed}` snapshot). Nothing accepted offers no Undo. `toastUndoError` turns a failed undo into a toast.
- **Where Undo does not exist.** A mid-week re-plan has **no Undo**: its removal, the No-plate overrides it writes and the new run cannot be reversed from `removed` alone. The plan wizard's No plate on an occupied cell offers one Undo (availability back, then `restorePlan`), but none when the removed plates share a cook session. Marking No plate on a confirmed plate from the re-plan start on opens the `ReplanSheet` (or `N`) instead of removing directly; drafts and earlier days keep the direct removal with Undo.
- **Generation runs.** `lib/queries/generation.ts`: `useGenerate({openBoard})` and `useRegenerate` start a run (202 `{run_id}`) and put `?run=<id>` on `/plan` (already on the board only `run` is added, so an anchored board keeps its URL; the wizard passes `openBoard: false` and follows the run itself). **`useRun(id)` polls** `GET /api/plan/runs/{id}` every 500 ms (`RUN_POLL_MS`) while `status` is `running`, stops on a final status and invalidates the plan views once when it first sees the run finished. There is no SSE (D9). `GenerationProgress` renders the stage; a reloaded board re-attaches through `running_run_id` on `GET /api/plan`. Chat streaming (`chatEvents`) is the only SSE left.
- **Plan wizard** (`/plan/new?start`, `components/plan-wizard`). Step 1 is the who-is-home grid (Away all day / Plan all day / Guests per day menu, which becomes `eaters_by_date`); "Make this my new default" writes the availability defaults and unticking reverts them. Step 2 is the optional instruction and "include a new recipe". It sends `chooser: heuristic` when settings show no AI provider. Below the recipe target it redirects to `/recipes` with a toast. `useApplyWindowAvailability` applies the grid; confirmed plates in cells set to No plate go through `ConfirmRemoval` first.

**Client-only state — Zustand stores.** Only the chat ones; everything else is local state, URL search params or `localStorage`.

- `src/lib/stores/chat-ui.ts` — chat panel open/closed state and the composer draft.
- `src/lib/stores/chat-stream.ts` — streaming AI message state.

### Forms

All forms use `react-hook-form` + `zod` via `@hookform/resolvers/zod`. Schema definitions live in `src/lib/schemas/`.

### Design system

**Cookbook** — full reference: [`DESIGN.md`](../DESIGN.md); spec: `docs/UX_DESIGN_SPEC.md` §3. This section is the short summary. Tokens are hex CSS variables in `src/index.css` (`:root` + `.dark`), exposed to Tailwind via `@theme inline`. `src/test/tokens.test.ts` checks values and WCAG contrast.

Use these names in new and rebuilt code:

- Colour: `bg-ground` (page), `bg-panel` (cells, sheets, cards), `bg-panel-sunken` (rail, toolbars), `text-ink`, `text-ink-muted`, `border-line`, `bg-accent` + `text-on-accent`, `bg-batch-1`…`bg-batch-6` and `bg-batch-N-tint`, `text-success` / `text-warning` / `text-danger`.
- Type: `text-caption`, `text-small`, `text-body`, `text-h3`, `text-h2`, `text-h1`. Body/UI font is Source Sans 3 (`font-sans`, the default); `font-heading` is Newsreader, for display only (page titles, recipe names, headlines) — never labels, buttons, inputs or data. `h1`–`h6` get Newsreader from the base layer. Numbers use `tabular-nums`. Sentence case, no all-caps labels.
- Shape: `rounded-cell` (4 px), `rounded-sheet` (16 px), `rounded-tag` (batch badge); buttons keep shadcn's `--radius` (8 px). `shadow-sheet` is the only shadow, for sheets and popovers.

**shadcn names.** `components/ui` reads the shadcn theme variables (`--background`, `--primary`, `--muted`, `--border`, …); `index.css` maps each onto a Cookbook token (table in its comment). App code uses the Cookbook names. The Botanical Atelier aliases, macro and chart palettes and utilities are gone: `src/test/tokens.test.ts` allowlists every custom property `index.css` defines, and `scripts/check-legacy.sh` (in `bun run check`) fails on the Phase 2 legacy patterns in `src/` and `e2e/`. Nutrition reads as one line of text: `editorial/MacroLine` (kcal/P/F/C, `units.*` keys).

**Gotcha:** shadcn's `--accent` is the same variable as the Cookbook accent (solid ink blue), so `bg-accent` (including shadcn menu/select highlights) needs `text-on-accent` / `text-accent-foreground` for text, never `text-ink`.

### Dead code and legacy gates

`bun run check` also runs `knip` (config `knip.json`; entry points are the route files, so an unused file, export or dependency fails the run; `components/ui/**` exports are exempt) and `scripts/check-legacy.sh`, which greps `src/` and `e2e/` for removed Phase 2 patterns (old routes, week/template names, Material and Botanical utilities). Remove the dead code instead of allowlisting it; if a legacy pattern is a legitimate new use, change the pattern list in the script deliberately.

### i18n

All user-visible strings go through `react-i18next`. Keys live in `src/lib/i18n/`. Use `useTranslation()` in components; never hardcode English strings (units too: `units.kcal`, `units.g`). `src/lib/i18n/keys.test.ts` fails when en/de key sets differ or a key is not referenced in `src/` (a string literal, a template-literal head such as `` `unit.${key}` ``, or a prefix in `DYNAMIC_PREFIXES`, the allowlist for keys composed from API data: `error.` for backend message keys and `slot.` for slot `name_key`s). Add a prefix there only for keys that really are built at runtime. `src/lib/hooks/useWakeLock` keeps the screen on (cook mode, in-store mode).

### Utility modules

`src/lib/domain/` — domain-logic modules shared across features (each has a sibling `*.test.ts`):

- `domain/nutrition.ts` — calorie/macro calculation from food portions
- `domain/units.ts` — unit conversion (g ↔ oz, ml ↔ fl oz, etc.)
- `domain/chatEvents.ts` — SSE event parsing for the AI chat stream

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
- The e2e backend runs with `PLANTRY_OFF_DISABLED=1` (no Open Food Facts calls) and the `fake` AI provider, so runs are hermetic.
- Local runs use `workers: 4` (CI: 1) to cap memory; don't raise it.
- Self-contained seeding: call the backend API directly through the `API` constant from `e2e/helpers.ts` (lane-aware: `http://localhost:${E2E_API_PORT ?? 8080}`), never a literal `localhost:8080`. Cleanup in `finally`. Always bypass the Vite proxy for seeding.
- Unique data: append `crypto.randomUUID().slice(0,8)` to names.
- After form submit: `waitForResponse` on the specific API call. Never `waitForURL`, never `waitForTimeout`.
- For elements absent from DOM (server-filtered): use `toHaveCount(0)`, never `not.toBeVisible()` (causes timeouts).
- Projects: `setup` runs `setup-wizard.spec.ts` first on the fresh DB (completes the wizard, sets `plan.anchor = today`); `chromium` and `ai-serial` depend on it. `global-settings` (specs that write the profile or plan settings through the UI, such as `plan-anchor-settings.spec.ts`, and specs that run a real generation, which plans every active slot of its window from the shared recipes, such as `plan-wizard.spec.ts`) runs last and alone, after both. Never run with `--no-deps`.
- Plan board specs work in `futureWindow()` (`e2e/helpers.ts`): each parallel worker owns its own band of future windows, so one test's Clear this week or No plate overrides never touch another's plates.
- Settings are global: `setSetting` (`PUT /api/settings/{key}`) changes them for every running spec, so parallel specs override values page-scoped with `mockSettingValues` and restore anything they write.
- The shared DB accumulates recipes, so a spec that needs the library below or at the 10-recipe target stubs `GET /api/setup` with `page.route` (see `recipes-guided.spec.ts`), never relies on the real count.
- **Real generation goes in `global-settings`.** Any spec that runs an actual generation (heuristic or LLM) plans every active slot from the shared recipes and would race parallel specs, so it belongs in `GLOBAL_SETTINGS_SPECS` (`playwright.config.ts`): `plan-generate-heuristic`, `plan-review-llm`, `plan-wizard`, `plan-replan`. **Always pin `chooser`** in the request: the e2e backend always has the fake provider, and a request without `chooser` defaults to `llm`. Seed enough recipes (about 10 mains; `seedRecipeLibrary` in `e2e/helpers.ts`) and work far ahead (`plan-replan` uses a window 88 weeks out with a pinned clock).
- **LLM e2e pattern** (`plan-review-llm.spec.ts`): the fake client replays a script. Read `e2e/fixtures/chat-scripts/generate-week.json` (a template; placeholders are described in its `_comment`), fill it for the window, write it to a **unique tmp path** (the fake client is cached per path), `PUT` the `ai.fake_script` setting to that path, run, then delete the `ai.fake_script` setting. Each generation replays turn 1 (an H2 violation) and then turn 2 (a valid plan), so it ends `ok` after one repair; add `delay_ms` to a turn to observe progress; `generate-week-error.json` is an empty script, so every call fails. Override the cells so exactly the slots you expect are Plan (the e2e DB keeps leftover slots). Count a run's LLM calls with `GET /api/ai/runs/{id}` steps (`repair_attempts` is not in any API). Heuristic-only specs set `ai.provider` to empty through `mockSettingValues` instead.
- Specs that mutate global AI state (`ai-chat.spec.ts`, `settings-insights.spec.ts`, etc.) run under the `ai-serial` project with `workers: 1`. Never add AI-touching assertions to a parallel spec — it will cause flaky failures.
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
