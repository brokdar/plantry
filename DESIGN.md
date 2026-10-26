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

The system replaced the earlier *Botanical Atelier* look (sage green, Manrope/Inter, gradients, glass). Some of that look survives in legacy aliases and unrebuilt screens (see Legacy aliases under Components); it is not part of this system.

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

Six earthy colours, each with a solid badge colour and a pale tint: **1 Rust, 2 Ochre, 3 Herb, 4 Plum, 5 Olive, 6 Slate**. Tokens and utilities exist (`batch-N`, `batch-N-tint`). *Implemented.* The component that assigns and renders them (`BatchBadge`) is *specified*. Until then the only use is the recipe detail placeholder image (Herb tint with a Herb serif initial).

**The Batches-Only Rule.** Batch colours mean "this food comes from batch N". Never use them for state, decoration, or category colour. Batches take colours in order and cycle after six.

**The Tint-Means-Cook Rule.** A batch tint background marks the slot where the batch is cooked (and the Today card). Leftover slots stay plain panel and show the batch colour only in the badge. Badge text is `ground` on the batch colour.

### State colours

- **Success** (`success`): on target.
- **Warning** (`warning`): below target, or needs attention (for example, the nutrition completeness warning's triangle icon).
- **Danger** (`danger`): violations and destructive actions. shadcn `destructive` resolves to it.

**The Meaningful State Rule.** State colours appear only where they carry meaning, and always with an icon or a word. They are separate from batch colours and must never be used to tell batches apart.

### Outside the Cookbook palette (implemented, legacy)

`index.css` still defines a **macro palette** (`macro-protein`, `macro-carbs`, `macro-fat`, `macro-fiber`, each with a `-soft` variant; terracotta, wheat, sage and bark) and **chart tokens** `chart-1`…`chart-5`. These come from Botanical Atelier, are not in UX spec §3.1, and are not checked by the token test. They remain in use on nutrition surfaces. Do not extend them. Their future is an open question for the screens that show macros.

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
- **Shell** (*implemented*): below 768px, a fixed bottom tab bar (Today, Plan, Recipes, Shop) on panel with rounded top corners and a 4rem height plus safe-area inset (`--tab-bar-h`, `--tab-bar-inset`). Main content reserves that height as bottom padding. From 768px up there is a sunken left rail: an 80px icon rail at 768–1023px and when collapsed, and a 256px labelled rail from 1024px. Settings sits at the bottom of the rail; on phone it is a gear in the top bar.
- **Content width:** detail pages centre at `max-w-5xl` with 16px side padding on phone and 32px from 768px up. Two-column detail layouts use a 12-column grid (for example 5/7 for the header and 4/8 for ingredients and steps).
- **Verification widths:** 375px phone, 820px iPad portrait, 1180px iPad landscape, 1440px desktop, each in light and dark.

## Elevation & Depth

Flat by default. Depth comes from the two neutral layers (`panel` on `ground`, `panel-sunken` for chrome) and hairline borders.

### Shadow Vocabulary

- **Soft sheet shadow** (`--shadow-soft`, utility `shadow-sheet`): two-layer ambient shadow tinted with ink (deeper black in dark mode). For sheets and popovers only. *Token implemented*; no rebuilt component uses `shadow-sheet` yet.

**The One Shadow Rule.** Board cells and list cards get no shadow; the border carries separation. The only shadow is the sheet shadow, on overlays.

## Shapes

Corner radius is assigned by role, so not everything gets the same curve:

- **Cell** (`rounded-cell`): board cells and inputs.
- **Button** (`rounded-lg`, from `--radius`): buttons, nav items, callouts, recipe images.
- **Sheet** (`rounded-sheet`): sheets and the phone tab bar, top corners only. The tab bar uses `rounded-t-2xl` today, which is the same 16px.
- **Tag** (`rounded-tag`): batch badges, an asymmetric tag silhouette that is square at the left and round at the right.

The `rounded-cell`, `rounded-sheet` and `rounded-tag` utilities are *implemented* but not yet used by any component. Some legacy screens still use `rounded` (also 4px) or larger radii.

**Icons:** Lucide at a 1.5px stroke (`strokeWidth={1.5}`), 16px or 20px (`size-4`, `size-5`), always `aria-hidden` next to a text label or an `aria-label`. No emoji in the UI.

## Components

Built on shadcn/Radix primitives (sheet, dialog, popover, tabs, dropdown-menu, tooltip), sonner toasts, cmdk and dnd-kit. Screens rebuilt in Phase 0 that show the system: the shell (`components/shell/*`), recipe detail (`RecipeDetail.tsx`), `NutritionCompletenessWarning.tsx`, and the import summary and resolve rows (`components/import/*`).

### Buttons

- **Shape:** button radius, 32px tall by default (`components/ui/button.tsx`).
- **Primary:** accent fill with on-accent text. **Outline / secondary / ghost** use the neutral layers. **Destructive** is a danger-tinted fill with danger text.
- **Focus:** a ring in the accent colour.
- **Motion:** the button variants still carry Botanical hover lifts and a sage-tinted hover shadow. These are legacy, not part of this system; see Do's and Don'ts.

### Navigation (implemented)

- **Items:** muted ink at rest. Hover moves to a `panel` background with ink text. Active is accent text, semibold, on a 10% accent wash, with `aria-current="page"`. Lucide icons are 20px at a 1.5px stroke.
- **Focus:** a 2px accent ring on every item.
- **Touch:** tab bar links are at least 44px tall.

### Callouts (implemented: `NutritionCompletenessWarning`)

Panel background, hairline border, button radius, 12px by 16px padding. A warning-coloured triangle icon leads a semibold title and a muted body. Inline links are accent and underlined, and the underline is removed on hover. The callout uses `role="status"`.

### Recipe detail (implemented)

A serif h1 name. Meta values use h3-size tabular sans over small muted labels. The ingredient list has hairline dividers and right-aligned tabular quantities. Steps are numbered in muted tabular sans, capped at 68ch. Notes sit in a sunken panel block. With no image, the placeholder is a Herb-tint tile with a large serif initial.

### Batch badge and plate cell (specified, Phase 2)

`BatchBadge` will own the colour assignment and the linked highlight. It will be the same component on the board, Today, cook mode, recipe detail and the shopping list.

- **Cook cell:** batch tint, a 1.5px ink inset outline, badge "Cook × 4".
- **Leftover cell:** plain panel, badge "2/4" in the batch colour.
- **Frozen:** "2/4 · frozen" with a snowflake icon.
- **Surplus:** a dashed-outline badge, "2 spare".
- **Linked highlight:** hovering or tapping a badge outlines every cell of that batch for 2s.

Also specified: `PlateCell`, `DayMeter`, `PlateSheet`, `CookStrip`, `AvailabilityGrid`, `SuggestionCard`, `ShoppingRow` and `StepPager`. See UX spec §4 and §9.

**The Never-Colour-Alone Rule.** A badge always carries text ("Cook × 4", "2/4"). Colour alone never identifies a batch or a state.

### Interaction patterns (spec §6, applies to all new work)

- Use sheets for editing in context. Use dialogs only for destructive confirmations and the plan wizard. Never stack two overlays.
- Prefer undo to confirm: reversible actions run immediately with a 6s undo toast.
- Show skeletons in place of content, not centred spinners.
- Errors state what happened and what to do, with no apology.

### Motion

150–250ms ease-out, and only for state change: a sheet slides in, a plate cross-fades on swap, a ticked item slides down, linked batch cells highlight. There is no page-load choreography and no hover animation on cards. Under `prefers-reduced-motion`, slides become fades. *Partly implemented:* nav uses 150–200ms colour and width transitions. There is no global reduced-motion rule in `index.css` yet; only `AnimatedNumber` checks the preference.

### Voice

A plain, calm tool. Short factual sentences, sentence case, active verbs. No exclamation marks (except the 10-recipe milestone toast), no "Oops", no apologies. A toast names the result using the control's verb ("Accept and shop" gives "Plan accepted"). Every string exists in English and German, and the German is written natively. Vocabulary: plate, batch, leftover, no plate, routine, draft and confirmed, assistant (never "AI", "agent" or "LLM" in labels). See spec §7.

### Legacy aliases

`index.css` remaps the shadcn and Material names from Botanical Atelier onto the Cookbook tokens, in both themes (the table in the file's comment is authoritative). The main mappings:

- `background` and `surface` resolve to **ground**.
- `card`, `popover` and `surface-container-lowest` resolve to **panel**.
- `secondary`, `muted`, `sidebar` and `surface-container(-low)` resolve to **panel-sunken**.
- `foreground` and `on-surface` resolve to **ink**.
- `muted-foreground`, `on-surface-variant` and `outline` resolve to **ink-muted**.
- `border`, `input`, `outline-variant` and `surface-container-high(est)` resolve to **line**.
- `primary`, `ring`, `tertiary` and `ai-accent` resolve to **accent**.
- `primary-foreground`, `on-primary` and `accent-foreground` resolve to **on-accent**.
- `destructive` resolves to **danger**.
- `shadow-editorial` resolves to **shadow-soft**.

The legacy utilities `gradient-primary`, `editorial-shadow`, `glass-header` and `btn-shimmer` still exist. `gradient-primary` is now a solid accent fill.

- New and rebuilt code uses the Cookbook names: `bg-ground`, `bg-panel`, `text-ink`, `border-line`, `text-accent`, `text-on-accent`, and so on.
- Legacy names stay until each screen is rebuilt in Phases 1–3. Do not add new uses of them.
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
- **Don't** add gradients, glass or blur, shimmer sweeps, hover lifts, or sage-tinted shadows. These are Botanical Atelier leftovers still found in `GeneratePlanButton`, `components/ui/button.tsx` and `TopBar`'s `glass-header`.
- **Don't** add shadows to cells or list cards, or a second shadow style.
- **Don't** use all-caps or letter-spaced labels.
- **Don't** use emoji in the UI.
- **Don't** use centred spinners.
- **Don't** add new uses of the legacy Material or shadcn aliases, or extend the macro and chart palettes.
