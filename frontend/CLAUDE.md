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
bun run e2e                           # playwright (auto-spawns backend + frontend via webServer)
bun run check                         # lint + typecheck + unit tests
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

Routes live in `src/routes/`. The router plugin auto-generates `src/routeTree.gen.ts` — **never edit it manually**. All routes are wrapped by `__root.tsx`, which mounts `AppShell` (rail + phone tab bar), renders the not-found page, and hosts the `CommandPalette` and `CopyFromWeekDialog`. `/setup` renders without the shell.

```
src/routes/
├── __root.tsx         # AppShell, CommandPalette, CopyFromWeekDialog
├── index.tsx          # `/` — redirects to /setup while `GET /api/setup` says not completed,
│                      #   else to /today (`lib/setup-redirect.ts`, shared with /today)
├── today/             # `/today?date=` — Today (components/today, view model lib/plan/today.ts)
├── setup/             # `/setup` — first-run SetupWizard
├── components/        # recipe library; new.tsx, $id/ (detail), $id/edit.tsx
├── ingredients/       # food/ingredient CRUD; new.tsx, $id/edit.tsx
├── import/            # recipe import from URL (ImportWizard)
├── plan/              # `/plan?start&cell&day` — the plan board (components/plan, view model
│                      #   lib/plan/board.ts); `day` focuses that day (Today's week strip)
├── shop/              # `/shop?from&to` — Shop (components/shop); without a range, the server's
│                      #   default for the anchored window
├── presets/           # preset library + editor
└── settings/          # index.tsx: `/settings` (section list on phone, else redirects; maps legacy
                       #   `?tab=`); $section.tsx: `/settings/$section` (household, meals, cooking,
                       #   assistant, system — `settings-sections.ts`)
```

**Navigation** (`components/shell/primary-nav.ts`): four destinations — Today → `/today`, Plan → `/plan`, Recipes → `/components` (also active on `/ingredients*` and `/import`, which are linked from the recipe list header), Shop → `/shop`. Settings sits at the bottom of the rail (≥768 px) and as a gear in the Today header on phones; the compact Generate button sits in the `/plan` header on phones (there is no top bar). Presets is not in the nav; it is reachable via ⌘K (`CommandPalette`). Unknown paths (including the removed `/calendar`, `/day/…`, `/archive`) render the root not-found page with a link to Today.

### Components

```
src/components/
├── ui/                # shadcn primitives (never hand-edit generated files)
├── shell/             # AppShell, SideNav, MobileBottomNav, GeneratePlanButton, primary-nav.ts
├── today/             # TodayPage, Verdict (header + headline), CookCard/LeftoverCard, MealRow,
│                      #   Reminders, RateQuestion, PlanBanner, WeekStrip
├── shop/              # ShopPage, RangePicker, SectionList, ShoppingRow, CopyAsText (clipboard text);
│                      #   ticks in localStorage `plantry:purchased:range:{from}:{to}`
├── plan/              # the board: PlanPage, Board (BoardGrid ≥768 px, DayList phone), PlateCell,
│                      #   PlanToolbar, EmptyWindow, BoardSkeleton, ShortcutsDialog; keynav in lib/plan/keynav.ts;
│                      #   PlanSheet (one sheet per cell: PlateSheet, CellSheet or FoodPicker), BatchSection,
│                      #   QuantityStepper
├── batch/             # BatchBadge, BatchHighlightProvider (hooks in batch-highlight.ts), CookStrip,
│                      #   DayMeter, NutritionPopover; pure helpers in lib/plan/ (batch-colours, format)
├── availability/      # AvailabilityGrid (weekday/date × slot modes), RoutinePicker
├── setup/             # SetupWizard and its steps; wizard-draft.ts, use-wizard-save.ts
├── presets/           # preset library, editor, CopyFromWeekDialog
├── component/         # shared input widget: QuantityUnitInput (leaf amount + unit)
├── components/        # food component CRUD: ComponentEditor, ComponentList, ComponentCard, RecipeDetail,
│                      #   NutritionCompletenessWarning (shared by editor, detail and import)
├── import/            # ImportWizard, IngredientResolveRow, ImportNutritionSummary
├── ingredients/       # ingredient CRUD + lookup: IngredientEditor, IngredientList, LookupPanel
├── editorial/         # design system editorial primitives
├── chat/              # AI chat panel
├── command/           # CommandPalette (cmdk)
├── images/            # image crop/upload UI
└── settings/          # SettingsShell, sections/ (one per section), controls.tsx (Stepper, Switch,
                       #   NumberField, OptionCards, SettingsGroup, SavedNote), HouseholdHelper,
                       #   AIProviderControls (shared with the setup wizard), SourceBadge
```

### Data layer

**Server state — TanStack Query only.** Never mirror server data in Zustand.

- `src/lib/api/` — one file per resource, all calls go through `apiFetch` in `lib/api/client.ts`. `ApiError` carries `status` + `messageKey` for i18n error display.
- `src/lib/queries/` — TanStack Query hooks + `keys.ts` (centralized key factories). Key hierarchies are intentional: invalidating a parent key also drops all child keys (e.g. invalidating `sessionKeys.detail(id)` also drops `sessionKeys.leftoverCells(id, count)`). **Each `useMutation` hook owns its own `invalidateQueries` (onSuccess) and `toastError` (onError) — call sites just call `mutate()` with no cache or error knowledge.**

**Client-only state — Zustand stores.**

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

**Legacy aliases (plan decision D2).** The shadcn variables (`--background`, `--primary`, `--muted`, `--border`, …) and the old Material utilities (`bg-surface*`, `text-on-surface*`, `border-outline*`, `*-ai-accent*`, `*-primary-container`, `*-tertiary*`) are remapped onto the Cookbook tokens; the mapping table is the comment above the alias block in `index.css`. Existing screens still use them and keep working in both themes. Don't add new usages; a screen moves to the new names when it is rebuilt.

**Gotcha:** shadcn's `--accent` is the same variable as the Cookbook accent (solid ink blue), so `bg-accent` (including shadcn menu/select highlights) needs `text-on-accent` / `text-accent-foreground` for text, never `text-ink`.

### i18n

All user-visible strings go through `react-i18next`. Keys live in `src/lib/i18n/`. Use `useTranslation()` in components; never hardcode English strings.

### Utility modules

`src/lib/domain/` — domain-logic modules shared across features (each has a sibling `*.test.ts`):

- `domain/nutrition.ts` — calorie/macro calculation from food portions
- `domain/units.ts` — unit conversion (g ↔ oz, ml ↔ fl oz, etc.)
- `domain/chatEvents.ts` — SSE event parsing for the AI chat stream

Loose `*.ts` files in `src/lib/` are planner-specific utilities with their own unit tests:

- `dates.ts` — `todayISO` (the client's date, sent as `today`, D25) and `shiftYMD`
- `slot-label.ts` — slot display names
- `preset-apply-toast.ts` — toast message logic after preset apply
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

- `bun run e2e` is fully self-contained: playwright spawns the backend (`go run ./cmd/plantry` with test env vars, always fresh) and reuses or starts the Vite dev server. No manual server setup needed.
- The e2e backend runs with `PLANTRY_OFF_DISABLED=1` (no Open Food Facts calls) and the `fake` AI provider, so runs are hermetic.
- Local runs use `workers: 4` (CI: 1) to cap memory; don't raise it.
- Self-contained seeding: call the backend API directly (`http://localhost:8080`), cleanup in `finally`. Always bypass Vite proxy for seeding.
- Unique data: append `crypto.randomUUID().slice(0,8)` to names.
- After form submit: `waitForResponse` on the specific API call. Never `waitForURL`, never `waitForTimeout`.
- For elements absent from DOM (server-filtered): use `toHaveCount(0)`, never `not.toBeVisible()` (causes timeouts).
- Projects: `setup` runs `setup-wizard.spec.ts` first on the fresh DB (completes the wizard, sets `plan.anchor = today`); `chromium` and `ai-serial` depend on it. `global-settings` (specs that write the profile or plan settings through the UI, such as `plan-anchor-settings.spec.ts`) runs last and alone, after both. Never run with `--no-deps`.
- Plan board specs work in `futureWindow()` (`e2e/helpers.ts`): each parallel worker owns its own band of future windows, so one test's Clear this week or No plate overrides never touch another's plates.
- Settings are global: `setSetting` (`PUT /api/settings/{key}`) changes them for every running spec, so parallel specs override values page-scoped with `mockSettingValues` and restore anything they write.
- The shared DB accumulates recipes, so a spec that needs the library below or at the 10-recipe target stubs `GET /api/setup` with `page.route` (see `recipes-guided.spec.ts`), never relies on the real count.
- Specs that mutate global AI state (`ai-chat.spec.ts`, `generate-plan.spec.ts`, etc.) run under the `ai-serial` project with `workers: 1`. Never add AI-touching assertions to a parallel spec — it will cause flaky failures.
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
