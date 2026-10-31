---
name: Plantry
description: Self-hosted weekly meal planner; the "Cookbook with batch badges" visual system.
colors:
  ground: "#fbfbfa"
  panel: "#ffffff"
  panel-sunken: "#f3f4f6"
  ink: "#1c2233"
  ink-muted: "#5a6175"
  line: "#e3e5ea"
  accent: "#27407a"
  on-accent: "#fbfbfa"
  batch-1: "#7a3b2e"
  batch-1-tint: "#f3e7e2"
  batch-2: "#8a6a12"
  batch-2-tint: "#f4eedb"
  batch-3: "#2f6b5a"
  batch-3-tint: "#e1efe9"
  batch-4: "#6a3d6e"
  batch-4-tint: "#efe4f0"
  batch-5: "#5d6b2a"
  batch-5-tint: "#ecefdc"
  batch-6: "#3d5a80"
  batch-6-tint: "#e3eaf3"
  success: "#2e6b3f"
  warning: "#9a5b00"
  danger: "#a23a2a"
  ground-dark: "#161821"
  panel-dark: "#1d2030"
  panel-sunken-dark: "#12141c"
  ink-dark: "#e6e8f0"
  ink-muted-dark: "#a3a8bb"
  line-dark: "#2e3245"
  accent-dark: "#9db2ea"
  on-accent-dark: "#161821"
  batch-1-dark: "#e29a88"
  batch-1-tint-dark: "#35231f"
  batch-2-dark: "#d8b75a"
  batch-2-tint-dark: "#332b14"
  batch-3-dark: "#7cc4ad"
  batch-3-tint-dark: "#1b302a"
  batch-4-dark: "#c99bcd"
  batch-4-tint-dark: "#2e2131"
  batch-5-dark: "#b5c46e"
  batch-5-tint-dark: "#272c16"
  batch-6-dark: "#93b0d6"
  batch-6-tint-dark: "#1c2533"
  success-dark: "#79c48f"
  warning-dark: "#e3a54f"
  danger-dark: "#ee8b7b"
typography:
  display:
    fontFamily: "Newsreader Variable, Georgia, serif"
    fontSize: "1.75rem"
    fontWeight: 600
  headline:
    fontFamily: "Newsreader Variable, Georgia, serif"
    fontSize: "1.4375rem"
    fontWeight: 600
  title:
    fontFamily: "Newsreader Variable, Georgia, serif"
    fontSize: "1.1875rem"
    fontWeight: 600
  body:
    fontFamily: "Source Sans 3 Variable, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
  label:
    fontFamily: "Source Sans 3 Variable, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
  caption:
    fontFamily: "Source Sans 3 Variable, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
  numeric:
    fontFamily: "Source Sans 3 Variable, system-ui, sans-serif"
    fontFeature: "tnum"
rounded:
  cell: "4px"
  button: "8px"
  sheet: "16px"
  tag: "3px 10px 10px 3px"
spacing:
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "6": "24px"
  "8": "32px"
  "12": "48px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.on-accent}"
    rounded: "{rounded.button}"
    height: "32px"
    padding: "0 10px"
  nav-item:
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.button}"
  nav-item-active:
    textColor: "{colors.accent}"
    rounded: "{rounded.button}"
  callout:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.button}"
    padding: "12px 16px"
  notes-panel:
    backgroundColor: "{colors.panel-sunken}"
    textColor: "{colors.ink}"
    rounded: "{rounded.button}"
    padding: "16px 20px"
---

# Design System: Plantry

Source of truth: `frontend/src/index.css`, checked against [`docs/UX_DESIGN_SPEC.md`](docs/UX_DESIGN_SPEC.md) §3 by `frontend/src/test/tokens.test.ts` (values and WCAG contrast). If this file and `index.css` disagree, `index.css` wins and this file is stale.

**Status key:** *Implemented* means it is in `index.css` or a shipped component today. *Specified* means the UX spec defines it but it is not built yet (scheduled for Phases 1–3).

## Overview

**Creative North Star: "Cookbook with batch badges"**

A well-used home cookbook: serif headings like a recipe title, a quiet paper-white ground, ink-blue for actions, and the earthy colours of cooked food (rust, ochre, herb) reserved for batches. Plantry is an **Operate** tool. It should feel familiar, scannable and calm. Its identity lives in type, palette and one signature detail (the batch badge on every slot), never in unusual controls.

The system replaced the earlier *Botanical Atelier* look (sage green, Manrope/Inter, gradients, glass). Its aliases, palettes and utilities were removed in Phase 2 (Z1); it is not part of this system.

Principles (UX spec §1.2):

1. One obvious next action per screen.
2. Batches are visible everywhere. A plate always says whether it is cooked here or a leftover, and from which batch.
3. Summary first, detail on tap. Nutrition, reasons and macros sit one tap away.
4. The assistant acts in place. Inline actions do most of the work; chat is a side panel.
5. Nothing silently changes what you bought. Confirmed plates show a padlock.

**Key Characteristics:**

- Cool paper neutrals with a slight blue bias, so they read as chosen rather than default grey.
- One action colour (ink-blue accent). Food colours belong to batches only.
- Serif for titles, sans for everything you read as data or press.
- Flat surfaces separated by hairlines; one soft shadow for overlays.
- Motion only to explain a state change.

## Colors

Two neutral layers and an ink pair, one accent, six batch colours with tints, and three state colours. Light and dark values are separate tokens in the frontmatter (`*-dark` keys). In code, dark mode is the `.dark` class on `<html>`, set by `theme-provider.tsx` from the Light / Dark / System choice (System follows `prefers-color-scheme`). *Implemented.*

### Primary

- **Ink Blue** (`accent`): primary actions, the focus ring, current selection, active nav item, and links. Info states also use it. Text and icons on it use **on-accent**, never `ground` or white by hand.

### Neutral

- **Paper Ground** (`ground`): the page background.
- **Panel** (`panel`): cells, sheets, and cards that have a role (callouts, the mobile tab bar).
- **Sunken Panel** (`panel-sunken`): the second neutral layer, used for the side rail, toolbars, and the recipe notes block.
- **Ink** (`ink`): primary text.
- **Muted Ink** (`ink-muted`): secondary text, labels, quantities, inactive nav items.
- **Hairline** (`line`): borders and dividers. It carries separation in lists in place of shadows.

### Batch palette

Six earthy colours, each with a solid badge colour and a pale tint: **1 Rust, 2 Ochre, 3 Herb, 4 Plum, 5 Olive, 6 Slate**. Tokens and utilities exist (`batch-N`, `batch-N-tint`). *Implemented.* `lib/plan/batch-colours.ts` assigns them (`colourBatches`: cook-cell order within the window, cycling after six) and `BatchBadge` renders them. *Implemented.* The recipe detail placeholder image also uses Herb (tint with a Herb serif initial).

**The Batches-Only Rule.** Batch colours mean "this food comes from batch N". Never use them for state, decoration, or category colour. Batches take colours in order and cycle after six.

**The Tint-Means-Cook Rule.** A batch tint background marks the slot where the batch is cooked (and the Today card). Leftover slots stay plain panel and show the batch colour only in the badge. Badge text is `ground` on the batch colour.

### State colours

- **Success** (`success`): on target.
- **Warning** (`warning`): below target, or needs attention (for example, the nutrition completeness warning's triangle icon).
- **Danger** (`danger`): violations and destructive actions. shadcn `destructive` resolves to it.

**The Meaningful State Rule.** State colours appear only where they carry meaning, and always with an icon or a word. They are separate from batch colours and must never be used to tell batches apart.

### Nutrition

There is no per-macro palette. Nutrition reads as one line of text, `MacroLine`: kcal in ink, then protein, fat and carbs in ink-muted ("452 kcal · P 31 g · F 12 g · C 50 g"), with tabular numerals and the `units.*` labels.

## Typography

**Display Font:** Newsreader Variable (optical size axis plus italic), with Georgia and serif as fallbacks.
**UI Font:** Source Sans 3 Variable, with system-ui and sans-serif as fallbacks.

Both are self-hosted through `@fontsource-variable/*` and imported at the top of `index.css`, so the LAN app makes no Google Fonts call. *Implemented.*

**Character:** a recipe-book serif for names and titles over a plain, sturdy humanist sans for the working parts.

### Hierarchy

The scale is fixed rem with a ratio of about 1.2. Utilities: `text-h1`, `text-h2`, `text-h3`, `text-body`, `text-small`, `text-caption`. *Implemented.*

- **Display / h1** (Newsreader 600): page titles, recipe names on detail and cook mode. Base styles give `h1` this face, size and weight.
- **Headline / h2**, **Title / h3** (Newsreader): section headings such as Ingredients, Steps and Notes. Base styles set face and size only; components add `font-semibold`.
- **Body** (Source Sans 3 400): the default `body` size. Prose (recipe steps, notes) is capped at 68ch.
- **Small / label** and **Caption** (Source Sans 3): metadata labels, nav labels, helper text.
- **Numbers** (Source Sans 3 with `tabular-nums`): macros, quantities, counts, step numbers. Recipe detail shows meta values at h3 size in the sans, not the serif.
- *Specified, not tokenised:* the Today headline is 26px on phone. Newsreader italic is reserved for board row labels and notes (Phase 2).

**The Serif-For-Names Rule.** Newsreader is for display only: titles, recipe names, the Today headline, board row labels. Never use it for buttons, inputs, labels, nav, or data.

**The Sentence-Case Rule.** Labels use sentence case. There are no all-caps labels and no letter-spaced eyebrows.

## Layout

- **Spacing:** the spec scale is 4, 8, 12, 16, 24, 32 and 48px. There are no custom spacing tokens. Use Tailwind's default 4px steps that land on that scale (`1, 2, 3, 4, 6, 8, 12`). Lay out with `gap`, not margins.
- **Shell** (*implemented*): below 768px, a fixed bottom tab bar (Today, Plan, Recipes, Shop) on panel with rounded top corners and a 4rem height plus safe-area inset (`--tab-bar-h`, `--tab-bar-inset`). Main content reserves that height as bottom padding. From 768px up there is a sunken left rail: an 80px icon rail at 768–1023px and when collapsed, and a 256px labelled rail from 1024px. Settings sits at the bottom of the rail; on phone it is a gear in the Today header (there is no top bar), and the compact Generate button sits in the `/plan` header. There is no centre action button in the tab bar. The setup wizard (`/setup`) renders without the shell.
- **Content width:** detail pages centre at `max-w-5xl` with 16px side padding on phone and 32px from 768px up. Two-column detail layouts use a 12-column grid (for example 5/7 for the header and 4/8 for ingredients and steps).
- **Verification widths:** 375px phone, 820px iPad portrait, 1180px iPad landscape, 1440px desktop, each in light and dark.

## Elevation & Depth

Flat by default. Depth comes from the two neutral layers (`panel` on `ground`, `panel-sunken` for chrome) and hairline borders.

### Shadow Vocabulary

- **Soft sheet shadow** (`--shadow-soft`, utility `shadow-sheet`): two-layer ambient shadow tinted with ink (deeper black in dark mode). For sheets and popovers only. *Implemented*; `RoutinePicker` uses it.

**The One Shadow Rule.** Board cells and list cards get no shadow; the border carries separation. The only shadow is the sheet shadow, on overlays.

## Shapes

Corner radius is assigned by role, so not everything gets the same curve:

- **Cell** (`rounded-cell`): board cells and inputs.
- **Button** (`rounded-lg`, from `--radius`): buttons, nav items, callouts, recipe images.
- **Sheet** (`rounded-sheet`): sheets and the phone tab bar, top corners only. The tab bar uses `rounded-t-2xl` today, which is the same 16px.
- **Tag** (`rounded-tag`): batch badges, an asymmetric tag silhouette that is square at the left and round at the right.

The `rounded-cell`, `rounded-sheet` and `rounded-tag` utilities are *implemented*. `rounded-cell` is in use (availability and slot cells, settings inputs, `SourceBadge`); `rounded-tag` is used by `BatchBadge`; `rounded-sheet` is not used yet. Some legacy screens still use `rounded` (also 4px) or larger radii.

**Icons:** Lucide at a 1.5px stroke (`strokeWidth={1.5}`), 16px or 20px (`size-4`, `size-5`), always `aria-hidden` next to a text label or an `aria-label`. No emoji in the UI.

## Components

Built on shadcn/Radix primitives (sheet, dialog, popover, tabs, dropdown-menu, tooltip), sonner toasts, cmdk and dnd-kit. Screens rebuilt in Phase 0 that show the system: the shell (`components/shell/*`), recipe detail (`RecipeDetail.tsx`), `NutritionCompletenessWarning.tsx`, and the import summary and resolve rows (`components/import/*`). Phase 1 added the setup wizard (`components/setup/*`), the settings sections (`components/settings/*`), `AvailabilityGrid` and the guided Recipes state.

### Buttons

- **Shape:** button radius, 32px tall by default (`components/ui/button.tsx`).
- **Primary:** accent fill with on-accent text. **Outline / secondary / ghost** use the neutral layers. **Destructive** is a danger-tinted fill with danger text.
- **Focus:** the global 2px accent outline (see Focus).
- **Motion:** colour changes only, 150ms ease-out; a press nudges the button down 1px. No hover lift or shadow. Badges (`components/ui/badge.tsx`) follow the same rule.
- **Generate plan** (`GeneratePlanButton`): a solid accent button. Below the recipe target it stays focusable but `aria-disabled`, and a tooltip (plus a link to Recipes in the wide rail) says how many recipes are missing.

### Navigation (implemented)

- **Items:** muted ink at rest. Hover moves to a `panel` background with ink text. Active is accent text, semibold, on a 10% accent wash, with `aria-current="page"`. Lucide icons are 20px at a 1.5px stroke.
- **Focus:** a 2px accent ring on every item.
- **Touch:** tab bar links are at least 44px tall.

### Callouts (implemented: `NutritionCompletenessWarning`)

Panel background, hairline border, button radius, 12px by 16px padding. A warning-coloured triangle icon leads a semibold title and a muted body. Inline links are accent and underlined, and the underline is removed on hover. The callout uses `role="status"`.

### Recipe detail (implemented)

A serif h1 name. Meta values use h3-size tabular sans over small muted labels. The ingredient list has hairline dividers and right-aligned tabular quantities. Steps are numbered in muted tabular sans, capped at 68ch. Notes sit in a sunken panel block. With no image, the placeholder is a Herb-tint tile with a large serif initial.

### Batch primitives (implemented: `components/batch/*`)

`BatchBadge` is the same component on the board, Today, cook mode, recipe detail and the shopping list. Its colour comes from `colourBatches` (`lib/plan/batch-colours.ts`); the linked highlight from `BatchHighlightProvider`.

- **Badge:** a `rounded-tag` in caption semibold with tabular numerals, `ground` text on the solid batch colour. Variants: cook "Cook × 4", leftover "2/4", frozen "2/4 · frozen" with a 12px snowflake, and in the cook strip the dashed-outline extras "2 spare" and "2 to freeze" (batch-colour text and dashed border, no fill). German: "Koche × 4", "2/4 · gefroren", "2 übrig", "2 zum Einfrieren".
- **Linked highlight:** pointing at a badge (hover, or the tap that starts a touch) outlines every cell of that batch for 2s. Cells opt in with `useBatchCell`, which sets `data-highlighted` to the batch colour, and style it with `BATCH_HIGHLIGHT_CLASS`: a 2px ring in that batch colour, offset 2px over `ground`. It fades in over 200ms ease-out (instant under reduced motion). It is a ring (box-shadow), so the accent focus outline stays untouched.
- **Cook strip** (`CookStrip`): a "Cook days" region with one 44px button per batch in cook order, "Sun · Chili × 4" plus any dashed extras, named "Sun · Chili × 4, 2 spare". The backend surplus splits into "to freeze" (unconsumed frozen portions, at most the surplus) and "spare" (the rest). Pressing one scrolls its cook cell into view (instantly under reduced motion) and highlights the batch. The row scrolls sideways when it overflows.
- **Day meter** (`DayMeter`): a 4px rounded bar on a `line` track that runs to 125% of the kcal target, with a 1px `ink` tick at the target (80%), so an overshoot reads by length as well as colour. Success fill between 70% and 115% of the target, warning fill outside, a full `ink-muted` bar with no target. A 6px warning dot marks an incomplete day. It is a `meter` with value text "1,840 of 2,200 kcal" ("…, incomplete" on an incomplete day); a transparent button stretched over the 44px row opens the popover.
- **Nutrition popover** (`NutritionPopover`): panel with the sheet shadow. The day as title, "Per person", then kcal, protein, fat and carbs as "value / target unit" in tabular numerals. An incomplete day ends with a warning triangle, "Incomplete" and the foods without data.

*Specified* (Phase 2, later steps): the cook cell (batch tint, a 1.5px ink inset outline), the leftover cell (plain panel), `PlateCell`, `PlateSheet`, `SuggestionCard`, `ShoppingRow` and `StepPager`. See UX spec §4 and §9.

### Availability grid (implemented: `AvailabilityGrid`)

A weekday (or date) × slot table of cell modes, used in Settings → Meals and the setup wizard. It holds no server state; the parent owns the value.

- **Plan:** panel cell with a hairline border and muted "Plan".
- **No plate:** no border or fill, muted italic "No plate". The hairline appears on hover.
- **Routine:** panel cell with a muted "Routine" caption over the preset name in ink.
- **Interaction:** a tap cycles Plan → No plate → Routine → Plan. Routine first asks for a preset in `RoutinePicker` (a popover with the sheet shadow); closing it without a pick (Cancel, Escape, outside click) lands on Plan, so No plate is never a dead end. With no presets the cycle is Plan ↔ No plate. Focus returns to the cell when the picker closes. An optional column-header menu sets "No plate all day" or "Plan all day".
- **Shape:** `rounded-cell` buttons at least 44px tall, 150ms colour transitions.
- **Accessibility:** `role="grid"` with one tab stop (roving tabindex); arrow keys, Home and End move between cells. Each cell's name reads the full day, the slot and the state.
- **Width:** the grid keeps a 32rem minimum width at every breakpoint and scrolls sideways inside its own box when its container is narrower; the slot column stays sticky. State words wrap; preset names clamp to two lines. The `surface` prop (`ground` or `panel`) gives the sticky column the background the grid sits on.

### Guided state (implemented: `RecipeOnboarding`)

Shown on Recipes while the library is below the planning target (10 recipes).

- **Progress row:** "n of 10 recipes …" in body ink above an 8px accent bar on `panel` with an `ink-muted` border, exposed as a `progressbar`.
- **Option cards:** two cards (Import, Create), one column on phone and two from 640px. Each is panel with a hairline border and button radius: a 20px accent icon, a title (an `h2` styled `text-h3`), a muted body, and a 44px button at the bottom (primary for Import, outline for Create).
- When the target is reached the state disappears and a one-time milestone toast fires.

**The Never-Colour-Alone Rule.** A badge always carries text ("Cook × 4", "2/4"). Colour alone never identifies a batch or a state.

### Interaction patterns (spec §6, applies to all new work)

- Use sheets for editing in context. Use dialogs only for destructive confirmations and the plan wizard. Never stack two overlays.
- Prefer undo to confirm: reversible actions run immediately with a 6s undo toast.
- Show skeletons in place of content, not centred spinners.
- Errors state what happened and what to do, with no apology.

### Focus (implemented)

One indicator everywhere: a base `:focus-visible` rule in `index.css` draws a 2px solid accent outline with a 2px offset. A component's `outline-none` still wins; nav items and `GeneratePlanButton` draw an equivalent 2px accent ring instead.

### Motion

150–250ms ease-out, and only for state change: a sheet slides in, a plate cross-fades on swap, a ticked item slides down, linked batch cells highlight. There is no page-load choreography and no hover animation on cards. Nav uses 150–200ms colour and width transitions.

**Reduced motion** (*implemented*): under `prefers-reduced-motion`, a global rule in `index.css` collapses transitions to near zero, runs animations once at 150ms and removes the slide offsets of sheet and popover animations, so slides become fades. `.animate-spin` is exempt: loading spinners keep turning because they convey state. `AnimatedNumber` also checks the preference.

### Voice

A plain, calm tool. Short factual sentences, sentence case, active verbs. No exclamation marks (except the 10-recipe milestone toast), no "Oops", no apologies. A toast names the result using the control's verb ("Accept and shop" gives "Plan accepted"). Every string exists in English and German, and the German is written natively. Vocabulary: plate, batch, leftover, no plate, routine, draft and confirmed, assistant (never "AI", "agent" or "LLM" in labels). See spec §7.

### shadcn names

The shadcn primitives in `components/ui` read the shadcn theme variables. `index.css` maps each onto a Cookbook token in both themes (the table in the file's comment is authoritative):

- `background` resolves to **ground**; `card` and `popover` to **panel**; `secondary` and `muted` to **panel-sunken**.
- `foreground` resolves to **ink**; `muted-foreground` to **ink-muted**; `border` and `input` to **line**.
- `primary` and `ring` resolve to **accent**; `primary-foreground` and `accent-foreground` to **on-accent**; `destructive` to **danger**.

App code uses the Cookbook names: `bg-ground`, `bg-panel`, `text-ink`, `border-line`, `text-accent`, `text-on-accent`, and so on. The Botanical Atelier Material aliases, the macro and chart palettes and the `gradient-primary`, `editorial-shadow`, `glass-header` and `btn-shimmer` utilities are gone; `src/test/tokens.test.ts` allowlists every custom property `index.css` defines.

- **The Accent Collision.** `--accent` is both the Cookbook accent and shadcn's hover/focus fill, because they share a variable name. Menu and select highlights therefore render solid accent. Text on any accent fill must use `text-on-accent` (shadcn's `accent-foreground` resolves to the same value). Never put `text-ink` or `text-accent` on an accent fill.

## Do's and Don'ts

### Do:

- **Do** use the Cookbook token names in every new or rebuilt component.
- **Do** keep one accent: ink-blue for actions, focus, selection and links.
- **Do** set numbers in Source Sans 3 with `tabular-nums`.
- **Do** give every interactive element a visible 2px accent focus outline. On touch devices, targets are at least 44 × 44px; everywhere else, at least 24px.
- **Do** keep text and badge pairs at WCAG AA (4.5:1 for text, 3:1 for large text and UI marks). The token test enforces this for the core pairs.
- **Do** pair every state colour with an icon or a word.
- **Do** separate list items with hairlines, not shadows.

### Don't:

- **Don't** use Newsreader for buttons, inputs, labels, nav or data.
- **Don't** use batch colours for state, category or decoration.
- **Don't** add gradients, glass or blur, shimmer sweeps, hover lifts or zooms, or sage-tinted shadows (the Botanical Atelier look).
- **Don't** add shadows to cells or list cards, or a second shadow style.
- **Don't** use all-caps or letter-spaced labels.
- **Don't** use emoji in the UI.
- **Don't** use centred spinners.
- **Don't** use the shadcn names in app code, or add a per-macro or chart palette.
