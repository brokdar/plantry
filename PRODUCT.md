# Product

<!-- impeccable:product-schema 1 -->

This is the summary for design work. The full product definition is [`docs/PRODUCT.md`](docs/PRODUCT.md). The screens, voice and accessibility rules are in [`docs/UX_DESIGN_SPEC.md`](docs/UX_DESIGN_SPEC.md), and the visual system is in [`DESIGN.md`](DESIGN.md). If this file and those disagree, those files win.

## Platform

web

## Users

Primary: any self-hoster who installs Plantry from GitHub, meaning a home cook who runs a Raspberry Pi, a NAS or a home server. They cook most dinners at home, often for a household of two to four, track macros loosely or follow diet rules, and care about privacy. They meet Plantry as strangers: no one is there to explain it, and the library starts empty.

Secondary: people focused on fitness who want their targets respected, and households sharing one planner on the home network.

Jobs: plan the week, cook once and eat several times, know what to buy, cook today's meal, and find new recipes.

## Product Purpose

Plantry is a self-hosted weekly meal planner. The user keeps a library of recipes they actually cook and says when they are home. Plantry then drafts a week around batch cooking and nutrition targets. The user reviews the draft, adjusts it by hand or with the assistant, and accepts it, and the shopping list and the Today view follow from it.

Success: within the first hour a new user deploys Plantry, finishes setup, builds a library of about ten recipes, generates and accepts a week, shops from the list and cooks from cook mode. The full list is in docs/PRODUCT.md, "Success Criteria".

## Positioning

- **Plates and batches, not recipes.** A planned meal is a plate made of 1..N components, and one cook session feeds several plates across several days.
- **A checked plan, not a suggestion.** Hard rules are written in Go. The heuristic or the LLM proposes a plan, and the checker decides what is allowed.
- **Drafts first.** Nothing generated reaches Today or Shop until the user accepts it.
- **Local and optional AI.** Everything runs on the user's hardware. A user-supplied OpenAI or Anthropic key adds the assistant, the LLM chooser and Discover; with no key, everything else still works.
- **Trustworthy numbers.** Nutrition is always computed from ingredient data and never comes from a model. Discovered recipe pages are fetched and checked by the app itself.

## Operating Context

- **Planning:** done on a desktop or iPad, usually as a session of 10–15 minutes on the weekend.
- **Viewing:** mostly on a phone, on cooking days (Today, then cook mode at the stove with wet hands, the screen kept on) and in the store (the shopping list held in one hand).
- **Library work:** importing and editing recipes happens mostly on desktop, and browsing and quick edits on a phone.
- Every flow has to work end to end on a phone, and touch is a first-class input on phones and iPads.
- Plantry runs on the LAN for a single user, with no login. It works offline. Open Food Facts, USDA FDC and the AI features degrade gracefully when they are unavailable.

## Capabilities and Constraints

- Features: plan board (7 days × slots), availability (Plan / No plate / Routine), generation, draft vs confirmed, batch cooking and freezer, assistant panel, Today, cook mode, recipes and ingredients (barcode, OFF, FDC, URL import), Discover, the Chef (plate tips, substitutions and variants, Taste page, finds, themes and New dishes, help in cook mode, recipe sites, monthly library review), shopping list with in-store mode, Settings, Insights.
- Out of scope: food log, pantry tracker, social features, grocery delivery, fitness tracker, SaaS or accounts, telling users what diet to follow.
- One Go binary with the React SPA embedded and one SQLite file, cross-compiled to amd64, arm64 and arm/v7.
- Vocabulary is fixed: Plate, Batch, Leftover, No plate, Routine, Draft/Confirmed, Assistant, Recipe/Ingredient (the en/de table is in UX_DESIGN_SPEC §7). Labels never use "AI", "LLM" or "agent".

## Brand Commitments

- Name: Plantry.
- Voice: a plain, calm tool. Short factual sentences, sentence case, active verbs. No exclamation marks (the only exception is the 10-recipe milestone toast), no "Oops", no apologies. The assistant states facts and explains only when asked.
- Bilingual: every string exists in English and German, and the German copy is written natively rather than translated word for word.
- Visual stance (user decision, 2026-11-15): the familiar, modern meal-planner look, done with real craft, is preferred over a themed world. The finish bar is Linear's precision with Mealie/Paprika's category familiarity. Explicitly ruled out: cold and corporate (navy on blue-grey), flat and grey, cream plus serif, busy colour, and tall vertical cards on the plan board.

## Evidence on Hand

- Product and UX record: docs/PRODUCT.md, docs/UX_DESIGN_SPEC.md, docs/PLANNING_ASSISTANT_SPEC.md, docs/ARCHITECTURE.md.
- Assets: frontend/public/favicon.svg, frontend/public/manifest.json.
- Not available: no testimonials, user counts, press or seeded recipes. Never make them up.

## Product Principles

1. **The stranger test.** A self-hoster with an empty library and no guide has to understand what to do next.
2. **Planning has room, cooking and shopping need speed.** Dense surfaces suit a big screen. Phone surfaces need large targets, one-hand reach and glanceable text.
3. **The user decides.** Generated and assistant changes stay drafts, can be undone and are clearly labelled until the user accepts them.
4. **AI is optional.** No flow depends on a provider. Features that need one are hidden when there is no key, not shown broken.
5. **Honest data.** Incomplete nutrition is flagged and never silently shown as zero. Only verified links are shown.

## Accessibility & Inclusion

- WCAG 2.2 AA: contrast, visible focus, targets of at least 24 px (44 px on touch), and colour is never the only signal (badges always carry text).
- The plan board is a real grid with full accessible names per cell. Live regions announce assistant changes and generation progress.
- Text scales to 200 %. `prefers-reduced-motion` and `prefers-color-scheme` are honoured, with a theme override.
- English and German. German strings run longer, so layouts must allow for that.
