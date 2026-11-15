---
name: Plantry
description: Self-hosted weekly meal planner; a green-neutral board where every batch reads as one coloured line across the week.
colors:
  ground: "#f5f6f4"
  panel: "#ffffff"
  panel-sunken: "#eef0ec"
  panel-raised: "#ffffff"
  ink: "#18201b"
  ink-muted: "#5d6861"
  line: "#e1e4de"
  accent: "#157a3c"
  on-accent: "#ffffff"
  batch-1: "#c2410c"
  batch-1-tint: "#fff1e8"
  batch-2: "#1d4ed8"
  batch-2-tint: "#e9effe"
  batch-3: "#6d28d9"
  batch-3-tint: "#f2ecfe"
  batch-4: "#0f766e"
  batch-4-tint: "#e3f4f1"
  batch-5: "#be185d"
  batch-5-tint: "#fce8f1"
  batch-6: "#9d6006"
  batch-6-tint: "#fbf1dc"
  success: "#2e6b3f"
  warning: "#a15c00"
  danger: "#b42318"
  ground-dark: "#111513"
  panel-dark: "#191e1b"
  panel-sunken-dark: "#0d100e"
  panel-raised-dark: "#212723"
  ink-dark: "#e6ebe7"
  ink-muted-dark: "#a0aaa3"
  line-dark: "#2a312d"
  accent-dark: "#4cc276"
  on-accent-dark: "#0d100e"
  batch-1-dark: "#f08a4b"
  batch-1-tint-dark: "#3a2418"
  batch-2-dark: "#7ea3f5"
  batch-2-tint-dark: "#1c2640"
  batch-3-dark: "#a98bf5"
  batch-3-tint-dark: "#2a2140"
  batch-4-dark: "#3cc7b3"
  batch-4-tint-dark: "#143330"
  batch-5-dark: "#ef7fb0"
  batch-5-tint-dark: "#3a1d2b"
  batch-6-dark: "#e0b341"
  batch-6-tint-dark: "#362c12"
  success-dark: "#6fcf8f"
  warning-dark: "#e3a54f"
  danger-dark: "#f08b7b"
typography:
  h1:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "1.75rem"
    fontWeight: 700
    letterSpacing: "-0.02em"
  h2:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "1.4375rem"
    fontWeight: 700
    letterSpacing: "-0.01em"
  h3:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "1.1875rem"
    fontWeight: 700
    letterSpacing: "-0.01em"
  body:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
  small:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
  caption:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 600
  numeric:
    fontFamily: "Figtree Variable, system-ui, sans-serif"
    fontFeature: "tnum"
rounded:
  button: "8px"
  cell: "10px"
  panel: "11.2px"
  sheet: "16px"
  pill: "9999px"
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
  plate-card:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.cell}"
    height: "84px"
    padding: "10px"
  plate-card-cook:
    backgroundColor: "{colors.batch-1-tint}"
    textColor: "{colors.ink}"
    rounded: "{rounded.cell}"
  batch-pill-cook:
    backgroundColor: "{colors.batch-1}"
    textColor: "{colors.on-accent}"
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
    padding: "1px 8px"
  batch-pill-leftover:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.batch-1}"
    typography: "{typography.caption}"
    rounded: "{rounded.pill}"
    padding: "1px 8px"
  review-panel:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.panel}"
    padding: "10px 16px"
  nav-item:
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.button}"
  nav-item-active:
    textColor: "{colors.accent}"
    rounded: "{rounded.button}"
---

# Design System: Plantry

Source of truth: `frontend/src/index.css`, checked against [`docs/UX_DESIGN_SPEC.md`](docs/UX_DESIGN_SPEC.md) §3 by `frontend/src/test/tokens.test.ts` (values and WCAG contrast). If this file and `index.css` disagree, `index.css` wins and this file is stale. This file replaces the earlier "Cookbook" record (Newsreader serif, ink-blue, paper ground); none of that world survives.

## Overview

**Creative North Star: "The Timetable Board"**

The familiar modern meal planner, executed with precision: a light neutral ground with a faint green bias, white 10 px cards under a hairline and a whisper of shadow, one fresh green for actions, and six vivid colours that mean nothing except "this food belongs to batch N". Plantry is an **Operate** tool. It should feel familiar on first sight, scannable in a glance and calm. Its identity is the batch lane: each batch is drawn like a timetable line from the day it is cooked to the last day its leftovers are eaten.

The product stance (PRODUCT.md, 2026-11-15) prefers a familiar, craft-finished meal-planner look over a themed world. Ruled out: cold navy on blue-grey, flat grey, cream paper with a serif, busy colour, and tall vertical photo cards on the plan board.

Principles carried from the product (UX spec §1.2):

1. One obvious next action per screen.
2. Batches are visible everywhere. A plate always says whether it is cooked here or a leftover, and from which batch.
3. Summary first, detail on tap.
4. The assistant acts in place; chat is a side panel.
5. Nothing silently changes what you bought. Confirmed plates show a padlock.

**Key Characteristics:**

- Green-biased neutrals, so the ground reads as chosen, not default grey.
- One accent, green, for actions, selection and focus only.
- One typeface, Figtree; weight carries hierarchy.
- White cards separated by a hairline and a 1 px whisper shadow; one deeper shadow for overlays.
- Batch colours appear only on lanes, lane labels, pills and the cook-cell tint.
- Browser surfaces are themed too: text selection, caret and scrollbars take the accent and ink.

**Known gap.** Only the plan board (`/plan`) has been redesigned into this world. Today, Recipes, Shop, Cook and Settings inherit the tokens, Figtree and the shadcn mapping, but their layouts and components have not been reworked and may still carry shapes and treatments from the previous system. Treat the plan board as the reference, not those screens.

## Colors

A green-neutral ground, one green accent, six vivid batch colours with pale tints, and three state colours. Light and dark values are separate tokens in the frontmatter (`*-dark` keys). In code, dark mode is the `.dark` class on `<html>`, set by `theme-provider.tsx` from the Light / Dark / System choice (System follows `prefers-color-scheme`). Dark is a near-black green-grey with brighter batch colours.

### Primary

- **Fresh Green** (`accent`, #157a3c; dark #4cc276): primary actions (Accept and shop), the focus ring, current selection, today's date header, active nav item (on a 10% accent wash), links, and the drop-target state of a cell. Text on it is `on-accent` (#ffffff; dark #0d100e).

### Neutral

- **Ground** (`ground`, #f5f6f4; dark #111513): the page, and the sticky day headers and meal column so scrolled content slides under them.
- **Panel** (`panel`, #ffffff; dark #191e1b): cards, cells, sheets, the side rail, the review and empty-week panels.
- **Sunken Panel** (`panel-sunken`, #eef0ec; dark #0d100e): second neutral layer for hover fills, tags and the unsaved-draft chip.
- **Raised Panel** (`panel-raised`, #ffffff; dark #212723): the assistant panel; equal to panel in light, a step lighter in dark.
- **Ink** (`ink`, #18201b; dark #e6ebe7): primary text, and the 1 px target tick on the day meter.
- **Muted Ink** (`ink-muted`, #5d6861; dark #a0aaa3): secondary text, meal-row labels, captions, inactive nav.
- **Hairline** (`line`, #e1e4de; dark #2a312d): card borders, dividers and the meter track.

### Batch palette

Six vivid colours, each with a solid line colour and a pale tint: **1 Orange** (#c2410c), **2 Blue** (#1d4ed8), **3 Violet** (#6d28d9), **4 Teal** (#0f766e), **5 Pink** (#be185d), **6 Amber** (#9d6006). Tints: #fff1e8, #e9effe, #f2ecfe, #e3f4f1, #fce8f1, #fbf1dc (dark line / tint pairs are in the frontmatter). `lib/plan/batch-colours.ts` (`colourBatches`) assigns them in cook-cell order within the window and cycles after six; `BATCH_CLASSES` spells out the Tailwind classes.

**The Batches-Only Rule.** A batch colour means "this food comes from batch N". It draws the lane line, the lane label, the pill and the cook-cell tint, and nothing else: never state, category, decoration or a button.

**The One Green Rule.** The accent is for actions, selection and focus. It is not a batch colour and not a success colour; success has its own token.

**The Tint-Means-Cook Rule.** A batch tint background marks the cell where the batch is cooked. Leftover cells stay plain panel and show the batch colour only in the outlined pill.

### State colours

- **Success** (`success`, #2e6b3f; dark #6fcf8f): the day meter fill when on target (70 % to 115 % of the kcal target).
- **Warning** (`warning`, #a15c00; dark #e3a54f): the meter fill outside that range, the incomplete-day dot, the attention triangle and the dashed attention border on a cell.
- **Danger** (`danger`, #b42318; dark #f08b7b): violations and destructive actions. shadcn `destructive` resolves to it.

**The Meaningful State Rule.** State colours appear only where they carry meaning, and always with an icon or a word.

### Browser surfaces

Text selection is the accent at 28 % (`::selection`), the caret is the accent, and scrollbars use `ink-muted` at 45 % on a transparent track.

## Typography

**One typeface:** Figtree Variable (upright and italic), with system-ui and sans-serif as fallbacks, self-hosted through `@fontsource-variable/figtree` (no Google Fonts call from a LAN app). `font-heading` and `font-sans` both resolve to Figtree; `font-heading` survives only as a role alias.

**Character:** a friendly, sturdy geometric sans that does the work of titles, labels, controls and data. There is no second face; weight (400 body, 600 emphasis, 700 headings) and size carry the hierarchy.

### Hierarchy

Fixed rem, ratio about 1.2. Utilities: `text-h1`, `text-h2`, `text-h3`, `text-body`, `text-small`, `text-caption`.

- **h1** (700, 1.75rem / 28 px, letter-spacing -0.02em): page titles, the week range on the plan board. Base styles set size, weight and spacing.
- **h2** (700, 1.4375rem / 23 px): section headings and the Today headline. **h3** (700, 1.1875rem / 19 px): card and panel titles such as the empty-week panel title (semibold there).
- **Body** (400, 0.9375rem / 15 px; 1 rem on phones): default text, meal-row labels (semibold, muted), day numbers (semibold, tabular).
- **Small** (0.8125rem / 13 px; 0.875rem on phones): plate names on cards (semibold, two lines), draft summary, hints.
- **Caption** (0.75rem / 12 px): lane labels (semibold, in the batch colour), pill text (semibold), card sublines, kcal text under no target.
- **Numbers**: `tabular-nums` for macros, kcal, counts, dates and pills.
- Prose (recipe steps, notes) is capped at 68ch.

**The One Face Rule.** Hierarchy comes from weight and size, never from a second typeface or a letter-spaced label.

**The Sentence-Case Rule.** Labels use sentence case. There are no all-caps labels and no letter-spaced eyebrows.

## Layout

- **Spacing:** 4, 8, 12, 16, 24, 32 and 48 px. Lay out with `gap`, not margins. Cards sit 4 px apart on the board (`gap-1` in a `p-1` grid).
- **Shell:** below 768 px a fixed bottom tab bar (Today, Plan, Recipes, Shop; 4 rem plus the safe-area inset, `--tab-bar-h`, `--tab-bar-inset`). From 768 px a sticky panel-coloured left rail with a hairline right border: 80 px icon rail at 768 to 1023 px or when collapsed, a 208 px (`w-52`) labelled rail from 1024 px, holding nav, the Generate plan button and Settings.
- **Plan board** (`BoardGrid`): a real ARIA grid, a meal column plus seven day columns starting on the shopping day. From 1024 px the meal column is 7 rem and the days share the width; from 768 to 1023 px the meal column is 6 rem and days are `(100cqw - 7rem) / 4.5` wide, so four and a half days show, the cut-off fifth hints at the swipe, and columns snap per day. Day headers (56 px, on ground) and the meal column stay sticky; the scroll box is capped at 70dvh below 1024 px.
- **Above the board:** the toolbar header (week title, stepper, draft summary, attention count, assistant, shop actions), then the draft review panel when drafts exist (hidden below 768 px) or the empty-week panel when the window has no plates. The batch lane strip sits between day headers and the first meal row.
- **Phone:** the board becomes a day list with a cook strip of pills instead of lanes.
- **Content width:** detail pages centre at `max-w-5xl` with 16 px side padding on phone, 32 px from 768 px.
- **Verification widths:** 390 px phone, 900 and 1024 px tablet, 1440 px desktop, each in light and dark, English and German (German runs longer).

## Elevation & Depth

Hybrid: hairline plus a whisper. Surfaces are separated by a 1 px `line` border; cards, the cook-strip pills, and the panels above the board add a 1 px whisper shadow. Overlays get the one deeper shadow. Depth never uses colour.

### Shadow Vocabulary

- **Card whisper** (`--shadow-card`, utility `shadow-card`: `0 1px 2px rgb(24 32 27 / 0.05)`): plate cards, review panel, empty-week panel, cook-strip pills.
- **Soft sheet shadow** (`--shadow-soft`, utility `shadow-sheet`: `0 2px 6px -2px rgb(24 32 27 / 0.08), 0 12px 32px -8px rgb(24 32 27 / 0.16)`; dark `0 2px 6px -2px rgb(0 0 0 / 0.3), 0 12px 32px -8px rgb(0 0 0 / 0.45)`): sheets, popovers, the assistant overlay.

**The Whisper Rule.** Cards carry at most the 1 px whisper; only overlays carry the soft sheet shadow. No coloured halos, no hover lift, no hard offset shadows.

## Shapes

Radius is assigned by role:

- **Cell and card** (`rounded-cell`, 10 px): plate cards, inputs, the quick-action buttons on a card.
- **Button** (`rounded-lg`, from `--radius` 0.5rem = 8 px): buttons, nav items, the week stepper, the card thumbnail.
- **Panel** (`rounded-xl`, 1.4 × `--radius` = 11.2 px): the review and empty-week panels above the board. The UX spec rounds this to 12 px; the build computes 11.2.
- **Sheet** (16 px): sheets and the phone tab bar, top corners only.
- **Pill** (`rounded-tag`, full): batch pills and cook-strip chips.

**Icons:** Lucide at a 1.5 px stroke (the cell padlock and attention triangle use 2 px), 16 px or 20 px, `aria-hidden` next to a text label or carrying an `aria-label`. No emoji in the UI.

## Components

Built on shadcn/Radix primitives (sheet, dialog, popover, tabs, dropdown-menu, tooltip), sonner toasts, cmdk and dnd-kit. Components below are the ones redesigned in this world.

### Buttons

- **Shape:** 8 px radius, 32 px tall by default (`components/ui/button.tsx`); 44 px on touch in the plan toolbar and empty-week panel, 36 px with a fine pointer.
- **Primary:** accent fill, on-accent text; at most one per screen region (Accept and shop, Plan this week). **Outline, secondary, ghost** use the neutral layers. **Destructive** is a danger-tinted wash with danger text. **Link** is accent text.
- **Focus:** the global 2px accent outline. **Motion:** colour changes only.

### Navigation (`SideNav`)

Muted ink at rest; hover moves to `panel-sunken` with ink text; active is accent text, semibold, on a 10% accent wash with `aria-current="page"`. Lucide icons 20 px at 1.5 px. The collapsed rail shows a 40 px accent tile with a leaf icon as brand.

### Plan card (`PlateCell`)

- **Size and shape:** about 84 px tall (`min-h-[5.25rem]`), 10 px radius, 8 px padding (10 px once the cell is 7 rem or wider).
- **Content:** a 30 px rounded thumbnail (the food photo, or a square tinted by a name-hashed batch tint with the food's initial in bold muted ink) beside a two-line semibold small name; the bottom row holds the batch pill and a truncating caption (side dish or subline). The thumbnail hides when the cell is narrower than 7 rem.
- **Surfaces:** plain or leftover plate: panel, hairline, card whisper. **Cook cell:** batch tint background, hairline. **Draft:** dashed border (`ink-muted` at 50 %) in place of the hairline and no whisper. **Attention:** 1.5 px dashed warning border with a warning triangle top right. **Confirmed:** solid hairline and a 12 px padlock top right. **No plate:** transparent with a dashed hairline and muted "No plate". **Empty:** a muted plus and "Add"; hover moves to `panel-sunken`. **Drop target:** accent border over a 10% accent wash.
- **Hover quick actions** (fine pointers, drafts only): Replace and Different side, 28 px icon buttons at the bottom right, out of the tab order (keyboard has R and S).
- **Assistant pulse:** a cell the assistant just changed flashes a 2px accent outline once (200 ms); off under reduced motion.

### Batch pills (`BatchBadge`)

A full pill in caption semibold with tabular numerals. **Cook** is solid: batch colour fill, `on-accent` text ("Cook × 4"). **Leftover and frozen** are outlined: 1.5 px batch-colour border, panel fill, batch-colour text ("2/4", "2/4 · frozen" with a 12 px snowflake). **Surplus** extras ("2 spare", "2 to freeze") use a 1 px dashed outline. In a board cell narrower than 7 rem the cook pill compacts to "× N" (the tint and the filled lane dot already say cook); the full text stays for screen readers. Leftover and frozen pills carry a spoken form ("Portion 2 of 4") instead of the bare fraction.

### Batch lane strip (`BatchLanes`) (signature)

Under the day headers, one 1.5 rem lane per batch, positioned over the space the board reserves. Each lane is a button inside the "Cook days" labelled list: the recipe name in the batch colour (caption semibold, truncating, sticky to the meal column on ground so lines scroll under it), then a 3 px rounded line in the batch colour from the cook day to the last leftover day, centred on the day columns. A filled 12 px dot marks the cook day and hollow 11 px dots (2.5 px batch-colour ring on panel) mark each leftover day. Hover or focus thickens the line to 4 px and underlines the label; click reveals the cook cell. Lanes render only from 768 px; the phone shows the cook strip instead.

**The Linked Highlight Rule.** Pointing at or focusing any card, pill or lane outlines every cell of that batch in its colour (a 2 px ring offset 2 px over ground, fading in over 200 ms), keeps its lane at full strength, and dims the other lanes to 30 % and the other cells to 50 %. On touch it holds for 2 s. Reduced motion makes it instant.

### Cook strip (`CookStrip`)

The phone version of the lanes: a "Cook days" list of 44 px pills ("Sun · Chili × 4", plus dashed extras) on panel with a hairline and the card whisper; scrolls sideways on phone and wraps from 768 px. Activating one scrolls to and highlights the cook cell.

### Day meter (`DayMeter`)

A 4 px rounded bar on a `line` track that runs to 125% of the kcal target, with a 1 px `ink` tick at the target (80%): success fill from 70% to 115%, warning fill outside. With no target there is nothing to measure, so the day's kcal shows as muted caption text with tabular numerals instead of a bar. A 6 px warning dot marks an incomplete day. It is a `meter` with value text and opens the nutrition popover from a full-size 44 px (32 px in the board header) button stretched over it.

### Review and empty-week panels

Panel background, hairline, 11.2 px radius, card whisper. The **review panel** (drafts waiting, hidden below 768 px) holds a muted "Draft" chip on `panel-sunken`, a semibold count, a wrapping hint (own line below 1280 px) and Discard drafts (outline) plus Accept and shop (primary). The **empty-week panel** matches it: h3 title, muted body capped at 52ch, Plan this week (primary), Copy week (outline) and "plan by hand" (link).

### Other screens

Plate sheet, assistant panel, shopping, cook mode, recipes, discovery and settings components exist and use the tokens, but have not been redesigned for this world (see the Overview gap). Their behaviour rules (44 px touch targets, undo over confirm, skeletons not spinners) are in UX spec §5 and §6.

### Interaction patterns (spec §6)

- Sheets for editing in context; dialogs only for destructive confirmations and the plan wizard; never stack two overlays.
- Prefer undo to confirm: reversible actions run immediately with a 6 s undo toast.
- Skeletons in place of content, not centred spinners.
- Errors state what happened and what to do, with no apology.

### Focus

One indicator everywhere: a base `:focus-visible` rule draws a 2px solid accent outline with a 2px offset. Nav items draw an equivalent 2px accent ring.

### Motion

150 to 250 ms ease-out, only for state change: sheets slide in, a plate cross-fades on swap, the linked highlight fades in, lane lines thicken on hover. No page-load choreography and no hover animation on cards. Under `prefers-reduced-motion` a global rule collapses transitions to near zero, runs animations once at 150 ms and removes slide offsets; `.animate-spin` is exempt, and the assistant cell pulse is off.

### Voice

A plain, calm tool. Short factual sentences, sentence case, active verbs. No exclamation marks (except the 10-recipe milestone toast), no "Oops", no apologies. A toast names the result using the control's verb. Every string exists in English and German, written natively. Vocabulary: plate, batch, leftover, no plate, routine, draft and confirmed, assistant (never "AI", "agent" or "LLM" in labels). See spec §7.

### shadcn names

`index.css` maps each shadcn theme variable onto a Plantry token in both themes (the table in the file's comment is authoritative): `background` to **ground**; `card` and `popover` to **panel**; `secondary` and `muted` to **panel-sunken**; `foreground` to **ink**; `muted-foreground` to **ink-muted**; `border` and `input` to **line**; `primary` and `ring` to **accent**; `primary-foreground` and `accent-foreground` to **on-accent**; `destructive` to **danger**. App code uses the Plantry names (`bg-ground`, `bg-panel`, `text-ink`, `border-line`, `text-accent`, `text-on-accent`).

- **The Accent Collision.** `--accent` is both the Plantry accent and shadcn's hover/focus fill, so menu and select highlights render solid accent. Text on any accent fill must be `text-on-accent`.

## Do's and Don'ts

### Do:

- **Do** use the token names in every new or rebuilt component.
- **Do** keep one accent: green for actions, focus, selection, today and links.
- **Do** use batch colours only for lanes, lane labels, pills and the cook-cell tint, and always with text.
- **Do** let weight and size carry hierarchy in Figtree; set numbers with `tabular-nums`.
- **Do** separate cards with a hairline and at most the 1 px whisper shadow.
- **Do** mark drafts with a dashed border and confirmed plates with a padlock.
- **Do** give every interactive element a visible 2 px accent focus outline; targets are at least 44 x 44 px on touch and 24 px otherwise.
- **Do** keep text and pill pairs at WCAG AA (4.5:1 text, 3:1 large text and UI marks); the token test enforces the core pairs.
- **Do** pair every state colour with an icon or a word, and design for German string length.

### Don't:

- **Don't** reintroduce a serif, a second typeface, all-caps or letter-spaced labels.
- **Don't** use batch colours for state, category or decoration, or tell batches apart by colour alone.
- **Don't** add gradients, glass or blur, shimmer, hover lifts or zooms, coloured halos or hard offset shadows.
- **Don't** make tall vertical photo cards on the board; keep cards compact (about 84 px).
- **Don't** put a shadow on overlays and a second shadow style on cards; use the two defined shadows only.
- **Don't** use emoji in the UI or centred spinners.
- **Don't** use the shadcn names in app code, or add a per-macro or chart palette.
