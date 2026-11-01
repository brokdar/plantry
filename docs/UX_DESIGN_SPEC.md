# Spec: Plantry UX and Visual Design

This document defines **how Plantry looks and works for the user**: which screens exist, how each workflow is laid out on desktop, iPad and phone, the visual system, the interaction patterns, and the voice. It is the UX companion to [PLANNING_ASSISTANT_SPEC.md](PLANNING_ASSISTANT_SPEC.md), which defines the behaviour and data model. Where they overlap, the planning spec owns *what happens* and this document owns *how it is presented*.

- Built from a design interview with the product owner on 2026-10-25. It follows Impeccable's `shape` and Operate-mode guidance and Anthropic's frontend-design guidance (both in `.claude/skills/`).
- Reference mockup: [design/visual-directions.html](design/visual-directions.html). Open it in a browser; the **Chosen** section is the target.
- **This is a redesign.** The current *Botanical Atelier* look (sage green, Manrope/Inter) and the current page structure are replaced. Product truth, data and functionality carry over.

---

## 1. Design brief

| | |
|---|---|
| **Who** | A home cook who dislikes planning but needs a plan to shop, and who often cooks once for several days. Single user, or a household sharing one install. |
| **When** | Saturday morning planning session (10–15 min, usually desktop); quick glances during the week (phone); in the supermarket (phone, one hand); at the stove (phone, wet hands). |
| **Job** | Get from "blank week" to an accepted plan plus shopping list with as little decision-making as possible, then know each day what to cook or reheat. |
| **Mode** | **Operate**: a working tool. Familiar patterns, scannable, calm. The identity lives in type, palette and one signature detail, never in unusual controls. |
| **Success** | Saturday: availability → generate → 2–3 tweaks → accept, in under 5 minutes of user effort. Weekdays: Today answers "what now?" in one glance. |
| **Unique truth** | Plantry thinks in **batches**: cook once, eat several times. No other planner shows that structure, so it is the design's signature (§4). |
| **Anti-goals** | A calendar app with recipes attached. Dashboards full of numbers. Chat as the main interface. Many pages doing the same thing. Decorative motion. |

### 1.1 Device roles

| Screen | Primary device | Must also work well on |
|---|---|---|
| Plan (Saturday flow, review) | Desktop / laptop | iPad (landscape and portrait), phone |
| Recipes (library, editing, import) | Desktop | iPad, phone (browse and quick edit) |
| Today | Phone | iPad, desktop |
| Cook mode | Phone | iPad (kitchen stand) |
| Shopping list | Phone | everything |
| Setup wizard, Settings | Desktop | everything |

"Must also work well" means complete functionality, not a stretched or reduced view. Every flow can be done end to end on a phone, and touch is a first-class input on iPad and phone.

### 1.2 Design principles

1. **One obvious next action per screen.** Today has "the thing to do now"; Plan has "Accept and shop"; Shopping has the next unticked item.
2. **Batches are visible everywhere.** A plate always says whether it is cooked here or a leftover, and from which batch.
3. **Summary first, detail on tap.** Nutrition, reasons and macros live one tap away, never in the default view.
4. **The assistant acts in place.** Inline actions on plates do most of the work; the chat is a side panel for anything bigger.
5. **Nothing silently changes what you bought.** Confirmed plates show a padlock. Anything that would alter them asks first (planning spec §3.5).

---

## 2. Information architecture

### 2.1 Navigation

| | Phone (<768 px) | iPad / desktop (≥768 px) |
|---|---|---|
| Primary | Bottom tab bar: **Today · Plan · Recipes · Shop** | Left rail with the same four destinations, with labels |
| Settings | Profile/gear icon in the Today header | Bottom of the rail |
| Assistant | Inside Plan (and the chat sheet); not a destination | Same |
| Command palette | — | `⌘K` / `Ctrl+K`, kept from today |

On first open of the day, the daily recipe suggestion appears as a sheet over Today (§5.9). It is never a page.

### 2.2 Screen inventory

| Screen | Route | Status vs. current app |
|---|---|---|
| Setup wizard | `/setup` | **New** |
| Today | `/today` (landing; `/` redirects) | **New**, replaces the planner as landing page |
| Cook mode | `/cook/:sessionId` | **New** |
| Plan | `/plan` (current/next window), `/plan?start=YYYY-MM-DD` | **Replaces** `/` planner grid, `/calendar` (week, month, agenda), `/day/$date`, `/archive` |
| Plan wizard steps | `/plan/new` (steps 1–3, then lands on `/plan`) | **New** |
| Shopping | `/shop` | **Promoted** from a panel to a destination |
| Recipes library | `/recipes` | **Replaces** `/components` |
| Recipe detail | `/recipes/:id` | **New**, a reading view, distinct from editing |
| Recipe editor | `/recipes/:id/edit`, `/recipes/new` | Rework of the component editor |
| Import | Sheet inside Recipes (`/recipes?add=url`) | **Replaces** the hidden `/import` route |
| Discover | Sheet inside Recipes (`/recipes?add=discover`) | **New** |
| Ingredients | `/recipes/ingredients` (a tab inside Recipes) | **Demoted** from a top-level page |
| Settings | `/settings/:section` | Restructured (§5.11) |
| Assistant insights | `/settings/assistant/insights` | **New** |

**Removed as destinations:**
- Calendar month and agenda views: past weeks are reached with the window arrows in Plan.
- `/day/$date`: replaced by Today with a date.
- `/archive` and `/presets`: presets become *routines*, edited in Settings → Meals and assigned in the availability grid.
- The separate Ingredients and Import nav entries.

Old routes redirect to their replacements.

### 2.3 Primary flows

```
First run:     Setup wizard ─▶ Recipes (guided, 0/10) ─▶ ... ─▶ 10 recipes ─▶ "Plan your first week" ─▶ Plan wizard
Saturday:      Today (banner "Plan next week") ─▶ Plan wizard (3 steps) ─▶ Plan review board ─▶ Accept ─▶ Shop
Weekday:       Today ─▶ (cook day) Cook mode │ (leftover day) done │ rate yesterday
In store:      Shop (sections, tick off)
Mid-week:      Plan ─▶ mark days "No plate" ─▶ "Re-plan from today" ─▶ review changes ─▶ confirm
New recipe:    Recipes ─▶ Add ▸ Import URL │ Discover │ Create manually ─▶ review ─▶ saved
Daily idea:    Today ─▶ suggestion sheet ─▶ Add to my recipes │ Not for me │ Another
```

---

## 3. Visual system: "Cookbook with batch badges"

**The world:** a well-used home cookbook. Serif headings like a recipe title, a quiet paper-white ground, ink-blue for actions, and the earthy colours of cooked food (rust, ochre, herb) for batches. From the *Market* direction it takes the **batch badge** placed directly on each slot. These replace all current tokens in `src/index.css`.

### 3.1 Colour tokens

Neutrals have a slight blue bias towards the ink accent, so they read as chosen rather than default grey.

| Token | Light | Dark | Use |
|---|---|---|---|
| `--ground` | `#FBFBFA` | `#161821` | Page background |
| `--panel` | `#FFFFFF` | `#1D2030` | Cells, sheets, cards with a role |
| `--panel-sunken` | `#F3F4F6` | `#12141C` | Rail, toolbars, second neutral layer |
| `--panel-raised` | `#FFFFFF` | `#252A3F` | Assistant panel surface |
| `--ink` | `#1C2233` | `#E6E8F0` | Primary text |
| `--ink-muted` | `#5A6175` | `#A3A8BB` | Secondary text, labels |
| `--line` | `#E3E5EA` | `#2E3245` | Borders, dividers |
| `--accent` | `#27407A` | `#9DB2EA` | Primary actions, focus ring, current selection, links |
| `--on-accent` | `#FBFBFA` | `#161821` | Text on accent |

**Batch palette.** Batches are assigned in order and cycle after six; a window rarely has more than four. Badge text uses `--ground` on the batch colour; tints are for the "cook here" cell and the Today card.

| Batch | Light (badge / tint) | Dark (badge / tint) |
|---|---|---|
| 1 Rust | `#7A3B2E` / `#F3E7E2` | `#E29A88` / `#35231F` |
| 2 Ochre | `#8A6A12` / `#F4EEDB` | `#D8B75A` / `#332B14` |
| 3 Herb | `#2F6B5A` / `#E1EFE9` | `#7CC4AD` / `#1B302A` |
| 4 Plum | `#6A3D6E` / `#EFE4F0` | `#C99BCD` / `#2E2131` |
| 5 Olive | `#5D6B2A` / `#ECEFDC` | `#B5C46E` / `#272C16` |
| 6 Slate | `#3D5A80` / `#E3EAF3` | `#93B0D6` / `#1C2533` |

**State colours** are separate from batch colours and used only where they carry meaning:

| State | Light | Dark |
|---|---|---|
| Success / on target | `#2E6B3F` | `#79C48F` |
| Warning / below target, needs attention | `#9A5B00` | `#E3A54F` |
| Danger / violation, destructive | `#A23A2A` | `#EE8B7B` |
| Info | uses `--accent` | |

All text and badge pairs meet WCAG AA (≥4.5:1 for text, ≥3:1 for large text and UI marks). Colour is never the only signal: badges always carry text ("Cook × 4", "2/4"), and states carry an icon or word.

### 3.2 Typography

| Role | Face | Use |
|---|---|---|
| Display | **Newsreader** (serif, opsz 6–72), weights 500/600, italic 400 | Page titles, recipe names on detail and cook mode, the Today headline, row labels on the board (italic), notes. **Never** for buttons, inputs, data or labels. |
| UI | **Source Sans 3**, weights 400/600 | Everything else: controls, plate names in cells, lists, data |
| Numbers | Source Sans 3 with `font-variant-numeric: tabular-nums` | Macros, quantities, counts |

Both are self-hosted via `@fontsource-variable` (no Google Fonts call from a LAN app). Manrope and Inter are removed.

**Scale:** fixed rem, ratio about 1.2. Body 15 px (0.9375 rem), small 13 px, caption 12 px, h3 19 px, h2 23 px, h1 28 px (Newsreader 600), and the Today headline 26 px on phone. Line length for prose (recipe notes, instructions) is capped at 68ch. Labels use sentence case; there are no all-caps labels.

### 3.3 Shape, spacing, elevation

- Spacing scale: 4 · 8 · 12 · 16 · 24 · 32 · 48 px. Layouts use `gap`, not margins.
- Radii by role:
  - board cells and inputs: 4 px
  - buttons: 8 px
  - sheets and the phone tab bar: 16 px (top corners only)
  - batch badges: `3px 10px 10px 3px`, a tag shape
  - Not everything gets the same radius.
- Elevation: none on cells. One soft shadow for sheets and popovers only. No shadow on cards in lists; the border carries separation.
- Icons: Lucide (already in use), 1.5 px stroke, 16/20 px. No emoji as UI.

### 3.4 Motion

- 150–250 ms ease-out, only for state change: sheets slide in, a plate swaps with a short cross-fade, a ticked shopping item slides down, and linked batch cells highlight.
- No page-load choreography and no hover animations on cards.
- `prefers-reduced-motion` disables slides (fade only).

---

## 4. Signature: batch badges

The one memorable thing in Plantry: every slot shows where its food comes from.

| Element | Appearance |
|---|---|
| **Cook cell** (the session's own slot) | Batch tint background, 1.5 px `--ink` inset outline, badge **"Cook × 4"** |
| **Leftover cell** | Plain `--panel`, badge **"2/4"** in the batch colour |
| **Frozen portion cell** | Plain panel, badge **"2/4 · frozen"** with a snowflake icon |
| **Cook strip** (above the board) | One badge per cook day, **"Sun · Chili × 4"**; tap to scroll to and highlight the batch |
| **Linked highlight** | Hovering (desktop) or tapping (touch) any badge outlines every cell of that batch for 2 s. This is the main way to understand a batch at a glance. |
| **Use-by** | Leftover cells on their last valid day add "use by today". Today shows "use by Wed" on the badge. |
| **Surplus** | A badge with a dashed outline in the cook strip: **"2 spare"** or **"2 to freeze"** |

Badges are the same component everywhere: board, Today, cook mode header, recipe detail ("last cooked as a batch of 4"), and the shopping list's "used in" expansion.

---

## 5. Screens

Every screen below lists its job, its layout per size, and the states it must handle. Sizes: **phone** <768 px; **tablet** 768–1199 px (iPad portrait and landscape); **desktop** ≥1200 px.

### 5.1 Setup wizard (`/setup`)

**Job:** get a working configuration in under two minutes by accepting defaults.

- Centred single column (max 560 px) on every size, with a step indicator. The steps are a real sequence, so numbering is honest here.
- Seven steps as in planning spec §7.1. Each step shows the defaults already selected, with a one-line explanation of why they matter for planning.
- The **availability grid step** is a compact weekday × meal grid. Tapping a cell cycles *Plan → No plate → Routine*; routines open a small picker.
- **Footer:** Back · **Next** (primary) · "Skip, use defaults" (text button, from step 2 on).
- **Finish screen:** "You're set up. Add 10 recipes and Plantry can plan your weeks." → **Add recipes**.
- **States:**
  - The AI step shows key validation inline ("Key works, 3 models available" / "Key rejected by Anthropic: check it was copied completely").
  - Without a key, a calm note: "You can add this later. Planning works without it."

### 5.2 Today (`/today`, landing)

**Job:** answer "what do I do today?" in one glance.

**Phone layout (primary):**
```
Monday, 2 November                       [gear]
Nothing to cook today                    ← Newsreader headline: the day's verdict
┌──────────────────────────────────────┐
│ [3/4 · use by Wed]                   │  ← batch badge
│ Dinner                               │
│ Chili sin carne, leftover            │
│ Reheat 1 portion · 610 kcal          │
└──────────────────────────────────────┘
Breakfast   Oats (routine)
Lunch       Chili, lunchbox  [2/4]
──────────
Tomorrow    Lunch is the last Chili
Freezer     Take curry out tonight        (only when relevant)
──────────
How was Sunday's Chili?   [Loved it] [Not again]
[Plan next week]  ← banner, only from the day before shopping day until planned
```

- **Headline verdicts** use exactly one of these:
  - "Cook Chili for 4 tonight"
  - "Nothing to cook today"
  - "Quick cook: Omelette"
  - "No plates planned today"
- **Cook day:** the main card becomes **Start cooking** (primary), which opens Cook mode. The card shows the scaled time estimate.
- **Rating:** one question at a time, for the most recent unrated eaten plate. It disappears after an answer, and an undo toast is available.
- **Tablet and desktop:** two columns. Left is the same content; right is "This week", a compact read-only strip of the 7 days with badges. Tap a day to open Plan at that day.
- **States:**
  - No accepted plan for today: "No plan for today" with **Plan this week**.
  - Draft only: "Your plan for this week is still a draft" with **Review plan**.
  - Before setup is complete: redirect to `/setup`.
  - Below 10 recipes: card "Add N more recipes to start planning" with **Add recipes**.

### 5.3 Cook mode (`/cook/:sessionId`)

**Job:** cook a batch from the phone with wet hands.

- Full screen, large type (body 19 px, step text 22 px Newsreader). Requests a screen wake lock and tolerates refusal.
- **Header:** recipe name, then the badge **"Cooking × 4"**. The portion stepper scales live and explains its effect: "Covers Sun dinner, Mon lunch, Mon dinner, Tue lunch."
- **Two tabs:**
  - **Ingredients:** scaled and tickable, with rounding hints ("1 can, using ⅔").
  - **Steps:** one step per screen. Swipe or tap Next/Previous, with large 56 px hit areas at the bottom.
- **Timers:** detected durations ("5 min", "20 Minuten") become a timer button inside the step. A running timer stays visible as a pill across steps.
- **"Freeze 2 portions"** reminder on the last step when `frozen_portions > 0`.
- **End:** **Done cooking** marks the session cooked and returns to Today.
- **Tablet:** ingredients and steps side by side.

### 5.4 Plan wizard (`/plan/new`)

**Job:** collect the few things only the user knows, then generate.

Three short steps. It opens as a full page on phone and as a centred panel over the dimmed board on tablet/desktop, so the week stays visible in the background.

1. **Who's home?** The window's weekday × meal grid, prefilled from defaults. Tap a cell to cycle Plan / No plate / Routine. Each day header has a menu: "Away all day", "Guests" (sets eaters for that day). A checkbox "Make this my new default". Copy: "Mark the meals you don't need. Everything else gets planned."
2. **Anything special this week?** One optional text field (placeholder: "e.g. guests on Friday, something light") and one toggle, "Include a new recipe", prefilled from settings. **Generate plan** is the primary action. **Skip** generates without a note.
3. **Generating.** A short checklist of real progress ("Reading your week · Choosing recipes · Checking batches and keep days · Saving draft"), then a cross-fade onto the review board. On failure: "The assistant didn't respond (timeout). Try again, or plan without the assistant." The second option runs the heuristic planner.

### 5.5 Plan review board (`/plan`)

**Job:** understand the week at a glance, fix what you don't like, accept.

**Desktop (≥1200 px):**
```
┌ rail ┐ Week of 31 Oct          Draft · 10 plates · 4 cook days    [Ask assistant] [Accept and shop]
│Today │ ‹ previous   next ›     1 plate needs attention
│Plan  │ Cook days: [Sat · Pasta × 1] [Sun · Chili × 4] [Wed · Curry × 3]   Thu · Omelette (quick)
│Recip.│ ───────────────────────────────────────────────────────────────────────────────
│Shop  │          Sat 31   Sun 1    Mon 2    Tue 3    Wed 4    Thu 5    Fri 6
│      │          ▬▬▬▬     ▬▬▬▬▬    ▬▬▬▬▬    ▬▬▬      ▬▬▬▬     ▬▬▬▬▬    ▬▬▬     ← day meters
│      │ Breakfast  Oats · routine (collapsed row when all routine)
│      │ Lunch    No plate  Bread&soup [2/4]   [4/4]   No plate  [2/3]   [3/3]
│      │ Dinner  [Cook×1]  [Cook×4]  [3/4]    No plate [Cook×3] Omelette No plate
│Sett. │                                                     ┌ assistant panel (on demand) ┐
└──────┘                                                     └─────────────────────────────┘
```

- **Grid:** days as columns (the window starts on the shopping day) and meals as rows. A row in which every cell is a routine collapses to one line ("Breakfast: Oats every day").
- **Day header:** date, plus a thin meter for kcal against target (success colour; warning colour below 70 % or above 115 %). Tap or hover for a popover with kcal / protein / fat / carbs against targets and per-meal contributions.
- **Cell content:** badge, plate name (main + "with rice" for sides), and a tiny muted line ("cook here", "leftover", "routine"). A *No plate* cell is a transparent italic "No plate" with its reason if given. Draft cells have a dashed border; confirmed cells a solid border plus a 12 px padlock.
- **Cell interaction:**
  - Click or tap opens the **plate sheet** (side sheet on desktop/tablet, bottom sheet on phone).
  - Desktop hover shows two quick actions on the cell: **Different side** and **Replace**.
  - Drag and drop moves a plate (desktop and iPad); holding `⌘`/`Ctrl` while dragging copies it.
- **Plate sheet contents:**
  - Name and batch badge.
  - Components, each with *swap* and *remove*.
  - Eaters stepper. For a batch: portions stepper with live coverage text, plus the freeze toggle.
  - Macros for this plate.
  - "Why this?": the assistant's one-line reason.
  - Actions: **Confirm**, **Replace**, **Different side**, **Regenerate day**, **Mark as No plate**.
  - Confirmed plates show **Unlock** instead of Confirm, with the warning text from planning spec §3.5.
- **Needs attention:** checker violations appear as a warning-coloured corner mark on the cell and as a count in the header. Clicking the count steps through them.
- **Toolbar:**
  - **Ask assistant** opens the chat panel.
  - **Accept and shop** confirms all drafts and goes to Shop. A short toast with undo follows: "Plan accepted. 10 plates confirmed."
  - An overflow menu holds *Regenerate week* (drafts only), *Discard draft*, *Re-plan from today* (mid-week), and *Copy a previous week*.
- **Window navigation:** previous/next arrows plus a "This week" chip. Past windows are read-only, apart from rating and "Copy to next week".
- **Tablet:**
  - Landscape (≥1024 px) shows the full 7-day grid.
  - Portrait shows 4 days at a time with horizontal swipe, snapping per day. Day headers stay sticky, and the cook strip scrolls with the days.
- **Phone:** a vertical list of days. Each day is a block with its meals as rows (badge + name), and the day meter sits in the block header. The cook strip is a horizontally scrolling row of badges at the top, and a sticky bottom bar holds **Accept and shop**. Tapping a meal opens the bottom plate sheet. Quick actions are inside the sheet; there is no hover.
- **States:**
  - Empty window, no draft: an inline invitation with **Plan this week** (starts the wizard) and "or plan by hand". A teaching empty state, not "nothing here".
  - Loading: skeleton grid.
  - Generation running: the board stays visible with an inline progress bar in the header.
  - Offline or AI unavailable: the assistant button is disabled with a tooltip, and the heuristic is offered.

### 5.6 Assistant panel

- From 1280 px a **non-modal** docked right panel (380 px) that pushes the board narrower, so changes are visible as they land. From 768 to 1279 px a non-modal overlay side sheet (380 px) over the board, which keeps its full width (no dialog role; Escape closes it and returns focus). Below 768 px a bottom sheet at 92 dvh with a grab handle.
- **Top:** "Changes go into your draft. Confirmed plates are kept unless you agree."
- **Suggestion chips** relevant to the window, such as "No fish this week", "Make the chili 6 portions, freeze 2", "Lighter dinners".
- **Tool activity** is shown as human sentences ("Replaced Tuesday's side with roast potatoes") with an *Undo* per change, not as raw tool names or JSON. The raw detail moves to Assistant insights.
- Changed cells pulse their outline once (200 ms).

### 5.7 Shopping (`/shop`)

**Job:** buy everything in one walk through the store.

- **Header:**
  - Range ("Sat 31 Oct to Fri 6 Nov", tappable; defaults per planning spec §7.7).
  - A toggle "Include draft plates", shown only when drafts exist.
  - A count "18 items · 4 ticked".
- **Sections:** grouped by store section in a fixed walk order: produce, bakery, dairy and eggs, meat and fish, dry goods, cans and jars, spices and oils, frozen, other. Each ingredient gets a section automatically (from its OFF/FDC category or name classification) and can be changed by long-press or the row menu.
- **Row:** a large checkbox (44 px target), name, quantity in shopping units ("3 pcs", "1 bunch", "500 g"), and an expandable "used in" list with batch badges. Ticked rows move to the bottom of their section with a strike-through. *Clear ticked* sits at the end.
- **In-store mode** for phone: wake lock, larger rows, one-hand reach (actions at the bottom).
- **Desktop:** a two-column section layout and a "Copy as text" button.
- **States:**
  - No accepted plan: "Accept a plan to see what to buy" with **Go to plan**.
  - Everything ticked: "All done. 18 items bought."

### 5.8 Recipes (`/recipes`)

**Job:** browse and maintain the personal cookbook; add new recipes easily.

- **Library:**
  - **Cards:** photo (4:3), recipe name (Newsreader), and one meta line: time, kcal per portion, diet icon, last eaten ("3 weeks ago" / "new" / "never").
  - **Layout:** 4 columns on desktop, 3 on tablet, 2 on phone.
  - **Filters** as a chip row: Role (Main, Side, …), Diet, Time (≤30 min), "Not eaten lately", Favorites, New. There is a search field, and Sort offers Name, Last eaten and Added.
  - **Missing photo:** a typographic placeholder (the recipe initial on the batch tint), never a stock image.
- **Add** (primary, top right) opens a menu: **Import from a link**, **Discover recipes** (when AI is configured), **Create manually**.
- **Guided state (fewer than 10 recipes):**
  - A progress row above the grid: "4 of 10 recipes. Plantry needs 10 to plan a varied week."
  - Three option cards explain each way to add recipes, in two sentences each.
  - They collapse into the Add menu once 10 is reached, with a one-time toast: "10 recipes. You're ready to plan your first week."
- **Tabs:** *Recipes* and *Ingredients*. Ingredients is a dense table: name, kcal/100 g, diet class, section, used in N recipes, and a data-source badge (OFF / USDA / manual).
- **Recipe detail** (`/recipes/:id`), for reading and cooking:
  - Hero photo and name, with the source link.
  - Facts row: time, portions stepper, kcal and protein per portion, diet, keeps N days, freezable.
  - Scaled ingredients.
  - Steps.
  - **My notes** in a distinct block with the user's own text; the imported description is shown muted below.
  - "Cooked 6 times, last 3 weeks ago."
  - **Actions:**
    - Primary: **Cook now**, which creates a one-off session for today and opens Cook mode.
    - Secondary: Edit, Add to plan, Favorite, Create variant.
- **Recipe editor:** a two-pane layout on desktop (fields left, live preview of the card and macros right) and a single column on phone. A nutrition-completeness warning names the incomplete ingredients and links to them.
- **Import sheet:**
  - Paste a link, then a review screen with sections for basics, ingredients and steps.
  - Each ingredient row shows its match status: matched (with name and data source), needs choice (candidate list), or new (create with nutrition).
  - **Save recipe** is disabled until every row is resolved. The counter explains why: "2 ingredients need a choice."
- **Discover sheet:** mode chips (Daily idea · Explore a cuisine · Use what I know), a cuisine field for Explore, then 3 suggestion cards in the same format as the daily popup (§5.9).

### 5.9 Daily suggestion

- **Appearance:** a sheet over Today on the first open of the day, with a large photo (16:9) at the top.
- **Content:**
  - Name (Newsreader), cuisine, time, kcal and protein per portion, diet icon.
  - A one-line "why": "Uses 7 ingredients you already know. New cuisine for you: Persian."
  - The ingredient list with **new ingredients highlighted**: a small "new" label in `--accent` next to the name, with a legend "new = not in any of your recipes yet".
  - The source link.
- **Actions:**
  - **Add to my recipes** opens the import review, prefilled.
  - **Not for me** has an optional one-tap reason (too long, ingredients, not my taste).
  - **Show another.**
- Dismissing with the close button or the Escape key counts as "not now", not as a rejection.
- If generation fails, the sheet silently does not appear. It never shows an error on app open.

### 5.10 Mid-week re-plan

From Plan's overflow menu, or by marking confirmed cells *No plate*:

1. A side or bottom sheet lists the changes needed: "Thu and Fri are now No plate. Chili has 2 spare portions." It offers *Freeze 2 portions* (if freezable) or *Move to later days*, and **Re-plan the rest of the week** (optional).
2. If any confirmed plates would change, they are listed ("These were already bought for") with **Change them** / **Keep them**.
3. The result arrives as a draft overlay on the board with the usual review and Accept.

### 5.11 Settings (`/settings/:section`)

Desktop/tablet use a section list on the left with content on the right. On phone, the list drills down into each section.

| Section | Contents |
|---|---|
| You & household | Diet type, household portions helper, allergies, nutrition goal and macro targets |
| Meals & week | Meal slots (rename, reorder, icons), default availability grid, **Routines** (former presets: named plates assigned to routine cells) |
| Cooking & variety | Cook occasions per day, batch cooking, default keep days, freezing, variety weeks, new recipes per week, shopping day |
| Assistant | Provider, model, key (with test), notes to your assistant, daily suggestion on/off, cuisines to explore, prices; link to **Insights** |
| Data & system | Language, FDC key, re-run setup wizard, about/version |

Every setting shows its effect in one line ("Plantry plans at most 1 cooking session per day"). Changes save on blur with a quiet "Saved" note; there is no global Save button.

**Assistant insights:**
- A runs table: date, kind, model, prompt version, tokens, estimated cost, status, quality score, outcome.
- A small trend chart per metric: correction rate, acceptance rate, cost per accepted plan.
- Clicking a run opens its steps in plain language, with the raw JSON behind a disclosure.

---

## 6. Interaction patterns

| Pattern | Rule |
|---|---|
| Sheets vs dialogs | Use sheets for editing in context (plate, import, chat). Use a dialog only for destructive confirmations (discard draft, unlock bought plates, delete recipe) and the plan wizard. Never stack two overlays. |
| Undo over confirm | Reversible actions run immediately with a 6 s undo toast (remove side, mark No plate, tick item). Only irreversible or purchase-affecting actions ask first. |
| Feedback | Every action names its result in the toast, using the same verb as the control: "Accept and shop" → "Plan accepted". |
| Touch targets | ≥44 × 44 px on touch devices. Hover-only affordances must also exist in the tap sheet. |
| Keyboard (desktop) | Arrow keys move between cells; `Enter` opens the plate sheet; `R` replaces; `S` changes the side; `N` marks No plate; `A` opens the assistant; `⌘/Ctrl+Enter` accepts. Shortcuts are listed in the `?` overlay. |
| Focus | 2 px `--accent` outline with a 2 px offset on every interactive element. Sheets trap focus and return it to the cell. |
| Loading | Skeletons in place of content; no centred spinners. The AI shows real progress steps. |
| Errors | State what happened and what to do, with no apology: "Couldn't load chefkoch.de (blocked by the site). Paste the recipe text instead." |
| Empty states | Teach the next step and offer the action; never a bare "nothing here". |
| Offline | The API is on the LAN; only AI and lookups go online. Those controls disable with "Needs internet" and everything else keeps working. |

---

## 7. Voice and vocabulary

- **Plain, calm tool.** Short factual sentences, sentence case, active verbs. The assistant reports facts ("Planned 10 plates, 4 cook days. 1 plate needs attention.") and explains only when asked ("Why this?").
- **No exclamation marks** except the 10-recipe milestone toast. No "Oops", no apologies.
- **Bilingual:** every string in en and de. German copy is written natively, not translated word for word (e.g. "Kein Essen nötig" rather than "Kein Teller").

| User-facing term (en / de) | Meaning | Avoid |
|---|---|---|
| Plate / Mahlzeit | A planned meal in a slot | "Component plate", "cell" |
| Batch / Vorkochen (badge "Cook × 4" / "Koche × 4") | A cook session | "Session", "occasion" |
| Leftover / Rest | A plate eaten from an earlier batch | — |
| No plate / Kein Essen | Availability *none* | "Skipped", "Eating out" |
| Routine / Routine | Recurring plate from a saved template | "Preset", "template" |
| Draft · Confirmed / Entwurf · Bestätigt | Plan state | "Locked" |
| Assistant / Assistent | The AI | "Agent", "LLM", "AI" in labels |
| Notes to your assistant | `profile.system_prompt` | "System prompt" |
| Recipe · Ingredient / Rezept · Zutat | Composed and leaf food | "Food", "component" |

---

## 8. Accessibility

- WCAG 2.2 AA: contrast as in §3.1, focus visible, target size ≥24 px (≥44 px on touch), no colour-only meaning.
- The board is a real `grid` with row and column headers. Each cell's accessible name reads in full, e.g. "Monday dinner, Chili sin carne, leftover 3 of 4, confirmed".
- Live regions announce assistant changes and generation progress.
- Text scales to 200 % without loss (cells grow taller; the board scrolls horizontally inside its own container).
- `prefers-reduced-motion` and `prefers-color-scheme` are honoured, with a theme override in Settings.

---

## 9. Implementation notes

- **Keep:** shadcn/Radix primitives (`sheet`, `dialog`, `popover`, `tabs`, `dropdown-menu`, `tooltip`), sonner toasts, dnd-kit, cmdk, TanStack Query/Router, react-hook-form/zod, i18n setup.
- **Replace:** all tokens in `src/index.css` (Botanical Atelier) with §3. The fonts become Newsreader and Source Sans 3 via `@fontsource-variable`. A one-time `$impeccable document` pass afterwards records the new system in a root `DESIGN.md`.
- **New shared components:**
  - `BatchBadge`, which owns the colour assignment and linked highlight
  - `PlateCell` (the board cell in all states)
  - `DayMeter`
  - `PlateSheet`
  - `CookStrip`
  - `AvailabilityGrid` (used in setup, the plan wizard and settings)
  - `SuggestionCard` (daily popup and discover)
  - `ShoppingRow`
  - `StepPager` (cook mode)
- **One board, three layouts:** desktop/tablet grid and phone day list render from the same view model. This replaces the two diverging implementations (`PlannerGrid.tsx` 975 lines, `MobilePlannerGrid.tsx` 745 lines).
- **Remove with their routes:** calendar views, `ReadOnlyPlannerGrid`, `RevertBanner`, AI fill-session UI, chat mode selector, `/archive`, `/presets` page, the `onSaveAsTemplate` dead prop, and the mobile swap placeholder.
- **Phasing:** the UI work rides on the planning spec's phases.
  - Phase 0: new tokens and fonts, nav restructure, recipe detail/import fixes.
  - Phase 1: setup wizard, recipes guided state, availability grid.
  - Phase 2: Today, cook mode, Shop, board with batch badges.
  - Phase 3: plan wizard, review board actions, plate sheet.
  - Phase 4: assistant panel.
  - Phase 5: discover and daily suggestion.
- **Verification per screen:** an `$impeccable critique` and `audit` pass, plus the project's `frontend-review` skill, at phone (375 px), iPad portrait (820 px), iPad landscape (1180 px) and desktop (1440 px), in light and dark.

---

## 10. Decisions log (interview 2026-10-25)

| Topic | Decision |
|---|---|
| Devices | Plan mostly on desktop; everything must also work on iPad and phone. Today, Shop and Cook are phone-first. |
| Saturday flow | Three short guided steps, then the week board for review |
| Assistant | Inline plate actions plus a chat panel on demand |
| Batch visualisation | Grid with colour-linked batch badges and a cook-days strip |
| Today | "What to do now" first |
| Shopping | Grouped by store section, tick off |
| Recipe library | Photo cards with key facts |
| Visual direction | B "Cookbook" (serif headings, paper ground, ink accent) with A's batch badges on the slots |
| Nutrition | At a glance (day meter); details on tap |
| Cooking | Dedicated cook mode |
| Voice | Plain, calm tool |
| Navigation | 4 tabs (Today, Plan, Recipes, Shop); Settings behind an icon or at the bottom of the rail |

## 11. Resolved decisions (defaults accepted 2026-10-25, may be overridden)

1. **Store section order:** fixed for now, using the walk order in §5.7. It becomes reorderable in Settings only if requested. (Decided 2026-10-26; the original question stated no default.)
2. **Week start column:** the board starts on the shopping day (Saturday for this user), matching the planning window. There is no Monday-first board with a highlighted window.
3. **Photos for manual recipes:** uploading from the phone camera roll is allowed in the recipe editor, using the existing image crop UI.
