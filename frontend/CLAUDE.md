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

Routes live in `src/routes/`. The router plugin auto-generates `src/routeTree.gen.ts` — **never edit it manually**. All routes are wrapped by `__root.tsx`, which mounts `AppShell` (sidebar + top bar) and hosts the `CommandPalette` and `CopyFromWeekDialog`.

```
src/routes/
├── __root.tsx         # AppShell, CommandPalette, CopyFromWeekDialog
├── index.tsx          # redirects to /day/today
├── day/               # planner (date-keyed views)
├── presets/           # preset library + editor
├── ingredients/       # food/ingredient CRUD
├── settings/          # app + AI settings
├── calendar/          # read-only calendar view
├── import/            # barcode / bulk import
└── archive/           # past week archive
```

### Components

```
src/components/
├── ui/                # shadcn primitives (never hand-edit generated files)
├── shell/             # AppShell, SideNav, TopBar, MobileBottomNav
├── planner/           # SlotCell, SlotSheet, PlannerGrid, DnD wrappers
├── presets/           # preset library, editor, CopyFromWeekDialog
├── picker/            # food picker (search + barcode)
├── component/         # shared input widgets: ComponentPicker, PortionStepper, QuantityUnitInput
├── components/        # food component CRUD: ComponentEditor, ComponentList, ComponentCard
├── ingredients/       # ingredient CRUD + lookup: IngredientEditor, IngredientList, LookupPanel
├── editorial/         # design system editorial primitives
├── chat/              # AI chat panel
├── command/           # CommandPalette (cmdk)
├── calendar/          # read-only calendar
├── images/            # image crop/upload UI
└── settings/          # settings panels
```

### Data layer

**Server state — TanStack Query only.** Never mirror server data in Zustand.

- `src/lib/api/` — one file per resource, all calls go through `apiFetch` in `lib/api/client.ts`. `ApiError` carries `status` + `messageKey` for i18n error display.
- `src/lib/queries/` — TanStack Query hooks + `keys.ts` (centralized key factories). Key hierarchies are intentional: invalidating a parent key also drops all child keys (e.g. invalidating `plateKeys.range(from, to)` also drops `plateKeys.macrosRange(from, to)`).

**Client-only state — Zustand stores.**

- `src/lib/stores/planner-ui.ts` — ephemeral planner UI: which plate is being edited, the "add" sheet target, AI-fill session state, copy-hint-seen flag. One `localStorage` key: `plantry.copyHintSeen`.
- `src/lib/stores/chat-ui.ts` — chat panel open/closed state.
- `src/lib/stores/chat-stream.ts` — streaming AI message state.

### Forms

All forms use `react-hook-form` + `zod` via `@hookform/resolvers/zod`. Schema definitions live in `src/lib/schemas/`.

### Design system

Tokens are defined in `src/index.css` (OKLch via Tailwind 4 `@theme inline`). The design system is **Botanical Atelier**: a Material-style surface ladder (`--surface`, `--surface-container-*`, `--on-surface`, `--on-surface-variant`) with OKLch color values. Heading font: `Manrope Variable`. Body font: `Inter Variable`.

Custom Tailwind utilities available: `bg-surface`, `bg-surface-container`, `bg-surface-container-low`, `bg-surface-container-high`, `text-on-surface`, `text-on-surface-variant`, `border-outline`, `border-outline-variant`, etc.

### i18n

All user-visible strings go through `react-i18next`. Keys live in `src/lib/i18n/`. Use `useTranslation()` in components; never hardcode English strings.

### Utility modules

`src/lib/domain/` — domain-logic modules shared across features (each has a sibling `*.test.ts`):

- `domain/nutrition.ts` — calorie/macro calculation from food portions
- `domain/units.ts` — unit conversion (g ↔ oz, ml ↔ fl oz, etc.)
- `domain/chatEvents.ts` — SSE event parsing for the AI chat stream

Loose `*.ts` files in `src/lib/` are planner-specific utilities with their own unit tests:

- `planner-window.ts` — date window calculations
- `planner-keynav.ts` — keyboard navigation logic
- `planner-skip.ts` — slot-skip logic
- `slot-label.ts` — slot display names
- `preset-apply-toast.ts` — toast message logic after preset apply

## Testing

### Vitest (unit)

- Mock at `@/lib/api/*` module level — TanStack Query works normally with mocked fetch fns.
- Use `renderWithRouter` from `@/test/render` — wraps in `QueryClientProvider` (retry: false) + `RouterProvider` (createMemoryHistory).
- Use shared fixtures from `@/test/fixtures` — don't duplicate mock data per test file.
- Use `screen.findBy*` (async) — `RouterProvider` renders async.

### Playwright (e2e)

- `bun run e2e` is fully self-contained: playwright spawns the backend (`go run ./cmd/plantry` with test env vars, always fresh) and reuses or starts the Vite dev server. No manual server setup needed.
- Self-contained seeding: call the backend API directly (`http://localhost:8080`), cleanup in `finally`. Always bypass Vite proxy for seeding.
- Unique data: append `crypto.randomUUID().slice(0,8)` to names.
- After form submit: `waitForResponse` on the specific API call. Never `waitForURL`, never `waitForTimeout`.
- For elements absent from DOM (server-filtered): use `toHaveCount(0)`, never `not.toBeVisible()` (causes timeouts).
- Specs that mutate global AI state (`ai-chat.spec.ts`, `generate-plan.spec.ts`, etc.) run under the `ai-serial` project with `workers: 1`. Never add AI-touching assertions to a parallel spec — it will cause flaky failures.
- Stable = `--repeat-each=10 --workers=4` with zero failures.

## Gotchas

### React / ESLint

- **No ref access during render** (`react-hooks/refs`). Reset derived state in event handlers instead.
- **No setState in effects** (`react-hooks/set-state-in-effect`). Move state resets to event handlers.
- **Prefer `useDeferredValue`** over `useState`+`useEffect` debounce.
- `Date.now()` / `new Date()` during render is flagged by the React compiler as impure — isolate into a `useState` initializer or a separate component.

### shadcn

- `FormControl` uses `Slot.Root` (radix-ui) so id/aria forward to the child input. `useFormField` reads id from `FormItemContext`.
- `src/components/ui/` is generated — prefer wrapping over editing primitives directly.
