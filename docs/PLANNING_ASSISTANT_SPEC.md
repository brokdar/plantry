# Spec: Planning Assistant

Turn Plantry from a manual plan editor with a bolted-on chat into an assistant that plans the week for the user. The user states availability and preferences; the app produces a checked, reviewable draft built around batch cooking; the user adjusts and accepts; the shopping list and a daily "cook today" view follow.

This document is the source of truth for **what** changes, starting from the codebase as of commit `f66e939`. It describes target behaviour, data-model changes, API surface, removals, measurement, and a phased delivery plan in which the app stays usable after every phase. Implementation details (exact SQL, component structure) are left to the per-phase plans.

---

## 1. Summary

| Area | Today | Target |
|---|---|---|
| Availability | A per-slot "Eating out" fake plate (`plates.skipped`) | A weekday × slot default pattern plus per-date overrides; each cell is *plan*, *routine* or *none* |
| Cooking | Every plate is independent | **Cook sessions**: cook recipe X on day D for N portions; plates eat from sessions (fresh or leftover) |
| Portions | Integer portions per component; household size inflates the user's kcal | Recipe default yield → session portions (adjustable by user/agent) → per-eater portions × plate eaters |
| Generation | Chat agent mutates the live plan via 80–100 tool calls against a 20-iteration cap, with no dates in its prompt | Context assembled in Go → one structured plan proposal (LLM or heuristic) → Go checker → stored as a **draft** |
| Review | Revert only the plates the AI created | Confirm per plate, regenerate plate/day/side, chat on the draft, accept or discard |
| Daily use | No "today" view | `/today`: what to cook, what to eat, leftovers, freezer reminders, one-tap feedback |
| Setup | Empty grid, no slots, AI key save fails without `PLANTRY_SECRET_KEY` | Setup wizard with defaults → guided recipe onboarding (10-recipe target) |
| Recipes | Import yields 0-kcal ingredients; no source URL; one notes field | Fixed import, source URL and image stored, description separate from personal notes, planner attributes |
| Discovery | — | Agent researches new recipes online (daily suggestion, cuisine exploration, "use what I know") |
| Measurement | Nothing persisted beyond chat message blobs | Run log, plate audit trail, deterministic quality score, offline eval suite, AI insights page |

---

## 2. Why

### 2.1 User problem

Every Saturday the user plans the next week so they can shop. They dislike planning. What they do by hand today:

1. Work out which meals they actually need. A slot needs no plate when the user eats at a restaurant or is away. Location is irrelevant: food taken to the canteen *does* need a plate.
2. Plan so they cook at most once a day. A large Sunday batch covers 4–5 days, and ideally there is always one prepared meal available.
3. Keep a balanced diet: macro targets plus diet type, with personal rules such as "meat at most once a day, 2–3 days a week".
4. Avoid repeating dishes too often. Staples are fine, but not every other week.

Because the app doesn't do this for them, they don't use it. Other users differ (families cook more often and for more people), so all of this must be preferences with sensible defaults.

### 2.2 Verified state of the code (audit 2026-10-20)

Findings by area, from six parallel code audits with file:line evidence; the key items were re-checked by hand.

**AI agent**
- The agent's system prompt carries no dates: `ComposePrompt(p, nil, nil, prefs)` at `backend/internal/domain/agent/service.go:186`. The date-range, plates and planning-prefs sections are dead in production.
- The agent cannot see `skipped`, `favorite`, `last_cooked_at`, `cook_count`, or any macros of composed foods (`tools.go:598-615`, `tools.go:678-687`). The mode hints at `service.go:208-210` nevertheless tell it to respect `skipped` and prefer `favorite`.
- Mutations hit the live plan directly, with no draft or transaction. `MaxIterations = 20` (`loop.go:20`) is too few for a full week, and a failed run leaves a partial plan.
- No observability: model, tokens, cost, prompt version and run status are not persisted, the agent package does not log, and no link exists between plate changes and the run that made them.

**Planning model and settings**
- `skipped` is a plate row labelled "Eating out". `CopyWeek` (`preset/apply.go:380-404`) and undo drop it. Components can be added to skipped plates. Nothing prevents several plates per (date, slot).
- No concept of batch cooking, leftovers, keep time, freezing, household size or diet type exists anywhere.
- `cook_count`/`last_cooked_at` update only on manual "cooked"/"loved" feedback, use the click time rather than the meal date, and count once per plate (5 leftover plates → +5).
- No setup flow. No default slots are seeded. `plan.window_days` is unused. `PLANTRY_AUTH_PASSWORD` is documented but not implemented.

**Recipes and import**
- Recipe import: ingredient resolution needs FDC. Candidates without `existing_id` are disabled (`frontend/src/routes/import/index.tsx:474`). The fallback creates manual leaves with no nutrition, so imported recipes show ≈0 kcal. `source_url`/`image_url` are extracted but never saved. A JSON-LD `recipeYield` given as a number or array falls back to 1 portion.
- Missing nutrition silently counts as 0 (`food/nutrition.go:236`).

**Frontend**
- The shopping-list range presets are anchored on today, not on the viewed window.
- `/import` has no nav entry. Settings is missing from the mobile nav.

---

## 3. Core concepts

### 3.1 Planning window

The unit of planning is a **window** of 7 days, running from the shopping day (`plan.shopping_day`) through the day before the next shopping day and covering all slots. A Saturday shopper plans Saturday → Friday, so a Sunday batch and its Mon–Thu leftovers sit inside one window. The window replaces the dead `plan.window_days` setting.

- `plan.anchor` keeps deciding where the planner opens. Its default changes from `today` to `next_shopping_day`.
- The frontend fallback for `plan.shopping_day` (`"1"`) must match the backend default.
- There is no per-slot "shopping time" setting. The shopping list instead defaults to max(window start, today) through the window end (§7.7), so slots already past on the shopping day are not bought for. The range stays editable, and households that shop several times a week use a custom range.

### 3.2 Cell and availability

A **cell** is (date, slot). Its availability **mode** is one of:

| Mode | Meaning | Generator behaviour |
|---|---|---|
| `plan` | A plate is needed and should be planned | Fills it |
| `routine` | A plate is needed, filled from a bound preset (e.g. "Oatmeal" every weekday breakfast) | Applies the preset; it does not count as a cooking occasion |
| `none` | No plate needed (away, restaurant, invited) | Leaves it empty; no plate may exist there |

The mode resolves as follows: a per-date **override** wins, otherwise the weekday × slot **default** applies, otherwise `plan`. This replaces `plates.skipped` entirely. "No plate needed" becomes a property of the cell, not a fake plate.

### 3.3 Cook session

A **cook session** means "recipe X is cooked on date D at slot S, producing N portions". Plates consume portions from sessions:

- **fresh**: the plate is in the session's own cell
- **leftover**: a later cell, within the recipe's keep days
- **frozen**: portions set aside for the freezer, usable within `plan.freezer_keep_days`

Cells are ordered by (date, slot `sort_order`, slot id). A session can feed plates in its own cell (fresh) and in any later cell, including later slots on the same day (cook at lunch, eat at dinner). A "prep" session with no meal slot (e.g. Sunday afternoon) is deferred. If it is ever needed, `cook_slot_id` becomes nullable and is treated as before every slot of its date.

A session's **cooking occasion** is its (cook_date, cook_slot). Several sessions in the same occasion count as cooking once, e.g. curry plus rice at Sunday dinner. `plan.max_cook_occasions_per_day` caps occasions per day (default 1).

Composed components on a plate *may* reference a session. A composed component without one (legacy or quick manual entry) is treated as freshly cooked in its own cell and counts as an occasion. Leaf components (an apple, yoghurt) never need a session.

### 3.4 Portions: three levels

| Level | Field | Meaning |
|---|---|---|
| Recipe | `foods.reference_portions` | The yield as written. It is the recommended default and the base for ingredient scaling. Unchanged. |
| Session | `cook_sessions.portions` (REAL) | What is actually cooked this time. Suggested by the generator from demand; overridable by the user or the agent at any time. |
| Plate | `plates.eaters` (REAL, default from `profile.household_portions`) × component `portions` (REAL, per eater, usually 1) | Who eats and how much each eats |

Rules:

- **Session consumption** = Σ over linked components of `component.portions × plate.eaters`.
- **Ingredient amounts** for a session = recipe leaf grams × `session.portions / reference_portions`. They feed both the shopping list and the scaled cooking view.
- **Nutrition shown to the user** = per-portion macros × `component.portions`. This is one eater's intake, so a family plate no longer inflates the user's kcal.
- **Surplus** = `session.portions` − consumption. It is shown as "spare" or, if the user chooses, as freezer portions (`frozen_portions`).
- **Deficit**: consumption > portions is a checker violation (§6.4).

`profile.household_portions` (REAL, default 1.0) is the default number of eater portions per plate. The wizard offers a helper (adults × 1.0 + children × 0.5) but stores only the sum.

### 3.5 Plan state: draft and confirmed

Plates and cook sessions have exactly two states: `draft` or `confirmed`. There is no separate lock flag. A confirmed row is shown with a padlock, because its ingredients may already be bought.

- The generator only ever writes `draft` rows, tagged with the `ai_run_id` that created them.
- **Accept** confirms all drafts in a window. **Discard** deletes them. During review, **Confirm** on a single plate or session protects it before the whole window is accepted.
- **Confirmed rows are fixed context** for every generation scope, including "regenerate week". The generator never changes them. The UI reports "N confirmed plates kept".
- A confirmed row returns to draft only in two ways:
  - an explicit **Unlock** by the user, which warns that ingredients may already be bought
  - a **mid-week re-plan** that lists the affected confirmed rows and gets the user's yes
- Unlocking a session unlocks all plates linked to it, and unlocking a linked plate unlocks its session, so a session is never half-confirmed.
- **Mid-week re-plan** ("I'm away Thu–Fri, adjust") works as follows:
  - Its scope is today and later; past days are never touched.
  - Availability changes such as marking a cell `none` are manual edits and always allowed.
  - Freed session portions become freezer portions if the recipe is freezable, otherwise "N portions unassigned". The assistant may propose shifting them to later cells, but only as a draft.
  - A cooked session is never deleted automatically. A session whose cook date is still in the future may have its portions reduced.
- The shopping list and Today view use confirmed rows. The shopping list can optionally include drafts as a preview.
- Manual edits by the user are allowed on any row and do not change its state.

### 3.6 Diet class

Every food gets a **diet class**, ordered from most to least restrictive: `vegan` < `vegetarian` < `pescetarian` < `omnivore`.

- **Leaves:** classified at creation (OFF/FDC category mapping first, LLM classification if available, otherwise `unknown`). The user can override it.
- **Composed foods:** derived as the max of their children. An override is allowed, e.g. when a child is unknown.
- **Storage (as built):** only the user's override is stored (`foods.diet_class`, migration 00035; NULL = derived, nothing backfilled). Leaf and composed classes are derived on read in `domain/diet` and reported with `diet_class_source` (`auto`, `user` or `derived`). This replaces the stored leaf class and `diet_class_override` column described in §5.
- **Matching the profile:** `profile.diet_type` is one of the four classes. A food is compatible when its class ≤ the user's type. `unknown` is a checker warning, not a violation.
- **Meat rule:** the optional structured rule `profile.meat_days_per_week_max` counts days that contain any `omnivore`-class food.

Everything else ("meat once a day", "no fish on Mondays") stays free text in `profile.system_prompt`. It is given to the LLM and judged softly (§6.5).

### 3.7 Eating history (replaces cook counters)

"Last eaten" and "times eaten" are derived from **confirmed plates in the past**. An accepted plan counts as eaten by default, so the user does not have to click "cooked". Feedback becomes optional refinement: `loved`/`disliked`, plus `skipped` meaning "planned but not eaten", which excludes the plate from history. The `foods.cook_count` and `foods.last_cooked_at` columns are dropped (§11).

---

## 4. Preferences and defaults

All preferences live in the existing typed settings registry (`domain/settings/specs.go`) or the profile singleton, and are editable later in Settings.

### 4.1 Profile (`user_profile`)

| Field | Type | Default | Notes |
|---|---|---|---|
| `kcal_target`, `protein_pct`, `fat_pct`, `carbs_pct` | existing | the "maintain" preset (2200, 30/30/40) | The wizard offers cut/maintain/bulk |
| `diet_type` | enum vegan/vegetarian/pescetarian/omnivore | omnivore | New |
| `meat_days_per_week_max` | int 0–7, nullable | null | New, optional |
| `household_portions` | REAL > 0 | 1.0 | New |
| `dietary_restrictions` | existing free-text chips | [] | Kept, for allergies and intolerances |
| `system_prompt` | existing | null | Relabelled in the UI as **"Notes to your assistant"** |
| `preferences` | existing map | {} | Keeps likes/dislikes from feedback (§6.6) |

### 4.2 Settings (new keys)

| Key | Kind | Default | Meaning |
|---|---|---|---|
| `setup.completed` | bool | false | Drives the first-run redirect |
| `plan.max_cook_occasions_per_day` | int 1–3 | 1 | Hard limit |
| `plan.batch_cooking` | bool | true | Generator prefers multi-day sessions |
| `plan.default_keep_days` | int 1–7 | 3 | Used when a recipe has no `keep_days`. This is the conservative end of the common 3–4-day fridge guidance. Recipes can override it; values above 4 show a food-safety warning. A Sunday batch meant to last 4–5 days either gets `keep_days` 4 on the recipe or has its later portions planned as frozen. |
| `plan.anchor` | existing enum | **`next_shopping_day`** (was `today`) | Where the planner opens |
| `ai.price_overrides` | JSON map model id → {in, out, cache_read, cache_write} USD per 1M tokens | {} | Overrides or extends the built-in price table (§8.1) |
| `plan.freezing_enabled` | bool | false | Allows `frozen_portions` and planning from the freezer |
| `plan.freezer_keep_days` | int | 90 | |
| `plan.variety_weeks` | int 0–8 | 3 | A recipe eaten within this many weeks is avoided (§6.4) |
| `plan.explore_per_week` | int 0–3 | 0 | Recipes tagged `explore` that should be included per week |
| `discovery.daily_enabled` | bool | true (when AI is available) | Daily suggestion popup |
| `discovery.cuisines` | string list | [] | Exploration targets, e.g. thai, persian |

`plan.window_days`, `plan.week_starts_on` and `fixed_weekday` are removed. A window is the 7 days starting on `plan.shopping_day`; `plan.anchor` (`today` / `next_shopping_day`) picks the default one.

### 4.3 Availability defaults

The default for every weekday × active slot is `plan`. The wizard shows the grid prefilled this way, so the user only toggles exceptions, such as "Mon–Fri lunch → routine: Lunchbox".

### 4.4 Secret key

If `PLANTRY_SECRET_KEY` is not set, the app generates a random 32-byte key on first start and stores it in the data directory (`<data>/secret.key`, mode 0600). Saving an AI key then works out of the box. If the env var is set, it still wins.

---

## 5. Data-model changes

These describe the target shape. Every change ships as a goose migration plus `sqlc generate`, in pure SQL with no CGO, using the usual SQLite table rebuild where CHECK constraints change.

### 5.1 New tables

```
availability_defaults
  weekday     INTEGER 0..6        -- 0 = Monday (matches plan.shopping_day)
  slot_id     → time_slots ON DELETE CASCADE
  mode        TEXT plan|routine|none
  preset_id   → presets NULL       -- required iff mode = routine
  PRIMARY KEY (weekday, slot_id)

availability_overrides
  date        TEXT YYYY-MM-DD
  slot_id     → time_slots ON DELETE CASCADE
  mode, preset_id                  -- as above
  PRIMARY KEY (date, slot_id)

cook_sessions
  id
  food_id         → foods (composed only) ON DELETE RESTRICT
  cook_date       TEXT
  cook_slot_id    → time_slots
  portions        REAL > 0
  frozen_portions REAL >= 0 DEFAULT 0
  state           TEXT draft|confirmed
  ai_run_id       → ai_runs NULL
  note            TEXT NULL
  created_at, updated_at

freezer_removals                   -- user marks frozen portions gone without planning them
  id, cook_session_id → cook_sessions ON DELETE CASCADE, portions REAL, at

ai_runs, ai_run_steps, plate_events, recipe_suggestions   -- see §8 and §9
```

### 5.2 Changed tables

| Table | Change |
|---|---|
| `plates` | + `eaters REAL NOT NULL DEFAULT 1`, + `state` (existing rows become `confirmed`), + `ai_run_id NULL`, + `updated_at`. **Drop `skipped`** (§11 covers migrating existing rows). + **UNIQUE (date, slot_id)**, after merging duplicates. |
| `plate_components` | `portions` INTEGER → REAL > 0 (composed). + `cook_session_id NULL → cook_sessions ON DELETE SET NULL`. The leaf amount/unit/grams shape is unchanged. |
| `preset_plate_components` | `portions` INTEGER → REAL, so it mirrors `plate_components` |
| `foods` (composed) | + `source_url`, + `description` (imported blurb, separate from the personal `notes`), + `keep_days INT NULL`, + `freezable INT NULL` (null = unknown) |
| `foods` (all) | + `diet_class NULL`: the user override only (migration 00035, D28). The class itself is derived on read (§3.6) |
| `foods` | − `cook_count`, − `last_cooked_at` (replaced by derived history, §3.7) |
| `user_profile` | + `diet_type`, + `meat_days_per_week_max`, + `household_portions` |
| `ai_conversations` | − legacy `week_id` (dropped in 00032). The chat window is carried by `ai_runs.window_start`/`window_end` and the request's `from`/`to`; no `ai_conversations.window_start` column exists |

Every `diet_class` is computed on read, like nutrition; only the override is stored.

---

## 6. Plan generation

### 6.1 Pipeline

```
Generate(window, scope, instruction?)
  1. Resolve cells        availability → cells to fill (plan) / apply (routine) / keep empty (none)
  2. Build context        Go, deterministic (§6.2)
  3. Propose              Chooser: LLM (§6.3) or Heuristic (§6.7)
  4. Check                Go checker: hard rules (§6.4) → violations
  5. Repair               if violations and chooser = LLM: re-propose with violations listed, max 2 times
  6. Persist              draft cook_sessions + plates + components, tagged with ai_run_id;
                          unresolved violations stored on the run and flagged on affected plates
  7. Score                quality score (§6.5) stored on the run
```

This lives in a new domain package `backend/internal/domain/planning` with the ports `Chooser` (LLM / heuristic) and `Checker`, and an application service that is shared by HTTP handlers and agent tools.

**Scopes:** whole window, one day, one plate, one component ("different side"), and "empty cells only" (scope kind `empty_cells`; the mid-week re-plan uses it with a `from`/`to` range). Everything outside the scope, plus everything confirmed, is passed to the chooser as fixed context.

### 6.2 Context (what the chooser sees)

Assembled in one pass, compact and token-budgeted:

- **Dates:** today, the window dates with weekday names, the shopping day.
- **Cells:** for each cell, date, slot name and mode. For `plan` cells, whether it is empty, fixed (confirmed or out of scope), or in scope.
- **Fixed plates and sessions,** with their portions and remaining capacity. This includes still-valid leftovers and freezer portions from before the window.
- **Candidate recipes** (composed foods compatible with `diet_type`), one compact row each:
  - id, name, role, diet class
  - kcal/P/F/C per portion, plus a nutrition-completeness flag
  - prep + cook minutes
  - reference portions, keep days (or the default), freezable
  - last eaten, times eaten in the last 8 weeks
  - favorite, `explore` tag, liked/disliked tag score
- **History:** the plates of the last 4 weeks (date, slot, recipe names).
- **Preferences:**
  - profile targets
  - diet type, meat-day limit
  - household portions
  - every `plan.*` setting
  - the user's notes to the assistant
  - restrictions
- **The user's one-off instruction** for this generation, if any (e.g. "guests on Friday, 4 people").

### 6.3 LLM chooser

**One call.** The output is forced through a single tool `submit_plan` (provider `tool_choice` forced) with a JSON schema:

```
sessions: [{ key, food_id, cook_date, cook_slot_id, portions, frozen_portions, reason }]
plates:   [{ date, slot_id, eaters,
             components: [{ food_id, portions, session_key | null }] , reason }]
notes:    string   -- short summary shown to the user
```

`llm.Request` gains `ToolChoice` (both adapters). `Temperature` and `MaxTokens` are set explicitly for generation (MaxTokens well above the current 4096 default). The run is not tied to the HTTP request context: generation continues if the browser disconnects, and the UI polls or re-attaches by `ai_run_id`.

### 6.4 Checker: hard rules

| # | Rule |
|---|---|
| H1 | Every `plan` cell in scope has exactly one plate. No plate exists in a `none` cell. `routine` cells contain the preset's plate. |
| H2 | Cooking occasions per day ≤ `plan.max_cook_occasions_per_day`. Routine plates and leaf-only plates don't count. |
| H3 | For each session, consumption ≤ `portions − frozen_portions`, and frozen consumption ≤ `frozen_portions`. |
| H4 | A leftover or frozen plate's cell is strictly after the session's cook cell in (date, slot sort_order, slot id) order, and no later than cook_date + keep_days (frozen: + `freezer_keep_days`). Cook at lunch and eat at dinner is allowed; cook at dinner and eat that day's lunch is not. If a later slot reorder breaks H4 for existing sessions, the checker reports it and does not fix it silently. |
| H5 | Every food's diet class ≤ `diet_type`. `meat_days_per_week_max` is respected. |
| H6 | Confirmed rows and out-of-scope rows are untouched. |
| H7 | All IDs exist, sessions only reference composed foods, and every plate has at least one component. |
| H8 | **Variety:** no session of a recipe eaten within `plan.variety_weeks`. This is relaxable: if the library cannot satisfy H8, the checker allows the least-recently-eaten recipes and reports the relaxation as a warning instead of a violation. |

Violations that remain after repair don't abort the generation. They are persisted, shown on the affected plates ("needs attention"), and counted in the run's score.

### 6.5 Quality score (deterministic, stored per run)

| Component | Measure |
|---|---|
| coverage | filled plan cells / plan cells |
| hard_violations | count by rule |
| macro_deviation | mean absolute % deviation of daily kcal and P/F/C from targets (daily *and* weekly average) |
| cook_occasions | per day; days with ≥1 leftover/prepared meal ("always one prepared meal" signal) |
| variety | distinct recipes; repeats within `variety_weeks`; explore quota met |
| preference_fit | Σ favorite/liked − disliked tag hits |
| waste | surplus portions not frozen and not consumed |
| unknown_data | share of foods with unknown diet class or incomplete nutrition |

Free-text notes ("meat once a day") cannot be checked deterministically. An optional LLM-judge pass was specified here (0–5 with reasons, stored separately, off by default). **It was not built** (D20): `ai_runs.judge_json` stays unused and no judge is planned; free-text notes reach the LLM chooser as prompt context only.

### 6.6 Learning from the user

Signals, all stored with timestamps (§8):

- **Edits after generation:** the plate_events with actor = user on rows that a run created, classified as swap side / replace plate / change portions / delete.
- **Accept/discard** per run, and **regenerate requests** with the scope and instruction text.
- **Feedback:** loved/disliked. These keep updating the `likes`/`dislikes` tag lists, with two fixes: generic tags (role names, `explore`) are excluded, and a tag moved to the opposite list is removed from the original one.
- **Suggestion accept/reject** (§9).

The context builder turns these into per-recipe "liked/disliked" scores and a short "recent corrections" summary for the chooser (e.g. "user replaced fish twice on weekdays").

### 6.7 Heuristic chooser (no AI)

This is a deterministic greedy planner, so the core flow works without an AI key and gives the LLM a measurable baseline:

1. Place sessions on the earliest uncovered `plan` dinner cell.
2. Pick the main by score: least recently eaten → favorite → liked → fits remaining macro budget. It must satisfy the diet rules.
3. Size portions to cover the following cells up to keep days and `max_cook_occasions_per_day`.
4. Add a side if the main's role is `main`. Pick sides by the same score, and cook them fresh at the same occasion.
5. Fill lunch cells from the most recent session with remaining portions.

It aims for simple and correct, not optimal.

---

## 7. User-facing flows

### 7.1 First run: setup wizard (`/setup`)

`/` redirects to `/setup` while `setup.completed` is false. Every step is prefilled, so "Next, Next, Finish" yields a working configuration.

| Step | Content | Defaults |
|---|---|---|
| 1 Welcome | Language | Browser locale (en/de) |
| 2 You & household | Diet type, household helper (adults/children → `household_portions`), allergies | omnivore, 1.0 |
| 3 Nutrition goal | Cut/maintain/bulk cards, or custom | maintain |
| 4 Meals & week | Slots (Breakfast, Lunch, Dinner pre-created, editable). Weekday × slot availability grid (§4.3). | all `plan` |
| 5 Cooking style | Cooking occasions per day, batch cooking on/off, default keep days, freezing on/off | 1, on, 3, off |
| 6 Variety & shopping | Variety weeks, shopping day, new-recipe quota | 3, Saturday, 0 |
| 7 Assistant (optional) | AI provider + key + model; notes to the assistant; daily suggestion toggle | skip allowed |
| Finish | Sets `setup.completed` → navigates to the recipe onboarding | — |

The wizard can be re-run from Settings → System.

### 7.2 Recipe onboarding (Recipes page, guided state)

The guided state is shown while fewer than **10** composed foods exist:

- Progress "N / 10 recipes, the weekly planner unlocks at 10", with a one-line reason: the planner needs enough variety.
- Three option cards, each with a 1–2 sentence explanation:
  1. **Import from a URL**: paste a recipe link. This is the fastest way.
  2. **Discover recipes** (shown whenever an AI provider is configured, §9): get suggestions to add.
  3. **Create manually**, with a pointer to variants for personal versions.
- "Generate week" is visible everywhere but disabled below 10 recipes, with a link back here.
- Import gets a permanent nav entry (desktop and mobile). Settings is added to the mobile nav.

### 7.3 Saturday flow (`/plan`)

The planner shows a "Plan next week" banner from the day before the shopping day until the window is planned. The Today page shows the same banner.

1. **Availability:** the next window's grid, resolved from defaults. Tap cells to cycle plan → none → routine (preset picker). There are bulk actions per day ("away all day"). Changes become overrides for those dates. A checkbox "Save as my new default" writes them to the defaults instead.
2. **Anything special?** An optional free-text instruction ("guests Friday, 4 people", "something light this week"), an eaters override per day, and an "include a new recipe" toggle (prefilled from `plan.explore_per_week`). Until Phase 5 (discovery) a "new" recipe means one that was **never eaten** (no `last_eaten`), not one tagged `explore`.
3. **Generate:** a progress view (resolving cells → asking the assistant → checking → saving). If AI is unavailable, the heuristic runs instead.
4. **Review:** the existing planner grid in draft mode.
   - Draft plates are visually distinct, and leftover plates show "from Sun · Chili (day 2 of 4)".
   - Sessions show their cook day, portions (stepper), and surplus or freezer portions.
   - Per plate: **confirm** (padlock), **regenerate**, **different side**, swap, remove, change eaters.
   - Per day: regenerate day.
   - Violations and warnings are shown inline.
   - The chat panel (now non-modal, as a side panel on desktop) works on this draft (§7.6).
   - The run's `notes` summary is shown at the top.
5. **Accept:** confirms the drafts and opens the shopping list for exactly this window. **Discard** removes all drafts of the window.

### 7.4 Today (`/today`, the new landing page)

- **Cook today:** each session with `cook_date` = today, showing the recipe, the portions being cooked (with the reason, e.g. "covers Mon–Thu"), **scaled ingredient amounts**, instructions, and a freezing note if `frozen_portions > 0`.
- **Eat today:** today's plates, labelled fresh or leftover ("Chili, cooked Sunday").
- **Reminders:** "take X out of the freezer tonight" when tomorrow eats a frozen portion; "use by today" for the last leftover day.
- **One-tap feedback** on yesterday's and today's plates (loved / disliked / didn't eat it).
- The "Plan next week" banner when due.

The old read-only `/day/$date` route is replaced by `/today?date=…`.

### 7.5 Recipe view and cooking view

- A recipe shows `description` (imported), **My notes** (personal), a source link, its diet class (with override), keep days, freezable, and a nutrition-completeness warning.
- A **portion stepper** on the recipe page scales the ingredient list. It defaults to `reference_portions`; when opened from a session, it defaults to the session's portions.
- Scaling rounds as follows:
  - Mass and volume are scaled exactly and displayed rounded (g to 5 g above 100 g).
  - Countable units (Stück, Dose, Zehe, Bund, egg) are rounded **up** to whole units, with a hint "buying 1 Dose, using ⅔".

### 7.6 Chat on the draft

The chat remains for adjustments ("no fish this week", "swap Tuesday's side for something with potatoes", "make the chili 6 portions, I'll freeze 2").

- The system prompt now includes today's date, the window, and a compact plan summary, reusing §6.2.
- **New tools:**
  - `regenerate` (scope + instruction → calls the generation service)
  - `set_session_portions`, `set_plate_eaters`
  - `confirm` / `unlock` (unlock lists the affected confirmed rows and needs the user's yes in chat)
  - `set_availability`
  - `get_plan` (cells + sessions + plates in one call)
- **Changed tools:**
  - `list_foods` / `get_food` return macros per portion, diet class, keep days, last eaten, favorite.
  - `get_plates_range` returns state, eaters, and session links.
  - Agent tools take REAL portions (the `QuantityFromLegacyPortions` shim is removed).
- **Removed tools:** `copy_week` (the generator replaces it); `record_preference` is restricted to keys other than likes/dislikes, enforced in code.
- Mutations from chat on a draft window keep rows in the `draft` state, with the chat's `ai_run_id`.

### 7.7 Shopping list

- The source is confirmed sessions (scaled by `portions / reference_portions`), leaf components (× eaters), and routine plates in the window. A toggle includes drafts.
- The range defaults to max(window start, today) through the window end. The presets "this window" and "next window" are anchored on the **viewed window**, and a custom date range is available.
- Count units are rounded up as in §7.5. The list groups by ingredient and shows which recipes use it on expand.
- "Purchased" check state stays local (localStorage), as today.

---

## 8. Measurement and evaluation

### 8.1 Run log

```
ai_runs
  id, kind (generate | regenerate | chat | discovery | classify | import_fallback)
  started_at, finished_at, status (ok | error | iterations_exceeded | cancelled | partial)
  provider, model, prompt_version, prompt_hash
  window_start, window_end, scope_json, instruction
  chooser (llm | heuristic)
  iterations, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens
  repair_attempts, violations_json, score_json, judge_json NULL
  outcome (accepted | discarded | partially_accepted | open), outcome_at
  error_key NULL

ai_prompts         hash PK, text            -- system prompts stored once per distinct text

ai_run_steps
  id, run_id, seq, type (llm_call | tool_call)
  name, args_json, result_json (truncated > 8 KB), is_error, duration_ms
  input_tokens, output_tokens                -- for llm_call
```

- Recording happens through a decorator around `llm.Client` (the metered client) plus a hook at `ToolSet.Execute`. No adapter changes are needed.
- `Usage` is summed across iterations; today only the last one is reported.
- **Cost is computed on read, never stored.** Exact token counts are the source of truth.
  - `llm.Cost(model, usage) *float64` uses a built-in table in `domain/llm`: USD per 1M tokens for input, output, cache read and cache write, keyed by model-id prefix (longest prefix wins), with an `as_of` date.
  - `ai.price_overrides` (§4.2) overrides or adds entries per exact model id.
  - Missing cache prices default to fixed fractions of the input price: cache read 0.1× (Anthropic) or 0.5× (OpenAI), cache write 1.25× (Anthropic) or 0 (OpenAI).
  - OpenAI `prompt_tokens` includes cached tokens, so cached tokens are subtracted before pricing.
  - An unknown model's cost is shown as "cost unknown" with tokens still visible, and it is excluded from totals with a note ("excludes N runs").
  - Changing a price re-prices history. Tiers and long-context surcharges are ignored, and costs are labelled estimates.
  - Vendor billing APIs are not used, because they need admin keys and report daily rather than per run.
- **Web search fees** (§9.4) are counted from the server tool's usage, at a per-search price in the same table.
- The agent loop and the generation service log `ai.run.start` and `ai.run.end` through slog, with run_id, request_id, model, tokens and status.
- Each prompt template carries a `PromptVersion` constant, which is bumped on every change.

### 8.2 Plate audit trail

```
plate_events
  id, plate_id (no FK, survives deletes), cook_session_id NULL, date, slot_id
  action (create | update | delete | confirm | unlock | accept | discard)
  actor (user | generator | agent), ai_run_id NULL
  before_json, after_json, at
```

- Written by the plate and cook-session services. The actor and run id travel in `context.Context`, set by HTTP handlers (user), the generation service (generator) and agent tools (agent).
- **Derived signal:** user edits on rows created by a run before it is accepted. The **correction rate** = user-modified plates / generated plates, per run.

### 8.3 AI insights page (Settings → Assistant → Insights)

- **Runs list:** kind, date, model, prompt version, tokens, cost, status, score, outcome. Click through to the steps and the stored prompt.
- **Trends** over time and grouped by prompt version and model:
  - quality score components
  - correction rate, acceptance rate
  - cost per accepted plan
  - violation counts by rule
- **"Why this?":** each generated plate links to its run and shows the chooser's `reason`.

### 8.4 Offline eval suite

- **Location:** `backend/internal/eval` with a `cmd/plantry-eval` entry point.
- **Scenarios** are JSON fixtures: a seed library (≈30 recipes with full nutrition), profile, settings, availability, history, instruction, and per-scenario expectations. Initial set:
  - the single batch-cooker (this user)
  - a family of 4 cooking daily
  - a vegetarian with a 3-week variety window
  - a small library that forces H8 relaxation
  - a week with many `none` cells
  - a re-plan of one day with confirmed neighbours
  - a mid-week "away Thu–Fri" re-plan on an accepted window
- **Runner:** an in-memory SQLite plus the real generation service and checker. It runs each scenario N times (default 3) per configured chooser/model and writes a JSONL report keyed by (git sha, prompt_version, model) with score components, violations, tokens, cost and latency. A summary diff against the previous report is printed.
- **CI:** the heuristic chooser plus a scripted fake client. These are deterministic and assert no hard violations on the fixtures. The fake client also records the incoming `llm.Request`, so tests can assert on prompt content (this would have caught the nil-dates bug).
- **Real models:** gated behind `PLANTRY_EVAL_LIVE=1` plus keys, and run manually or nightly.

---

## 9. Recipe discovery

### 9.1 Modes

| Mode | Input | Goal |
|---|---|---|
| Daily suggestion | none | One recipe per day that fits the profile and adds variety. It rotates through `discovery.cuisines` if set. |
| Explore a cuisine | cuisine (e.g. Persian) + optional constraints | 3 suggestions from that cuisine |
| Use what I know | optional focus ingredient | Recipes whose ingredients are mostly **already in the user's database** (no pantry, nothing to maintain) |

### 9.2 Pipeline

1. **Research:** an LLM run finds candidate recipe **pages**. It uses web search where the provider supports it, and otherwise proposes URLs from the model's own knowledge (§9.4). It is constrained by:
   - diet type and allergies
   - the notes to the assistant
   - disliked tags
   - recently rejected suggestions
   - for "use what I know": the catalogue's ingredient names

   It prefers pages with JSON-LD and returns URLs with a one-line rationale each.
2. **Verify and import:** the app fetches every candidate URL itself (`adapters/httpfetch`). URLs that fail to load or parse are discarded, which filters out dead or invented links in both research modes. The rest go through the existing import pipeline (JSON-LD, LLM fallback), then the resolver against the local catalogue. Nutrition is computed from the resolved ingredients, never trusted from the LLM.
3. **Annotate:** each ingredient is matched against the catalogue.
   - A leaf food that appears in none of the user's recipes, or doesn't exist in the catalogue yet, is marked **new**.
   - "Use what I know" ranks candidates by the share of known ingredients.
4. **Store** a row in `recipe_suggestions`: id, mode, query, source_url, draft_json, image_path (downloaded from the page's `og:image`/JSON-LD image), status (pending/accepted/rejected), ai_run_id, created_at.

### 9.3 Presentation

- **Daily popup:** on the first app open of the day, if `discovery.daily_enabled` is set and an AI provider is configured. It is generated lazily at that moment, not by a background job.
- **The card shows:**
  - a large dish photo from the source page (placeholder if none)
  - name, cuisine, time, macros per portion, diet class
  - the ingredient list with **"new" ingredients highlighted** and a legend
  - a short "why this" line and the source link
- **Actions:**
  - **Add to my recipes:** the normal import review screen, prefilled. The recipe is tagged `explore` and its `source_url` is stored.
  - **Not for me:** rejected. The reason is optional. Rejections feed later research.
  - **Show another.**
- **On-demand:** a "Discover" button on the Recipes page opens the mode picker. It is also the third option card in onboarding (§7.2).

### 9.4 Provider capability

There are two research modes. Both feed the same verification step (§9.2).

| Mode | When | How |
|---|---|---|
| **Search-grounded** | Anthropic provider and the model reports web search support | The server-side `web_search` tool (`max_uses` ≈ 5, `allowed_callers: ["direct"]`), using the newest tool version the model supports and `web_search_20250305` as the safe default. Costs about $10 per 1,000 searches plus tokens. |
| **Knowledge-based** | Every other configuration (OpenAI, unsupported model, search disabled for the organisation) | The LLM returns candidate URLs as JSON from its own knowledge. Results are not fresh, and some URLs will fail verification. |

- **Capability detection (Anthropic):**
  - On settings save or first use, read `GET /v1/models/{model}` → `capabilities.server_tools.web_search.supported`. A missing field or a failed call counts as unsupported.
  - Organisation-level disablement is only visible at runtime (HTTP 400). In that case fall back to knowledge-based mode and cache "unavailable" for 24 h.
- **OpenAI:** web search requires the Responses API, while the adapter uses Chat Completions. It is deferred; OpenAI users get knowledge-based mode.
- **UI:** the card shows a "search-grounded" badge when that mode produced it. Discovery is hidden only when no AI provider is configured. The source link on every card also meets Anthropic's citation-display requirement.
- **Adapter changes (Phase 5):**
  - `llm.Request.WebSearch bool` and a capability query on the client.
  - Anthropic `toolBody` gains `type` and `max_uses`/`allowed_callers`.
  - `stream.go` tolerates `server_tool_use`, `web_search_tool_result` and `citations_delta`, and collects result URLs.
  - Handle `pause_turn` by resending the turn, and replay `encrypted_content` unchanged.
  - Refresh the stale hand-maintained model list in `anthropic/models.go`.
- **Every discovery is an `ai_runs` row** (`kind = discovery`), so acceptance rate and cost per accepted recipe appear on the insights page.

---

## 10. Recipe and import fixes

These are required before onboarding, because users will import their first 10 recipes.

1. **Resolver:**
   - Search the local catalogue by the query's own name first (case/umlaut-insensitive; later FTS), then FDC, then OFF.
   - Match local foods against the original German query, not the translated FDC description.
   - A candidate without `existing_id` is selectable and creates a leaf **with its nutrition**.
   - The manual-create fallback reuses an existing same-name leaf instead of failing on the unique index, and surfaces errors.
2. Persist `source_url` and download the image on import. Put the imported description into `description`, not `notes`.
3. Parse `recipeYield` given as a number or array. Read the JSON-LD `nutrition` block for display/comparison only, not as the source of truth.
4. **Count units:** create per-food portions when resolving ("1 Zwiebel" → the food's portion, falling back to the unit default with a confidence badge). Keep the original unit for display. Fix glued numbers ("100g Mehl") and the "(n)" plural suffix.
5. **Nutrition completeness:** composed nutrition returns `complete: false` plus the missing leaf names when any child lacks data. The UI warns instead of silently counting 0.
6. Classify diet class (§3.6) and suggest keep days/freezable on import. The user confirms on the review screen.
7. Fix the FDC refetch that passes the numeric id as a text query (`handlers/lookup.go:184-185`).

---

## 11. What is removed or replaced

| Removed | Replaced by | Migration |
|---|---|---|
| `plates.skipped`, `POST /api/plates/{id}/skip`, "Eating out" UI, `planner-skip.ts` | Availability overrides (`none`) | Each skipped plate → an override `none` for its (date, slot), then the plate is deleted |
| Duplicate plates per (date, slot) | UNIQUE constraint | Merge components into the oldest plate, then add the constraint |
| `foods.cook_count`, `foods.last_cooked_at`, `MarkCooked`, the "forgotten / most cooked" insights backed by them | Derived history from confirmed past plates | Insights queries rewritten over plates |
| AI "fill session", `RevertBanner`, chat mode select (fill_empty/replace_all), mode hints in `service.go` | Drafts + accept/discard + `regenerate` | — |
| `GeneratePlanButton` prefilling a chat prompt | Navigates to `/plan` (Saturday flow) | — |
| `copy_week` agent tool | Generator | The HTTP copy-week endpoint and dialog stay for manual use, now preserving availability |
| `plan.window_days` setting | Window = 7 days from the shopping day | Delete the setting row |
| `LegacyPortionsValue`, `QuantityFromLegacyPortions`, `TODO(plate-workflow-rework)` | REAL portions end to end | — |
| `/day/$date` route | `/today?date=` | Redirect |
| `ai_conversations.week_id` | `window_start` | Drop the column |
| `PLANTRY_AUTH_PASSWORD` in README/compose | — (single-user LAN, no auth) | Docs only |

---

## 12. API surface (new / changed)

```
GET    /api/setup                         → { completed, steps_done }
POST   /api/setup/complete

GET    /api/availability/defaults         PUT same (full grid)
GET    /api/availability?from&to          → resolved cells (mode, source: default|override, preset)
PUT    /api/availability/{date}/{slotId}  { mode, preset_id? }
DELETE /api/availability/{date}/{slotId}  (revert to default)

GET    /api/cook-sessions?from&to         POST, PUT /{id}, DELETE /{id}
PUT    /api/cook-sessions/{id}/portions   { portions, frozen_portions }
POST   /api/cook-sessions/{id}/freezer-removal { portions }

POST   /api/plan/generate                 { from, to, scope?, instruction?, chooser? } → { run_id }
GET    /api/plan/runs/{id}                → status, progress, notes, violations, score
GET    /api/plan/runs/{id}/events         (not built: the client polls GET /api/plan/runs/{id} every 500 ms; the
                                           run is re-attachable through `running_run_id` on GET /api/plan, D9)
POST   /api/plan/regenerate               { scope: { plate_id | date | component_id }, instruction? } → { run_id }
POST   /api/plan/accept                   { from, to }
POST   /api/plan/discard                  { from, to }
POST   /api/plates/{id}/confirm           POST /api/plates/{id}/unlock
POST   /api/cook-sessions/{id}/confirm    POST /api/cook-sessions/{id}/unlock   (cascade to linked plates)
GET    /api/plan/replan/preview           ?from&to[&none=date:slot…] → { none_cells, confirmed_affected, spare }
POST   /api/plan/replan                   { from, to, none_cells, unlock_confirmed?, spare: freeze|keep, regenerate,
                                            chooser?, instruction? } → { removed, run_id }
                                          (as built: replan writes the No-plate overrides itself, so the body carries
                                           `none_cells`; with confirmed plates affected it answers 409
                                           error.plan.confirmed_affected and the client re-sends with
                                           unlock_confirmed: true after the user agrees; a `from` before today is
                                           400 error.plan.replan_past. A re-plan has no Undo: its removal and the new
                                           run are one step the user can review and discard instead.)

GET    /api/today?date=                   → sessions to cook (scaled), plates, reminders

GET    /api/foods/{id}/scaled?portions=   → scaled ingredient list with rounding hints

GET    /api/ai/runs?kind&from&to          GET /api/ai/runs/{id} (with steps)   GET /api/ai/insights

POST   /api/discovery                     { mode, cuisine?, focus? } → { run_id }
GET    /api/discovery/daily               → today's suggestion (creates lazily)
GET    /api/discovery/suggestions?status=
POST   /api/discovery/suggestions/{id}/accept | /reject { reason? }
```

Changed endpoints:
- `GET /api/plates*` returns `eaters`, `state`, and `cook_session_id` per component.
- `GET /api/ai/runs*` returns the computed `cost_estimate_usd` (null if the model has no price).
- `GET /api/shopping-list` gains `include_drafts` and is computed from sessions.
- `GET /api/plan` reports per-eater intake and completeness per day (`days`); `GET /api/nutrition` was removed.
- Profile and settings endpoints carry the new fields.

---

## 13. Non-functional requirements

- **Without AI:** the setup wizard, onboarding, availability, cook sessions, the heuristic generator, review, accept, shopping and Today all work without an AI key. AI adds the LLM chooser, chat, classification fallback and discovery.
- **Performance:**
  - Context building and checking for a 7-day window with 300 recipes takes < 200 ms on a Raspberry Pi 4.
  - LLM generation shows progress within 1 s.
  - Accept and discard are single transactions.
- **Correctness:**
  - Generation persists atomically per attempt: a run never leaves a partial draft.
  - Accept and discard are atomic per window.
- **Cost:**
  - One generation targets ≤ 1 LLM call plus ≤ 2 repairs.
  - Discovery targets ≤ 1 research call per suggestion batch.
  - Costs are visible on the insights page.
- **Storage:**
  - `ai_run_steps.result_json` is truncated above 8 KB.
  - Prompts are deduplicated by hash.
  - Runs older than 180 days are pruned (configurable), keeping the `ai_runs` summaries.
- **Platform:** pure Go, no CGO, ARM cross-compile; SQLite only.
- **Tests:**
  - Every checker rule and every score component has table-driven unit tests.
  - Every new flow has a Playwright e2e test (setup wizard, onboarding gate, Saturday flow with heuristic, confirm/regenerate, mid-week re-plan, Today, discovery with a fake client).
- **i18n:** all new strings in en and de.

---

## 14. Delivery phases

Each phase ships independently and leaves the app usable.

| Phase | Content | Done when |
|---|---|---|
| **0 Foundations** | Fix the agent prompt dates (`service.go:186`). Expose skipped/favorite/macros to the agent as an interim. Fix import resolution and 0-kcal leaves, persist source_url and image, add `description`. Nutrition completeness. Auto-generated secret key. Shopping range anchored on the viewed window. **Run log (§8.1) and metered client.** Fake client records requests. Docs cleanup. | The existing chat produces dated plans. Importing a chefkoch recipe yields correct macros without manual leaf fixing. Every AI call is visible in `ai_runs`. |
| **1 Preferences & availability** | Profile/settings fields (§4). Availability tables and grid UI. Migrate skipped plates → overrides. UNIQUE (date, slot). Setup wizard. Recipe onboarding gate. Nav fixes. | A fresh install goes wizard → onboarding → planner. "No plate needed" is a cell state. |
| **2 Portions, sessions, Today** | REAL portions, `plates.eaters`, cook sessions (manual: "cook for N meals" on a plate creates a session plus leftover plates in the next `plan` cells), scaled recipe view, shopping list from sessions, derived eating history, `/today` as the landing page, plate audit trail (§8.2). | A user can plan a Sunday batch manually and see it on Today, with correct shopping amounts and per-eater kcal. |
| **3 Generator** | `domain/planning`: context builder, heuristic chooser, checker, score, drafts and confirm/unlock, accept and discard, mid-week re-plan, LLM chooser with forced `submit_plan`, repair loop, regenerate scopes, Saturday flow `/plan`, eval suite plus CI scenarios, insights page. | Without an AI key, the heuristic produces a violation-free week on the fixtures. With a key, a week is generated, reviewed, partially regenerated and accepted in under 5 minutes of user time. |
| **4 Chat on drafts** | Prompt with window + plan summary, new and changed tools (§7.6), non-modal chat panel, legacy shims removed. | "Make the chili 6 portions and freeze 2" and "different side on Tuesday" work and are logged as runs. |
| **5 Discovery** | Anthropic web search capability plus knowledge-based fallback (§9.4), discovery pipeline, suggestions table, daily popup, explore and use-what-I-know modes, onboarding card, `explore` quota in the generator. | A daily suggestion with photo and highlighted new ingredients appears. Accepting it adds a recipe with correct macros and its source. |

---

## 15. Out of scope

- Pantry or inventory tracking. Ingredients are "known" if they are in the database, nothing more. (Freezer portions are derived from sessions and are not a pantry.)
- Pack-size optimisation of the shopping list.
- Multiple users or accounts. The household is a portion count, not people with profiles.
- Per-member nutrition targets.
- Consolidating the calendar month/agenda views with the planner (separate cleanup).
- AI-generated images.
- Background schedulers. All AI work is triggered by the user or by opening the app.

---

## 16. Resolved questions (researched 2026-10-20, accepted by default)

1. **Web search:**
   - Search-grounded discovery uses Anthropic's server-side web search when the model reports support. Every other configuration uses knowledge-based URL proposals.
   - The app verifies every URL itself in both modes.
   - OpenAI search (Responses API) is deferred. Discovery is hidden only without any AI provider. See §9.4.
2. **Window boundaries:**
   - The window runs from the shopping day through the day before the next one, covering all slots.
   - The shopping list defaults to max(window start, today) → window end. No per-slot shopping-time setting.
   - `plan.anchor` defaults to `next_shopping_day`. See §3.1.
3. **Cost:**
   - Store tokens only and compute cost on read.
   - Prices come from a built-in prefix-matched table with an `as_of` date, plus user overrides in `ai.price_overrides`. Unknown model → "cost unknown". See §8.1.
4. **Same-day leftovers:**
   - Allowed when the eating slot sorts after the cooking slot (H4 rewritten).
   - A prep session without a meal slot is deferred.
   - Default keep days stay 3, with recipe overrides and a warning above 4. See §3.3, §4.2, §6.4.
5. **Accept and lock:**
   - Two states only, draft and confirmed (shown with a padlock), with no separate lock flag.
   - Confirmed rows are never changed by the generator.
   - Unlock and mid-week re-plan need explicit user consent. See §3.5.

Remaining unverified: food-safety keep times are based on secondary sources (USDA 3–4 days as reported in the press; no BfR primary source found), and the OpenAI Responses SSE format is unchecked because it is deferred.

---

## 17. Acceptance criteria (end state)

1. A fresh install with defaults reaches a planned, accepted week without the user typing any nutrient value or preference beyond the 10 recipes.
2. Saturday flow for this user's profile (1 occasion/day, batch on, variety 3 weeks, omnivore with "meat 2–3 days" note):
   - one Sunday session covers ≥ 3 later dinners within keep days
   - no day has more than one cooking occasion
   - no recipe repeats within 3 weeks when the library allows it
   - `none` cells stay empty
3. Confirming two plates and regenerating the week leaves those two untouched. A mid-week "away Thu–Fri" re-plan lists the affected confirmed plates and changes them only after the user agrees.
4. Changing a session from 4 to 6 portions with 2 frozen updates the shopping amounts, shows the freezer portions on Today, and passes the checker.
5. Setting `household_portions` to 3.5 triples-and-a-half the shopping amounts but leaves the user's daily kcal unchanged.
6. Every generation, regenerate, chat and discovery appears in the insights page with model, prompt version, tokens, cost, status, score, and outcome. The user's edits after generation are counted.
7. `plantry-eval` produces a comparable report for two prompt versions on the fixture scenarios.
8. Without an AI key, every flow except chat and discovery works (heuristic generation).

---

## 18. Glossary

| Term | Meaning |
|---|---|
| Window | The 7-day planning period, starting on the shopping day |
| Cell | (date, slot) |
| Availability mode | plan / routine / none for a cell |
| Cook session | One cooking of one recipe producing N portions |
| Cooking occasion | (cook_date, cook_slot): all sessions cooked together count once |
| Leftover plate | A plate eating from a session cooked in an earlier cell |
| Eaters | Eater-portions at a plate (household default) |
| Draft / confirmed | Plan state (§3.5); confirmed rows show a padlock and are fixed for generation |
| Chooser | The component that proposes a plan: LLM or heuristic |
| Checker | Deterministic hard-rule validation |
| Run | One logged AI or generator execution (`ai_runs`) |
| Correction rate | User-modified generated plates / generated plates |
| Diet class | vegan < vegetarian < pescetarian < omnivore |
| New ingredient | A leaf food not used in any of the user's recipes yet |
