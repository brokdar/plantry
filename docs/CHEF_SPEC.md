# Spec: The Chef

> **Status:** Shipped in releases R1–R5 (2026-11). This text is the spec as written before the build; where the build differs: reserved cells were not built (the trials showed no need for them), and the Chef is on by default for new installs only (ON-3); an upgrade keeps its setting.
>
> **Draft 2, 2026-11-24.**
> **Scope:** what the feature does, the data it needs, its API surface, its limits and how it is accepted. How the assistant organises its work internally (skills, subagents, prompts) is left to the implementation, by the owner's decision.
> **Language:** "the Chef" is this document's name for the capability, never a UI label; the user sees "the assistant" (UX spec §7).
> Starting values for weights, thresholds and limits are marked *(tunable)*; the implementation tunes them against the eval sets.

---

## 1. Summary

| Area | Today | Target |
|---|---|---|
| Plate quality | Numbers only; name-based misfit rules (`planning/pairing.go`) | Recipe attributes; rule-based plate fit ranks and filters components in every planning path; a tip offers one checked improvement when it matters |
| New recipes | Discover on request or once a day; the user imports each one | The assistant finds, cross-checks, adapts and checks recipes when a week needs them; they enter the library on trial without a mandatory review |
| Own recipes | Variants start empty | Filled variants with a list of changes; in-place edits with a diff and Undo |
| Taste | Flat liked/disliked tag lists; chat memory lost on restart | A weighted, fading taste model at five levels; a visible Taste page; confirmed chat statements |
| Variety | `plan.explore_per_week`, `plan.variety_weeks`, `discovery.cuisines` | Staples, a week theme, one "New dishes per week" setting, monotony caps, varied sides across a batch |
| Constraints | Diet type, restriction chips, free-text notes | Kitchen: active time weekday and weekend, equipment, skill, basics at home, heat and flavour leanings |
| Help | Assistant only on `/plan` | Context entry points in cook mode, shop and the recipe page; substitutions; an import check; an opt-in library review |
| Ingredient truth | Import review allows "Create new" without nutrition and "Skip" | Every ingredient line is matched to a food with nutrition; an unmatched line is an error |
| Cost | Run log and Insights | A monthly cap for the assistant's own work; no autonomous search outside planning |

---

## 2. Why

### 2.1 User problem

The owner: "It's currently only a number game and missing the chef's perspective. Is the plate any good, would a chef recommend this, are the ingredients working together, is something missing to get it to the next level." The user wants good recipes and plates without having to ask, configure or approve much. Tastes move ("this week Japanese, next week German") around a core of staples.

### 2.2 Verified state of the code (2026-11-24)

- The assistant lives only on `/plan`; its tools edit plans (`get_plan`, `find_foods`, `find_alternatives`, `apply_changes`, `regenerate`, `undo_last_change`, `confirm`, `unlock`, preset tools, `record_preference`). It has no recipe write access; undo covers plan changes only (`/ai/changes/{id}/undo`).
- `planning/pairing.go` holds name-based misfit rules and states that "the library has no cuisine or dish-type data". Side choice has no taste key.
- `profile.ApplyFeedback` keeps flat `likes`/`dislikes` tag lists. Feedback statuses are `loved`, `disliked`, `not_eaten`; notes are stored and never read. Today asks for feedback on every recent eaten plate (`planview/today.go`).
- Plate components reference foods with `ON DELETE RESTRICT`: a food ever planned cannot be deleted. Plate nutrition is computed live from the recipe; plates hold no nutrition snapshot.
- Discovery asks the model for candidate pages, fetches each page itself, parses JSON-LD, drops non-recipes and duplicates, and copies the page's `og:image`. URL imports also download the page image (`importer/service.go`, `ImportWizard.tsx`).
- `adapters/jsonld` extracts the author but ignores `aggregateRating` and `datePublished`. `adapters/httpfetch` does not read robots.txt or rate-limit per host.
- Anthropic web search exists (`adapters/anthropic`) but sends no domain restriction. OpenAI calls use Chat Completions without web search.
- The import review allows "Create new" with empty nutrition and "Skip", and cannot edit steps. Matching is by name: real German lines matched the English-seeded catalogue 11 of 72 times (15 %).
- `food.Service.CreateVariant` creates an empty variant; the planner does not treat a variant group as one dish.
- Shopping-list ticks live only in the browser (localStorage).
- H8 (variety) is a hard, relaxable rule: no session of a recipe eaten within `plan.variety_weeks` (default 3); when the library cannot satisfy it, the least recently eaten recipes are allowed with a warning.
- The price table lacks Fable 5.2 and gpt-6.2 and lists Sonnet 5.5 cache reads at $0.20 instead of $0.10 per 1M.
- Plantry runs over plain HTTP on the LAN; browsers refuse the microphone there.

---

## 3. Concepts

| Term | Meaning |
|---|---|
| **Chef on/off** | `chef.enabled`. Off: no autonomous Chef work and no Chef UI; everything else as today |
| **Recipe attributes** | Cuisine, dish kind, 1–3 hero ingredients, final texture, served temperature, saucy or dry, richness, colour family, a 0–3 taste profile (salt, acid, sweet, bitter, umami, heat), technique, format |
| **Plate fit** | The rule-based culinary reading of a plate from its components' attributes: *clashes* (a candidate is not offered; in the LLM chooser path a checker violation) and *gaps* (what the plate lacks; a ranking key and the source of tips) |
| **Pairing style** | Per cuisine: *bridge* (sides share flavours with the main) or *contrast* (sides supply what the main lacks) |
| **Cuisine palette** | Per cuisine, the sides and finishes that belong to it |
| **Tip** | One sentence and an **Apply** that holds a checked change for one plate |
| **Find** | A recipe the assistant found or designed, with sources, adaptation, checks, cost and a state (§5.7) |
| **Ready find** | A checked find not yet in the library; the planner and the daily idea can use it |
| **Origin label** | *Adapted from* (light or medium), *Inspired by*, *Created by the assistant* |
| **Trial** | A Chef recipe in the library that has not yet been cooked without a dislike; planned at most once at a time |
| **Needs your check** | Recipes the user asked for, imports and legacy recipes with a line that is not matched; they wait for the user |
| **Reserved cell** | A draft plate whose main is still being found; it carries a budget the find must meet (§5.6) |
| **Taste item** | One belief about the household's taste (level, key, polarity, scope, source, evidence) |
| **Staple** | A recipe the household keeps cooking and liking; never excluded by soft rules, still bound by hard rules |
| **Week theme** | A focus for one planning window (always 7 days from the shopping day): cuisine, ingredient or season, mood or format |
| **Basics at home** | Foods the household always has; not a pantry: no quantities, no stock |
| **Kitchen** | Active time weekday and weekend, skill, equipment, heat and flavour leanings |
| **Spend** | The provider cost of the assistant's own (capped) work this month, from the run log |
| **Planning session** | The user generating or regenerating a window, or the plan page open on a window that has drafts, plus 30 minutes after the last such action *(tunable)* |

---

## 4. Principles

- **P1 One assistant.** Reached through context entry points; the plan panel stays the place for free conversation. No global chat button, no separate tab.
- **P2 Low effort.** The assistant acts on its own; a Chef recipe counts as accepted unless the user opens it and changes or rejects it. A trial protects the user from an untested recipe (§5.8).
- **P3 Honest numbers, no exceptions.** Every ingredient line of every recipe, variant, substitution and garnish is matched to a food in the database that has nutrition. The assistant searches the catalogue, FDC and Open Food Facts and imports what it finds; a line that cannot be matched with confidence is an error: the recipe is not saved and not planned. Nutrition is never estimated by a model. Amounts follow §5.5 (MATCH-6).
- **P4 Real sources, own words.** Recipes come from real, fetched pages, cross-checked, adapted and written in Plantry's words with the source linked. The assistant never copies a source photo.
- **P5 Calm.** The assistant speaks only when silence would cost a meal or a purchase, or when it can lift a plate with one tap.
- **P6 Consistent.** The same plate gets the same reading every time.
- **P7 Rules win.** Hard plan rules, then nutrition targets, then plate fit, then preferences. Taste never breaks a target or a hard rule.
- **P8 Drafts and undo.** Plans stay drafts until accepted; every change the assistant makes to a plate, recipe, taste item or list can be undone.
- **P9 Optional AI.** §11.1 lists every surface with and without a provider key.
- **P10 Visible memory.** Everything the assistant learned is shown and editable.
- **P11 No idle spend.** The assistant searches on its own only during a planning session or for a theme the user set.

---

## 5. Requirements

Requirement ids are stable; plans and tests cite them.

### 5.1 Switch and rollout (ON)

- **ON-1** `chef.enabled` (default **off**) gates all autonomous Chef work (finds, attribute labelling, import checks, library review) and the Chef UI. Some parts reach every user regardless, as listed in §11.1: rule-based plate fit in planning, derived attribute chips, the Kitchen settings and "Probably at home", archive, the Taste page with migrated restrictions, the taste key in planning, P3 in the import review, and Discover no longer copying photos. Each ships with a release note.
- **ON-2** Turning it on runs the one-time attribute labelling of the library (ATTR-2) and shows the spend estimate first.
- **ON-3** The default flips to on for new installs only after the ship gate (AC-20) passes, including the owner's 2–3 weeks of cooking.

### 5.2 Recipe attributes (ATTR)

- **ATTR-1** Every composed food has recipe attributes. Leaves used as sides, toppings or garnishes carry the applicable subset (taste, texture, colour family).
- **ATTR-2** The assistant sets attributes when a recipe is saved, imported, adapted or edited, and once for the existing library when the Chef is turned on (counted against the cap).
- **ATTR-3** Attributes are checked against the ingredients: an acid taste needs an acid ingredient; richness follows the fat share of energy; hero ingredients are ingredients of the recipe by weight. A failed check drops the value; nothing is invented.
- **ATTR-4** Without a provider, attributes derivable from data are computed (hero ingredients by weight, richness, saucy from liquid share, cuisine from existing tags); the rest stays empty, and rules treat empty as "unknown, no judgment".
- **ATTR-5** Attributes are chips on the recipe page; each is editable; a user value always wins and is never overwritten.

### 5.3 Plate fit and composition (PLATE)

- **PLATE-1** Plate fit reads ten dimensions: taste balance (salt, acid, fat, heat), a fresh element, texture contrast, colour, temperature contrast, cuisine coherence, pairing style, sauce–starch fit, proportions, richness.
- **PLATE-2** Plate fit is computed by rules from attributes, without a model call, deterministically.
- **PLATE-3** Pairing style, palette and thresholds are defined per cuisine for the first set: German, Italian, French, Mediterranean, Indian, Chinese, Japanese, Thai, Mexican, Middle Eastern, and a neutral fallback for anything else. Classic plates of a defined cuisine (listed in the eval set) raise nothing.
- **PLATE-4** Decision order in the heuristic, the LLM chooser's checker and `find_alternatives`: hard rules → nutrition → plate fit → existing keys (repeats, last eaten, favourite, staple, taste).
- **PLATE-5** A clash removes a candidate in the heuristic's side and add-on choice and after a main swap (today's misfit rules). In the LLM chooser path a clash is a violation **only in the repair input** of that chooser (existing repair rounds, no extra rounds); it never enters the shared checker that guards `apply_changes` and the assistant's day tools, so the assistant never refuses a user's request because of plate fit. Gaps rank candidate sides and add-ons. `find_alternatives` returns a short fit phrase per candidate.
- **PLATE-6** Batches: the repeated main is never judged as a repeat. Plates of one batch vary their side, topping or finish by default, within nutrition (`chef.batch_variation`, switchable in Settings).
- **PLATE-7** A gap the planner fails to fill in two consecutive windows is queued as a library gap for the next planning session (FIND-1b).
- **PLATE-8** Pairing suggestions shown on a recipe are derived on read from plate fit over the library; they are not stored.
- **PLATE-9** A kill switch (`plan.plate_fit`, default on, not shown in the UI) turns plate fit off in the planner; plate fit and the taste key bump the heuristic version recorded with each run.

### 5.4 Tips (TIP)

- **TIP-1** A tip appears only on a plate with a clear gap or clash; other plates carry no mark.
- **TIP-2** Kinds: finishing acid, fresh herbs, crunch, a cool element, a quick pickle, a sauce, a contrast side swap, a cuisine-clash fix. Technique advice is never a plate tip.
- **TIP-3** A tip is shown only when its Apply holds a change that already passed: diet, allergies and never items, equipment, active time (≤ 5 minutes for an addition; a side swap stays within the slot's allowance), the day's nutrition targets, and P3.
- **TIP-4** Apply changes this plate only: it adds the item with an amount per eater (or swaps the side). Nutrition and the shopping list follow (basics at home are not listed). It is a normal plate change with Undo. Tips are computed by rules: no model call, no cost.
- **TIP-5** Later (not in the first release): after the second Apply of the same tip on the same recipe, the assistant offers once to make it the recipe's finishing touch.
- **TIP-6** Limits per window: one per plate, at most one across the plates of a batch, at most three in total *(tunable)*. Dismissing a tip cools it down for that recipe for the window; three dismissals of one kind within eight weeks silence that kind for eight weeks.
- **TIP-7** Tips show on the plan board and on Today. On a confirmed plate, Apply goes through the unlock rules.

### 5.5 Ingredient matching (MATCH)

- **MATCH-1** Order: clean the line (alternatives, ranges, "Salz und Pfeffer", "nach Geschmack") → catalogue match by meaning, across languages → FDC → Open Food Facts → for Chef finds only: a substitute with a known ratio (recorded as a change), then the next version of the dish.
- **MATCH-2** Confident means the same food in the same state (dry vs cooked, raw vs canned), plausible kcal per 100 g for its food group, and an amount that converts to grams.
- **MATCH-3** A new ingredient enters the catalogue only after a duplicate check and records its origin (find or import). When its recipe is rejected and nothing references it, it is removed; otherwise it stays.
- **MATCH-4** One matching path serves imports, Discover, finds, substitutions and tips.
- **MATCH-5** The import review drops "Create new" without nutrition and "Skip". A line without a confident match blocks Save and offers: search again with another name, pick from catalogue/FDC/OFF results, create a leaf with entered nutrition per 100 g, or remove the line. This applies with the Chef on or off.
- **MATCH-6 Amounts.** "To taste" lines of salt, pepper, dried spices and fresh herbs are matched to their food with no amount and count no nutrition (they are below the precision of the numbers); they are shown as "to taste". Any other line without a quantity (oil for frying, butter for the pan) needs an amount: the assistant writes explicit amounts in its own recipes; for imports the import check pre-fills a proposed amount that the review shows as proposed and the user can change. A proposed amount is an ingredient amount, not a nutrient value.
- **MATCH-7** Without a provider, matching uses the catalogue, FDC and OFF name search plus a bundled bilingual (en/de) synonym table for common ingredients. Target: ≥ 60 % of the 72-line German test set matched without a key *(tunable)*; with a key ≥ 95 %.

### 5.6 Reserved cells (RES)

- **RES-1** When a window needs new or theme dishes that no ready find covers, the planner places them as reserved cells. A reserved cell is stored apart from plates (no plate or cook session exists until it is filled, because both need a real food); the plan view shows it in place of a plate. It carries the sides chosen so far and a budget: role, kcal and protein band for the plate (so the rest of the day stays balanced), portions, batch size and the leftover cells it feeds, required keep days, slot time allowance, theme, diet and never items.
- **RES-2** The find must meet the budget; if it cannot within its search, the planner fills the cell (and its batch cells) from the library and says so in one line.
- **RES-3** A reserved cell counts as meeting its budget for the checker and day totals; it is excluded from the shopping list and is never shown on Today (drafts never are).
- **RES-4** Transitions:
  - The find arrives → one transaction writes a real draft plate and cook session (and the batch's leftover plates) with the found recipe (as a trial recipe, §5.8) and removes the reservation; this is the only change the planner makes to something the user has seen, and it is announced in the live region.
  - The user regenerates, replaces, moves, deletes the plate or marks the cell No plate → the reservation ends; a search already paid for completes and becomes a ready find.
  - The user removes the theme → reservations for it end the same way; their cells are filled from the library.
  - The user accepts the window while a search runs → the cell is filled from the library at once; the find becomes a ready find.
  - The user leaves the page → the search continues on the server.
  - A search fails (offline, cap reached, no fitting recipe) → RES-2.
- **RES-5** A later, better find never replaces a plate the user has seen; it becomes a ready find.

### 5.7 Finding recipes (FIND)

**Triggers and timing**

- **FIND-1** Triggers: (a) a window that needs new dishes (New dishes per week) or has a theme, during a planning session; (b) a queued library gap or a cell with no fitting option, during a planning session; (c) a request from the user (chat or entry point); (d) a theme the user set for a future window; (e) Fill my library (COLD-3).
- **FIND-2** No standing shelf and no search outside a planning session except (c), (d) and (e). The daily idea shows a ready find and never searches by itself; with no ready find it does not appear that day.
- **FIND-3** Ready finds are used before new searches when they fit. They do not expire.
- **FIND-4** Find states:

| State | Meaning | Next |
|---|---|---|
| searching | search, fetch, cross-check, adapt, match, gates | ready · failed · needs_check |
| ready | checked, not in the library; usable by the planner, the daily idea and the user | in_library (placed in a plan, added from the daily idea, kept by the user) · discarded (user) |
| needs_check | a requested find failed a gate the user can fix | in_library (fixed) · discarded |
| failed | an autonomous find failed a gate or found nothing | — (logged; the next candidate is tried) |
| in_library | a recipe with origin label; trial state per §5.8 | — |
| discarded | the user said Not for me before it reached the library | — (reason feeds TASTE) |

**Search and fetch**

- **FIND-5** Search uses the provider's web search restricted to enabled sites (Anthropic: `allowed_domains`). Queries carry dish, cuisine, diet and language only, never personal data.
- **FIND-6** Fetching obeys robots.txt groups for `*`, for `Plantry`, and for AI user agents acting for a user: `Claude-User`, `ChatGPT-User`, `ChatGPT Agent`, `Perplexity-User`, `MistralAI-User`, `Gemini-User` *(list maintained in code)*. Groups naming only training or search crawlers (`GPTBot`, `ClaudeBot`, `anthropic-ai`, `Claude-SearchBot`, `CCBot`, `Google-Extended`, `Bytespider`, …) do not apply. A site may carry `ai_groups_exempt` (set only for Chefkoch by owner decision): its AI user-agent groups are skipped, `*` and `Plantry` never are. Robots files are cached per host for 24 h; one request per host per second; a host answering 401/402/403/429 is skipped for 24 h. No paywall or block is bypassed; the user agent stays honest.
- **FIND-7** Default sites — EN: BBC Good Food, Budget Bytes, Minimalist Baker, Just One Cookbook, The Woks of Life. DE: Chefkoch (exempt per FIND-6; recipes with ≥ 50 votes), Einfach Backen, Küchengötter, EAT SMARTER, Malte's Kitchen, Zucker&Jagdwurst. Off by default: Bon Appétit, Epicurious, Kitchen Stories, RecipeTin Eats, Essen & Trinken, NYT Cooking, Serious Eats, AllRecipes and sister brands. The user can enable, disable and add sites. A link the user pastes always imports.
- **FIND-8** Providers without real web search (OpenAI today) do no autonomous search; requests still run and say the result may be thinner.

**Rank, cross-check, adapt**

- **FIND-9** Extraction adds `aggregateRating` (normalised by best/worst rating), `ratingCount`, `reviewCount` and `datePublished` to the existing author.
- **FIND-10** Ranking: Bayesian rating × source tier (test kitchen > vetted blog > user-submitted) × completeness *(constants tunable; start: prior 4.0 over 30 votes; tiers 1.0/0.95/0.8)*. Candidates under 10 votes are dropped unless editorial.
- **FIND-11** Cross-check: three versions are fetched; the best-ranked is the base; the others flag outliers (amount per serving, salt per kg, oven temperature, total time, liquid-to-starch) beyond 2 robust deviations. Versions are never averaged. A single source suffices at ≥ 4.5 from ≥ 200 votes, not baking, passing the gates, within a light adaptation.
- **FIND-12** Adaptation follows taste, heat, diet, never items, kitchen and portions; substitutions use known ratios; a protein swap changes time and safe core temperature. Technique and baking chemistry stay unless the origin label changes.
- **FIND-13** Origin labels: *Adapted from* light (≤ 2 swaps and ≤ 15 % of ingredient weight) or medium (≤ 4 swaps or one protein swap, ≤ 30 %); *Inspired by* beyond; *Created by the assistant* for own designs of sides, sauces, salads and simple components.
- **FIND-14** Steps are written in Plantry's words in the user's language; ingredient facts kept; source URL, site and author stored and shown. Finds and Discover never copy photos; a Chef recipe has no image until the user adds one.
- **FIND-15** Cuisine scope outside a theme: cuisines the household eats (from its library and history), Cuisines to explore, and with an empty library the cold-start answer (COLD-1) and the locale's home cuisine. Other cuisines only when the user selects or requests them.
- **FIND-16** A find fits the kitchen: no step needs missing equipment; active time fits the allowance (KIT-1); technique fits the known skill.
- **FIND-17** At most two specialty ingredients new to the household per window *(tunable)*.

### 5.8 Library entry, trial and review (NEW, REV)

- **NEW-1** A find enters the library (FIND-4 in_library) with its origin label and a **trial**. A trial recipe is placed in at most one cook session per window until it has been cooked without a dislike.
- **NEW-2** The trial ends when the recipe has been eaten (a past confirmed plate) and two days have passed without a "disliked" or "not eaten" rating. Today's existing feedback prompt is the channel; no new status is added.
- **NEW-3** A **To try** badge shows on trial recipes in the library and on plates; opening the recipe clears the badge, not the trial.
- **NEW-4** "Not for me" on a ready or trial recipe: a ready find is discarded; a library recipe is archived (hidden from planning and lists, kept for history because plates reference it). The one-tap reason feeds TASTE; "looks wrong" lowers the source's rank instead. A disliked first cooking archives the recipe the same way and asks for the reason.
- **NEW-5** Archived recipes can be restored from the library's filter.
- **REV-1** One review screen serves URL imports, Discover and Chef recipes: source line with rating and votes; origin label; what changed and why; flags with Apply/Keep; ingredients new to the catalogue highlighted; nutrition per portion.
- **REV-2** Name, portions, times, ingredients (amount, unit, food) and steps are editable inline: a sheet per ingredient on phones, two columns on desktop.
- **REV-3** Needs your check lists problem lines first, shows what was tried, and offers the MATCH-5 fixes.
- **REV-4** The daily idea's "Add" puts the find in the library on trial; the review stays one tap away and is not required (replaces UX spec §5.9's mandatory review).

### 5.9 Own recipe changes and recipe chat (VAR)

- **VAR-1** A change request ("spicier, with coconut yoghurt") creates a filled variant in the recipe's variant group with a changes list, through MATCH and GATE.
- **VAR-2** "Change the recipe itself" edits in place with a before/after diff and Undo. As with manual edits today, past plates show the recipe as it now is; when the recipe sits on confirmed future plates, the assistant says so before saving (their shopping changes).
- **VAR-3** The planner treats a recipe and its variants as one dish for H8 and repeats.
- **VAR-4** What the assistant may do on recipes from chat or an entry point: create a variant (no confirmation needed; undoable), edit in place (only on an explicit request; diff shown; undoable), set or correct attributes (user values stay), start a find, propose taste items (confirm chip, TASTE-8). It never deletes a recipe; archive is the user's action.
- **VAR-5** The change log and Undo extend to recipe, variant, attribute, taste item and batch override changes; Undo is refused once a later change depends on the row (as for plans).

### 5.10 Gates (GATE)

- **GATE-1 Block** (not saved, not planned): an unmatched line; an allergen, never item or diet conflict, including hidden ones (pesto, Worcestershire, fish sauce, gelatine); a core temperature below the stricter of USDA and BfR for mince, poultry, pork and fish; dried kidney beans not boiled; garlic or herbs in oil stored; canning or preserving; keep days beyond the safety table; missing name, ingredients, steps or yield; baking outside its ratio band.
- **GATE-2 Slot block:** missing equipment or over the slot's time allowance makes the recipe ineligible for that slot only.
- **GATE-3 Flag** (shown, not blocking): outlier amounts, raw or lightly cooked egg, rare whole-muscle beef or lamb, fewer versions checked than FIND-11 wants, high sodium per portion.
- **GATE-4** An autonomous find that fails GATE-1 is dropped (failed) and the next candidate tried; a requested find waits under Needs your check.

### 5.11 Taste model (TASTE)

- **TASTE-1** Taste items exist at five levels: hero ingredient, cuisine, flavour (taste dimensions and leanings), technique, format. Never items come only from the user.
- **TASTE-2** Signals *(weights tunable; starting values)*: loved +1.0, disliked −1.5, favourite +1.5, plate added by hand +0.6, plate removed or replaced in a draft −0.3, eaten +0.1 relative to how often offered, review swap −0.5 removed / +0.3 added, Not for me by reason, tip applied +0.3 / dismissed −0.2 on its flavour dimension. "Too long" is time evidence for KIT-1, not taste. Ignored tips teach nothing.
- **TASTE-3** Blame: a plate signal goes 70 % to the main, 30 % to the sides; within a recipe 40 % to hero ingredients and 15 % each to cuisine, flavour, technique and format. Fractional blame counts fractionally towards n.
- **TASTE-4** Learned score = evidence ÷ (n + 3): one implicit signal never flips an item.
- **TASTE-5** A confirmed statement ("I hate coriander", [Always]) sets the item directly and outranks learned evidence; it may flip an item.
- **TASTE-6** Long-term evidence fades: half-life about 4 months for likes, 8 for dislikes *(tunable)*; confirmed statements do not fade. Short-term items (this window's theme, "just this week" statements) end with the window.
- **TASTE-7** Implicit signals never create a never item.
- **TASTE-8** A chat statement about taste shows a chip [Always] / [Just this week]; without a tap it applies to the current window only. Health statements (allergic, celiac, intolerant) are stored only with the tap, as never items.
- **TASTE-9** Monotony is tracked apart from dislike: exposure per cuisine, protein and format over 14 days with soft caps *(tunable)*.
- **TASTE-10** Migration: restriction chips become never items; `likes`/`dislikes` become weak evidence; `record_preference` writes taste items.
- **TASTE-11** Settings → Taste, in plain sentences: Never · You love · You'd rather not · Leanings · Staples · Cuisines · Kitchen · Theme · Recently learned (with Undo). Each item can be removed, flipped or made never. Visible with or without a key.

### 5.12 Variety, staples and theme (THEME)

- **THEME-1** Staples are learned (≥ 3 cook sessions in 16 weeks, a positive signal, no dislike in 8 weeks) *(tunable)*; the favourite star pins one. Staples get a strong boost and are never excluded by soft rules (monotony caps, fading); they stay bound by hard rules including H8 and its relaxation.
- **THEME-2** A week theme belongs to one window; kinds: cuisine, ingredient or season, mood or format; set in chat or on the plan page; at most two per window.
- **THEME-3** A theme shapes the window's new dishes and the library recipes that match it; target 40–60 % of the window's cook sessions, as far as the library and finds supply. A theme may raise that window's new dishes up to 3 within the cap. "The whole week" only when asked.
- **THEME-4** The assistant suggests at most one theme per window (from Cuisines to explore, the season, recent loves), never applies it itself, and pauses suggestions for 4 weeks after two refusals.
- **THEME-5** A theme ends with its window; the next window offers "again?" with one tap.
- **THEME-6** One setting, **New dishes per week** (0–3), replaces `plan.explore_per_week` (value migrated). Its default is 1 when the Chef is on; with the Chef off it keeps today's meaning (recipes tagged explore that were never eaten) for the heuristic.
- **THEME-7** Later (not in the first release): "rest this staple" suggestions on a falling trend.

### 5.13 Kitchen (KIT)

- **KIT-1** Active cooking time: minutes for weekdays and the weekend (defaults 30 and 60). A batch cook day (a session for ≥ 3 plates) gets the weekend allowance.
- **KIT-2** Skill: beginner / home cook / confident; unknown counts as home cook. The assistant asks once, in context, when a recipe needs an advanced technique.
- **KIT-3** Equipment checklist: oven, stovetop, microwave, air fryer, pressure cooker, slow cooker, blender, food processor, stand mixer, wok, grill. Oven and stovetop default to present; an unknown tool counts as absent and is asked about once, in context.
- **KIT-4** Basics at home: a list of foods, prefilled (salt, pepper, oil, sugar, flour, common dried spices), editable in Settings and from the shopping list ("Always at home" on an item). On the shopping list they sit in a collapsed "Probably at home" group. Not learned from ticks (ticks stay in the browser).
- **KIT-5** Heat tolerance (none / mild / medium / hot) and up to five flavour leanings, learned (TASTE) and editable.
- **KIT-6** The setup wizard gets no new step: a time row in the cooking step and a heat row in the assistant step.
- **KIT-7** Kitchen data is visible and editable without a key; without the Chef it has no effect except the time allowance as a soft preference in the heuristic.

### 5.14 Help on the spot (HELP)

- **HELP-1** Entry points open with their context loaded and one-tap actions first, free text second. Each changes only its own scope.
- **HELP-2 Cook mode:** a Help button (≥ 56 px) with three tiles: "I don't have an ingredient" (pick from the recipe's ingredients), "Explain this step", "Something went wrong" (too salty, too thin, too thick, burnt, not cooking through). Answers are at most three lines, amount first.
- **HELP-3 Shop:** "Not available" in an item's menu gives 2–3 substitutes with shopping amounts.
- **HELP-4 Recipe page:** "Ask about this recipe", with chips "Make a variant" and "Check this recipe".
- **HELP-5 Plan:** the existing panel, with VAR-4 abilities added.
- **HELP-6 Today:** no entry point; tips and reminders only.
- **HELP-7 Substitutions:** offered substitutes are matched foods (imported first if needed, P3). Applying one changes this batch only: shopping list, nutrition and cook-mode amounts follow, with Undo. "Always in this recipe" and "Save as variant" are one tap away; after the same swap twice the assistant offers once to make it permanent.
- **HELP-8** Help in cook mode and shop always runs, without a cap dialog, and counts against the cap.
- **HELP-9** No in-app voice.

### 5.15 Import check (IMP)

- **IMP-1** With the Chef on, every URL import and Discover add runs a check before the review.
- **IMP-2** Applied with Undo per change: missing times, unit normalisation, keep days and freezable (within the safety table), safety cues on steps (core temperature), role, rewriting only unclear steps, prep-ahead lead times (soak, marinate, thaw), proposed amounts for quantity-less lines (MATCH-6).
- **IMP-3** Flagged only, never changed: amounts that are given, and ingredients.
- **IMP-4** Suggested: a finishing touch.
- **IMP-5** The review shows one line ("Checked: 4 changes, 1 to look at"); a clean recipe shows nothing. A finding never blocks Save; P3 does.
- **IMP-6** Imported text is otherwise the author's; a foreign-language import is translated into the user's language, link kept.
- **IMP-7** A link the user pastes keeps today's image download (the user's own explicit choice, a private copy); finds and Discover do not copy images (FIND-14).

### 5.16 Library review (LIB)

- **LIB-1** Opt-in (`chef.library_review_enabled`, default off).
- **LIB-2** At most monthly, one card on Recipes, at most three findings: near-duplicates (make them variants), recipes that keep getting replaced (hide from planning), data the user must fill. Computed by rules where possible; any model call is capped work.
- **LIB-3** Gaps the assistant can fill itself are queued for the next planning session (FIND-1b), not searched at once.
- **LIB-4** A recipe can be hidden from planning (the archive state of NEW-4, set by the user); it stays in the library, can be planned by hand, and can be restored.

### 5.17 Unasked moments (PRO)

- **PRO-1** Allowed: soak, marinate or thaw reminders in Today's reminders (computed without a model call); tips; a fresh element for reheated batch days (PLATE-6).
- **PRO-2** Not allowed: push notifications, anything unasked in Shop, repeating a dismissed suggestion, more than one Chef line on Today.
- **PRO-3** Later (not in the first release): an "Order of work" page in cook mode on batch days.
- **PRO-4** Noise budget: at most five unasked Chef lines per window across all kinds (tips, theme suggestion, trial questions, reminders excluded) *(tunable)*; tested.
- **PRO-5** One "What the assistant did" list per window (plan page): every autonomous change (filled cells, finds placed, tips applied, batch variations, recipes added on trial) with Undo per entry.

### 5.18 Cold start (COLD)

- **COLD-1** One free-text question when the Chef is turned on (or in setup with a key): "Name a few dishes you love, and anything you never eat." The answer becomes proposed taste items with confirm chips.
- **COLD-2** Later (not in the first release): a this-or-that of 8 dish pairs.
- **COLD-3** **Fill my library:** one tap, the assistant adds 10 recipes spread across roles and attributes, on trial, within the cap; they count towards the 10-recipe onboarding goal.
- **COLD-4** Week one is mild and familiar (cuisines per FIND-15). Today's existing feedback prompt stays as it is.

### 5.19 Cost and cap (COST)

- **COST-1** A monthly cap for capped work (`chef.monthly_cap`, USD like the run log, default 5). Capped work: finds, attribute labelling, import checks, help, library review. Planning and chat runs are not capped.
- **COST-2** At the cap, autonomous work stops until the next month; requested work runs after a warning; help in cook mode and shop runs without one (HELP-8).
- **COST-3** A model without a known price does no autonomous work.
- **COST-4** Settings shows this month's spend against the cap; Insights breaks it down by kind.
- **COST-5** The price table is updated (Sonnet 5.5 cache reads, Fable 5.2, gpt-6.2) before the cap ships.

---

## 6. Settings

| Key | Kind | Default | Meaning |
|---|---|---|---|
| `chef.enabled` | bool | false | ON-1 |
| `chef.new_dishes_per_week` | int 0–3 | 1 (used when the Chef is on) | Replaces `plan.explore_per_week` (migrated) |
| `discovery.cuisines` | string list | [] | Relabelled "Cuisines to explore" |
| `discovery.daily_enabled` | bool | true with a provider | Daily idea (from ready finds) |
| `chef.library_review_enabled` | bool | false | LIB |
| `chef.monthly_cap` | number, USD | 5 | COST |
| `chef.batch_variation` | bool | true | PLATE-6 |
| `chef.sites` | JSON list of {domain, lang, enabled, ai_groups_exempt} | FIND-7 | Allow-list |

Kitchen and taste data live on the profile and in taste tables.

---

## 7. Data-model changes

Feature-level; exact columns and migrations belong to the implementation.

**New**
- **Recipe attributes** per food: the attribute fields, a source per field (auto, rule, user), version.
- **Taste signals:** append-only (kind, level, key, weight, time, reference).
- **Taste items:** level, key, polarity (love, rather not, never), scope (always, window), source (learned, statement, user), evidence, n.
- **Week themes:** window start, kind, value, source.
- **Finds:** draft recipe, sources with ratings, cross-check flags, origin label, changes, gate results, state (FIND-4), trigger, budget (for reserved cells), run id, cost.
- **Tips:** plate id, kind, change ops, state (shown, applied, dismissed), times.
- **Batch substitutions:** realised as a hidden variant of the recipe (archived, not listed) that the batch's session and plate components point at, so nutrition, the shopping list, cook mode and every planner fact follow without new code paths; Undo points them back.
- **Food events:** an event store for recipe, attribute, taste-item and substitution changes, the base of their Undo (VAR-5); plan changes keep the existing plate events.
- **Reserved cells:** window, cell, budget, sides chosen, find id, state; read by the planner through a port.
- **Never items and basics at home:** tables with food references, not JSON on the profile.

**Changed**
- **Foods:** origin label, source rating and votes, date published, changes list, trial state, archived (hidden from planning and lists; every food reader honours it), finishing touch (optional last instruction).
- **Food components:** a `to_taste` flag; such a line keeps a nominal amount for the existing positive-amount checks and is skipped in nutrition sums.
- **Profile:** weekday and weekend minutes, skill, equipment, heat, leanings.
- **Catalogue leaves:** origin of an ingredient the assistant added.
- **Run log (`ai_runs`):** the kind list moves from a database CHECK into Go validation (safe table rebuild that keeps runs, change log and plate links); kinds `chef_attributes`, `chef_match`, `chef_import_check`, `chef_find`, `chef_help`, `chef_library_review`; each run stores its cost in USD at the time of the run.
- **Discovery suggestions:** the existing suggestions table evolves into the finds table (states per FIND-4) rather than a second table.

**Migrations of existing data**
- `plan.explore_per_week` → `chef.new_dishes_per_week` (value kept); recipes tagged explore keep the tag.
- `profile.dietary_restrictions` → never items; `preferences.likes/dislikes` → weak evidence.
- Pending Discover suggestions → ready finds (their stored images are not shown for finds; images already on library recipes stay).
- Existing leaves without nutrition and recipes with unmatched lines: listed once under Needs your check; they stay plannable as today (flagged "incomplete", never counted as zero) until fixed. New saves cannot create such data (MATCH-5).

---

## 8. API surface (new or changed)

Indicative; the implementation fixes names.

- `GET/PUT /api/foods/{id}/attributes`; `POST /api/foods/{id}/archive|restore`.
- `GET /api/taste`, `POST/PUT/DELETE /api/taste/items[/{id}]`, `POST /api/taste/statements/{id}/confirm` with scope.
- Profile endpoints gain kitchen fields and basics at home.
- `GET/PUT/DELETE /api/windows/{start}/themes`; `POST …/themes/suggestion/{id}/accept|decline`.
- `GET /api/finds?state=`; `GET /api/finds/{id}`; `POST /api/finds/{id}/keep|discard` with reason.
- Plan reads: plates carry a reserved state (budget, find state) and their tip.
- `POST /api/tips/{id}/apply|dismiss`.
- `POST /api/help` (context: cook session and step, shopping item, recipe; action) → answer and optional change; `POST/DELETE /api/sessions/{id}/overrides`.
- `POST /api/import/extract` — response adds the import check (changes with undo, flags, suggestions, proposed amounts).
- `GET /api/chef/library-review`; `GET /api/chef/spend`; `POST /api/chef/fill-library`.
- Change undo extends from plan changes to the VAR-5 kinds.
- Existing discovery routes are served by the find pipeline; the daily idea reads ready finds.

---

## 9. UI surfaces and vocabulary

**Surfaces**
- Plan board: reserved cells ("Finding a recipe…"), tip mark and popover with Apply, theme picker near the window header, To try badge.
- Today: tips, the existing feedback prompt, reminders (soak, marinate, thaw), daily idea from ready finds.
- Recipe page: attribute chips, origin label and source, changes list, finishing touch, "Ask about this recipe", archive.
- Recipes: Needs your check list, archived filter, library review card (opt-in).
- Review screen: §5.8.
- Cook mode: Help button and tiles.
- Shop: "Not available", "Always at home", "Probably at home" group.
- Settings: Taste page; Kitchen section; Assistant section gains the Chef switch, New dishes per week, Cuisines to explore, sites, library review, cap and spend.
- Insights: Chef block.

**Vocabulary (en / de proposal for UX spec §7; German written natively in the implementation)**

| Concept | en | de |
|---|---|---|
| Tip | Tip | Tipp |
| Apply | Apply | Übernehmen |
| Trial badge | To try | Zum Ausprobieren |
| Adapted from / Inspired by | Adapted from / Inspired by | Angepasst nach / Inspiriert von |
| Created by the assistant | Created by the assistant | Vom Assistenten erstellt |
| Needs your check | Needs your check | Bitte prüfen |
| Reserved cell | Finding a recipe… | Rezept wird gesucht… |
| Week theme | Theme | Thema |
| Staple | Staple | Klassiker |
| Basics at home | Probably at home | Wahrscheinlich zu Hause |
| Not available | Not available | Nicht da |
| Archive | Hide from planning | Nicht mehr planen |

("New" stays reserved for never-eaten recipes and new ingredients, as today; "This week" stays Today's strip.)

---

## 10. Measurement

**Run log.** Every capped run is logged with kind, model, tokens, web searches, cost, outcome and the find it produced.

**Insights → Chef block.** Loved and disliked rate of Chef recipes vs the user's own (shown from 10 ratings); trials passed and recipes archived with reasons; review edits per Chef recipe; import-check changes undone; staple frequency; tip apply rate; spend by kind against the cap.

**Offline evaluation.**
- Free (fake provider, rules, fixtures): plate fit sets (good and flawed plates per PLATE-3 cuisine), tip checks, reserved-cell transitions, robots handling, matching on the 72-line German set without a key, taste maths over simulated weeks, gates on planted unsafe recipes, migrations.
- Paid (needs an eval budget approved by the owner): find quality per theme and constraint, import check on planted errors, substitutions, latency and cost canaries, persona judgments (the household persona, extended to judge food; two more personas: a cuisine explorer, a family with a picky child), blind comparisons of weeks with and without the Chef.
- e2e never calls a real provider or the web; the fake provider scripts Chef runs.

---

## 11. Non-functional requirements

### 11.1 With and without a provider key

| Surface | No key | Key, Chef off | Key, Chef on |
|---|---|---|---|
| Rule-based plate fit in planning | on, with derivable attributes | on | on, full attributes |
| Tips | hidden | hidden | shown |
| Attribute chips | shown (derived, editable) | shown | shown |
| Taste page, Kitchen, basics at home | shown | shown | shown |
| Themes, New dishes per week | hidden (explore quota as today) | hidden | shown |
| Finds, reserved cells, daily idea, Fill my library | hidden | Discover as today, without photo copies | shown |
| Help entry points | hidden | plan panel only | all |
| Import check | off | off | on |
| P3 in the import review | on (MATCH-7 matching) | on | on |
| Library review | hidden | hidden | opt-in |

### 11.2 Other

- **NFR-1 Offline:** finds fail softly (RES-2); nothing else needs the web.
- **NFR-2 Latency:** plate fit adds ≤ 10 % to heuristic generation on the M5 fixture (guarded by the planning and agent CPU budgets in `test-budget.json`); a reserved cell fills in ≤ 3 min p90 and help answers in ≤ 10 s p90 (paid canary).
- **NFR-3 Cost:** a normal month (one planned window a week, New dishes 1, daily idea on) stays under $5 on Sonnet 5.5, measured in paid canaries before the default cap is final.
- **NFR-4 Fetched content:** page text is data, never instructions; the SSRF guard stays; FIND-6 applies to every fetch the assistant starts.
- **NFR-5 Privacy:** search queries carry no personal data.
- **NFR-6 Platform:** pure Go, no CGO, ARM builds, SQLite only, no new external service.
- **NFR-7 Tests:** the test speed contract applies; new packages get budgets; no real provider or web in tests.
- **NFR-8 Accessibility and i18n:** WCAG 2.2 AA; 44 px targets (Help ≥ 56 px); every string in en and de; reserved-cell fills announced in the live region.

---

## 12. Acceptance criteria

Free (fake provider and fixtures) unless marked *paid*.

1. With `chef.enabled` off, an upgraded install shows no Chef UI and logs no capped run in a simulated month.
2. After turning the Chef on, every composed food in a 100-recipe fixture has attributes; user-edited values survive re-labelling.
3. Plate fit returns the same reading for the same plate in 100 runs; the classic plates listed per PLATE-3 cuisine raise no tip.
4. On the sealed plate set, ≤ 10 % of good plates get a tip and ≥ 70 % of planted flaws get the right tip kind.
5. Every shown tip's Apply passes the checker; applying it updates nutrition and the shopping list; Undo restores both.
6. A window with New dishes 1 shows at once with one reserved cell meeting RES-1's budget; a scripted find fills it; each RES-4 transition behaves as specified; accepting first fills it from the library.
7. No Chef recipe in any test contains an unmatched line, a GATE-1 violation or a copied photo.
8. Fetching skips hosts whose robots.txt disallows `*`, `Plantry` or a listed AI user agent, except AI groups on an exempt site; training-crawler groups do not block; blocked hosts back off for 24 h.
9. A week with no planning session causes no capped run except requested help (run log over a simulated idle week).
10. The import review cannot save a recipe with an unmatched line; it can edit steps; "to taste" seasoning saves without an amount.
11. Without a key, ≥ 60 % of the 72-line German set matches; with a scripted key path ≥ 95 %.
12. A change request yields a filled variant with a changes list; the planner never places a recipe and its variant together where H8 forbids it.
13. One disliked plate does not flip a liked item; a never item arises only from the user; a confirmed statement flips an item; the Taste page shows every item with its source and can remove or flip it.
14. A theme set mid-week shapes 40–60 % of the next window's cook sessions where the library and finds supply them, and ends with the window.
15. A trial recipe is placed at most once per window until eaten without a dislike; a dislike archives it; archived recipes are never auto-planned.
16. A substitution in cook mode changes only that batch's shopping list, nutrition and amounts, and can be undone.
17. At the cap, autonomous work stops and requested work warns; help in cook mode still runs.
18. *Paid canary:* reserved cells fill in ≤ 3 min p90; help answers in ≤ 10 s p90; a normal month costs ≤ $5 on Sonnet 5.5.
19. *Paid quality gates:* the persona prefers the Chef week in ≥ 65 % of blind comparisons; ≥ 90 % of substitutions pass; the import check finds ≥ 80 % of planted errors with ≤ 5 % wrong fixes.
20. *Ship gate:* 0 unmatched lines and 0 food-safety misses across all evals; no regression on the free M5 check (full `plantry-eval` run, heuristic and scripted chooser, compared against main) and, if the budget allows, on a fresh sealed chat regression set; AC-18 and AC-19 pass; the final time-lapse trial passes (persona households over 4 simulated weeks, Chef off vs on, blind judging) and the owner has cooked a handful of Chef recipes (owner decision 2026-11-24, replacing 2–3 weeks of real cooking); then `chef.enabled` defaults to on for new installs.

---

## 13. Delivery phases (outline; the delivery order and cuts may differ)

| Phase | Content | Done when |
|---|---|---|
| **A Taste of the kitchen** | ON switch, attributes, plate fit in all planning paths, tips, batch variation, kitchen fields | Plans compose better plates without a key; tips appear only when worth it (AC 1–5) |
| **B Honest ingredients** | MATCH (cross-language, synonym table, P3 in review), review screen editing, import check, archive, legacy Needs your check | No recipe can be saved with an unmatched line; imports match ≥ 95 % with a key (AC 10–11) |
| **C Finds** | Robots-aware fetch, ranking, cross-check, adaptation, gates, finds and states, reserved cells, trial, daily idea from finds, Fill my library, cap and spend | New dishes arrive by themselves within the cap and pass every gate (AC 6–9, 15, 17) |
| **D Taste and themes** | Taste signals and items, Taste page, chat statements, staples, themes, New dishes per week | The plan follows what the household loves and the theme it set (AC 12–14) |
| **E Help** | Entry points, substitutions and batch overrides, recipe chat abilities, variants, library review | Help works at the stove and in the store (AC 16) |
| **F Ship** | Paid canaries and quality gates, owner trial, default on | AC 18–20 |

---

## 14. Out of scope

Pantry tracking and fridge photos, food log, budget planning, per-person taste profiles, social sharing, copied photos for finds, blocked or paywalled pages, nutrition from a model, voice, push notifications, wine pairing, smart appliances.
Later: rest-a-staple suggestions, this-or-that onboarding, Order of work page, a guest-day rule, "use up what was bought", seasonal ranking, a sitemap title index for OpenAI users, per-person dislikes.

---

## 15. Open questions

1. Theme picker placement on the plan page (UI pass).
2. Final German copy.
3. Eval budget for the paid gates (§10, AC 18–19): the earlier AI cap ($20) is nearly spent.

---

## 16. Decisions log

**Owner (2026-11-24):** one assistant, internals hidden · Chef recipes drafted by the assistant, which searches and imports ingredients; review optional; not opened = accepted · own recipes: variant by default, in place on request · plate judgment is the core; built into planning; a tip only when worth it · taste learned from feedback and chat; staples + week theme + quota; special cuisines only when selected or requested · constraints: time (short weekdays, open weekends), skill, basics at home, equipment, heat and flavour leanings · autonomous web search; robots.txt respected; Chefkoch allowed as an exception; Bon Appétit, Epicurious, Kitchen Stories off · help: substitutions, import fixes, library review opt-in · recipes carry pairing suggestions, finishing touch, source and changes · triggers: planning, gaps, daily idea (optional), monthly cap · Discover becomes the find pipeline's front door · unmatched ingredient is an error everywhere · timing on demand with reserved cells, no shelf, no idle spend · ship gate includes 2–3 weeks of owner cooking.

**Resolved in draft 2 (owner may overrule):**
- Chefkoch exemption is a per-site flag that skips AI user-agent groups only, never `*`.
- `chef.enabled` exists and defaults to off until the ship gate.
- Daily idea never searches; library-review gaps wait for the next planning session.
- Rejected recipes are archived, not deleted (history references them).
- "To taste" seasoning needs no amount; other quantity-less lines need an explicit amount (proposed by the import check, visible).
- Legacy unmatched data stays plannable and flagged until fixed; new saves are strict.
- Staples are bound by H8 like every recipe.
- A theme may raise that window's new dishes to 3.
- Basics at home are set by hand ("Always at home"), not learned from ticks.
- Pasted-link imports keep their image; finds and Discover never copy photos.
- The trial ends only after a cooking without complaint; the badge clears on opening.
- Deferred: rest-a-staple, this-or-that, Order of work, weekly cap release, adaptive tip halving.

**Resolved later (2026-11-24):**
- A plate-fit clash is a violation only in the LLM chooser's repair input, never in the shared checker (the assistant would otherwise refuse user requests).
- Reserved cells are stored apart from plates; filling writes a real plate and session.
- Batch substitutions are hidden variants; recipe undo gets a food event store.
- `to_taste` is a component flag with a nominal amount; the run log's kind check moves into Go and runs store their cost.
- Batch variation's automatic switch-off and TIP-5 are dropped from the first release; pairing suggestions are derived, not stored; plate fit has a kill switch.
