# Plantry — Product Definition

## Elevator Pitch

Plantry is a self-hosted weekly meal planner for home cooks who care about what they eat. You keep a library of recipes you actually cook, say when you are home, and Plantry drafts a week around batch cooking and your nutrition targets. You review it, adjust it with the assistant or by hand, accept it, and the shopping list and a daily "cook today" view follow. Everything runs on your own hardware — a Raspberry Pi, NAS or laptop — from one Docker container. No cloud. No subscription. No data leaves your machine except the AI calls you opt into.

Status: implemented through Phase 5 (2026-11). This file is the product definition; `PLANNING_ASSISTANT_SPEC.md` and `UX_DESIGN_SPEC.md` are the design record, `ARCHITECTURE.md` the technical contract.

## The Problem It Solves

Meal planning apps force one of three bad trade-offs:

1. **Cloud-first SaaS** — convenient, but it owns your data, pushes ads and paywalls features.
2. **Generic tools** (spreadsheets, notes, paper) — private but dumb. No nutrition, no shopping list, no help.
3. **Local recipe managers** (Mealie, Paprika) — private but passive. They store recipes; they do not plan.

Plantry is local-first, plans for you, and is modelled around how people actually eat.

## The Core Insight

A real dinner is a **main plus a side or two**, not one recipe. Plantry's planning unit is the **plate**: a composition of 1..N components, each a recipe (a composed food) or a plain ingredient. Swapping rice for naan or adding a side is one action. A tofu curry is a variant of the chicken curry, linked through a variant group, not a duplicate.

The second insight is **batch cooking**. People cook once and eat the leftovers. A cook session feeds several plates over several days, and a surplus can go to the freezer. The planner reasons about cook days, not just meals.

## Target User

**Primary:** the home cook who wants control. Cooks most dinners at home, often for a household of two to four, tracks macros loosely or has dietary rules, and runs a home server or Pi.

**Secondary:** fitness-minded people who want targets respected, and privacy-conscious households sharing one planner on the home network. The deployment is single-user and LAN-only; there is no login.

## Core Value Proposition

- **Self-hosted and private.** Your data, your hardware, one SQLite file.
- **It plans for you.** The user states availability and preferences; the app produces a checked, reviewable draft. Nothing reaches Today or the shopping list until accepted.
- **Works without AI.** A deterministic heuristic generates plans. An AI provider (your own OpenAI or Anthropic key) adds the LLM chooser, the assistant and discovery.
- **Trustworthy numbers.** One nutrition implementation computes everything from ingredient data. The LLM never supplies a nutrient value.
- **Small and durable.** A pure-Go binary (no CGO) with the web app embedded; builds for amd64, arm64 and arm/v7.

## Feature Map (by user goal)

### "Plan my week"

- A plan board for the window that starts on your shopping day: 7 days × time slots (rename, reorder, add or remove slots in Settings)
- Availability per cell: **Plan**, **No plate** (not eating at home) or **Routine**; weekday defaults plus per-date overrides, and guests per day
- **Generate** for the whole window or the empty cells. The hard rules (diet compatibility, repeats, cook days per day, batch and freezer rules) are checked in Go for both the heuristic and the LLM, with up to two repair rounds
- **Draft versus confirmed.** Generated plates are drafts: editable, never in Today or Shop. Confirm a batch, or **Accept and shop** for the window; accept and discard have Undo
- **Mid-week re-plan** when a cell becomes No plate after the plan is confirmed
- An **explore quota** (`plan.explore_per_week`): the generator places up to N recipes tagged explore that you have not yet eaten
- Routines: saved compositions applied to a day

### "Cook once, eat several times"

- Cook sessions link plates as fresh, leftover or frozen; the board shows batch badges ("Cook × 4", "2/4") and a cook strip
- Freezing is opt-in per recipe: portions go to the freezer only for recipes explicitly marked freezable
- Cook mode: one step per screen, timers detected from the step text, wake lock

### "Refine the plan with the assistant"

- A panel on `/plan`: ask in plain words ("swap Tuesday's dinner for something lighter")
- It reads the library, profile and targets and edits drafts through tools; every edit is a change line with Undo
- It never touches confirmed plates; it asks you for an unlock in the chat
- Undo is refused once a changed row has been confirmed

### "Know my nutrition"

- Kcal, protein, fat and carbs per portion, plate and day against targets (presets for cut, maintain, bulk, or custom)
- Incomplete data is flagged, never silently zero

### "Manage recipes and ingredients"

- Recipes with role, tags, image, times, steps, reference portions, diet class and variants
- Ingredients: barcode scan and Open Food Facts, USDA FoodData Central search, manual entry, per-unit gram definitions
- **Import from a URL**: JSON-LD first (no AI cost), an LLM only as a fallback; a review step resolves each ingredient line before saving

### "Find new recipes" (Discover)

- A **daily idea** over Today (turn it off with `discovery.daily_enabled`), rotating through your chosen cuisines
- A **Discover sheet** in Recipes: explore a cuisine, or find recipes built from ingredients you already use ("known" mode)
- A research call asks the model for candidate pages (with web search on Anthropic models); the app then fetches **every page itself**, keeps only genuine recipes, drops duplicates of your library and earlier suggestions, and computes nutrition from your ingredient catalogue
- The card shows photo, facts, why it was picked, ingredients (new ones are marked) and the source link
- **Add** opens the normal import review, pre-filled and tagged explore; **Not for me** records a reason (too long, ingredients, taste) that feeds the next research
- Needs a provider; without one the Discover entry points are hidden and nothing else changes

### "Know what to buy"

- The shopping list comes from confirmed cook sessions and plates (drafts only with a switch), summed in base units, rounded up for countable items, grouped by store section
- Ticks persist locally; an in-store mode with large rows

### "Keep it running"

- Settings: household and targets, meals and week, cooking rules, assistant (provider, model, key, cuisines), system (version, data)
- **Insights**: every AI run with prompt version, tokens, cost (web search fees included, price table dated) and outcome; discovery acceptance rate
- `plantry backup -out <file>` makes a consistent copy of the database and its `secret.key` while the server runs
- English and German

## Differentiators

1. **Plates and batches, not recipes.** The model matches how people eat.
2. **A checked plan, not a suggestion.** Rules are code; the LLM proposes and the checker disposes.
3. **Drafts first.** Nothing the AI does is final until the user accepts.
4. **AI without lock-in.** Bring an OpenAI or Anthropic key, or none.
5. **Verified discovery.** Suggested recipes are fetched and parsed by the app, so links are real and nutrition is computed, not hallucinated.
6. **Pure Go, single binary.** Runs on a Pi.

## Scope Boundaries — What Plantry Is Not

- **Not a food log.** It plans ahead.
- **Not a pantry tracker.** Without real-world sync a software pantry is a lie.
- **Not a social recipe platform.** No sharing, feeds or public recipes.
- **Not a grocery delivery app.** The shopping list is a list.
- **Not a fitness tracker.**
- **Not a SaaS.** No hosted option, no accounts.
- **Not opinionated about your diet.**

## Design Principles

1. **Local-first.** The app works offline; FDC, Open Food Facts and AI features degrade gracefully.
2. **Fast.** SQLite, small payloads, skeletons instead of spinners.
3. **Empty catalogs are fine.** First run is a setup wizard and a guided library with three ways in: import, discover, create. No seeded recipes.
4. **The AI is a tool, not a dependency.** Everything is usable with AI off.
5. **Macros over micros.**
6. **Tests are the contract.** Domain rules have unit tests, features have e2e tests, and e2e never calls a real provider or the web.
7. **Calm.** The Cookbook look: paper ground, ink-blue accent, Newsreader headings, sentence case, no exclamation marks. See `DESIGN.md`.

## Success Criteria

In the first hour a new user can:

1. Deploy with `docker compose up` on a Pi or laptop and finish the setup wizard.
2. Import or create ten recipes (the guided library counts to ten).
3. Mark when they are home and generate a week.
4. Review the drafts, ask the assistant for one change, and **Accept and shop**.
5. Open Today, cook from cook mode, and rate the meal.
6. Use the shopping list in the store.
7. Back up with `plantry backup` and restore it.

Without any API key, every step except the assistant and Discover works through the heuristic.
