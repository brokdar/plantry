# Plantry — Product Definition

## Elevator Pitch

Plantry is a self-hosted weekly meal planner for home cooks who care about what they eat. You build a small library of dishes you actually cook, compose them into plates on a weekly calendar, and let an AI agent help you plan, rotate, and stay on your nutrition targets. Everything runs on your own hardware — a Raspberry Pi, NAS, or laptop — in a single ~30 MB Docker container. No cloud. No subscription. No data leaves your machine.

## The Problem It Solves

Meal planning apps today force one of three bad trade-offs:

1. **Cloud-first SaaS** (Mealime, Yummly, Paprika Cloud) — convenient but owns your data, pushes ads, paywalls features, and disappears when the company pivots.
2. **Generic calendar tools** (Notion, paper, spreadsheets) — private but dumb. No nutrition awareness, no shopping list, no learning.
3. **Dedicated local apps** (Paprika desktop, Mealie) — private but static. No AI help, no rotation insight, no household-aware planning.

Plantry splits the difference: **local-first like Mealie, intelligent like a SaaS, modelled around how people actually eat.**

## The Core Insight

Most meal planners model a planned meal as "one recipe." That's wrong. A real dinner is usually a **main + one or two sides + maybe a sauce**. A breakfast is often "oatmeal + fruit + coffee." Forcing these into single-recipe rows either explodes your library (one recipe per combination) or forces awkward per-ingredient "swap" systems nobody uses.

Plantry's core data unit is the **component** (a self-contained dish: chicken curry, basmati rice, cucumber raita), and the planning unit is the **plate** — a composition of 1..N components. Swapping the rice for naan is one click. Adding a side salad is one click. Creating a "tofu curry" variant of chicken curry is a first-class recipe linked through a variant group, not a fragile override.

This single modelling decision — **plates, not recipes** — is what makes everything else in Plantry simple.

## Target User

**Primary:** the home cook who wants control. Typically:

- Cooks most dinners at home, often with leftovers or family
- Tracks macros loosely (cutting, maintenance, bulking) or has dietary preferences
- Runs a home server / Raspberry Pi / NAS and values self-hosting
- Distrusts cloud services for personal data
- Wants the app to help, not dictate

**Secondary:**

- Fitness enthusiasts who already track macros and want a planner that respects targets
- Privacy-conscious households wanting one shared planning tool on the home network
- Developers who appreciate a clean self-hostable tool and may contribute

## Core Value Proposition

Plantry is worth using because it is the only meal planner that is simultaneously:

- **Self-hosted and private** — your data, your hardware, your rules.
- **Built around plates, not recipes** — matches how people actually eat and swap things on the fly.
- **AI-assisted without AI lock-in** — bring your own OpenAI or Anthropic key; the agent edits your plan conversationally via tool calls.
- **Macro-aware by default** — components have real nutrition, plates and weeks aggregate it, the AI respects targets.
- **Tiny and durable** — one ~30 MB Docker container, single SQLite file, pure Go binary. Runs on a Raspberry Pi. Will still run in ten years.

## Feature Map (by user goal)

### "I want to plan what I'll eat this week"

- 7-day × N-slot weekly grid (desktop) / day scroll (mobile)
- Plates composed of 1..N components with role-aware swap UI (swap rice → naan, add a side)
- Quick-add from component library with role filters
- Saved **templates** for common compositions (one-click re-plate)
- Copy previous week's plan
- Per-plate free-text notes ("less chili this time")

### "I want AI to plan or refine my week"

- Persistent chat panel per week with streaming responses
- Agent can read the component library, read your profile, read your targets, and execute planning actions (add plate, add component, swap component, remove, set portions, record preference)
- Two generation modes: **fill empty slots** or **replace the whole week**
- AI explains its picks ("I chose tofu curry for Tuesday because you hit your fat ceiling Monday")
- Conversation history persisted per week; profile preferences learned from feedback

### "I want to track nutrition"

- Kcal + protein + fat + carbs per component, per plate, per day, per week
- Daily bars against targets; week summary
- Goal presets (cut / maintain / bulk) + custom macro split
- Dietary restrictions stored on profile and respected by AI

### "I want to manage my dish library"

- Create components with role (`main`, `side_starch`, `side_veg`, `side_protein`, `sauce`, `drink`, `dessert`, `standalone`)
- Variants linked via variant group (tofu curry ↔ chicken curry ↔ chickpea curry) — browse siblings from any one
- Tags for free-form grouping
- Image upload with crop
- Prep + cook time, reference portions, instructions

### "I want ingredients to be mostly automatic"

- Barcode scan → auto-fetch from Open Food Facts → one click to add
- Name search → USDA FDC lookup with auto-selected best match
- Local catalog checked first to avoid duplicates
- Manual override always available, with image upload and portion unit definition (e.g. "1 clove garlic = 3 g")
- You still own the catalog; the app just makes it painless to build

### "I want to import recipes from the web"

- Paste URL → Plantry extracts name, portions, ingredients, instructions, times, image
- Tries structured JSON-LD first (zero AI cost), falls back to LLM when missing
- Ingredient resolver auto-matches against local + FDC + OFF; user confirms ambiguous rows
- Result is a first-class component; tag it, assign a role, done

### "I want to rotate meals and not cook the same things every week"

- Components track `last_cooked_at` and `cook_count` (updated from feedback)
- Archive view: browse past weeks
- "Haven't cooked in 3 weeks" highlights on component library
- "Most cooked" insights to spot ruts
- AI uses this data when generating plans

### "I want the AI to learn what I like"

- Mark each plate after cooking: `cooked ✓`, `skipped ✗`, `loved ❤`, `disliked ✗`, with optional note
- Feedback updates user profile preferences (simple heuristics, not ML)
- AI reads profile before every generation; its picks drift toward what you actually cook and enjoy

### "I want to know what to buy"

- Shopping list aggregates all ingredients across the week's plates
- Groups by ingredient, sums in base units (g / ml)
- Respects per-plate component portions
- One panel, always a click away

### "I want to configure the app"

- Time slots: rename, reorder, add, remove (Breakfast / Brunch / Lunch / Dinner / Snack — your call)
- API keys: OpenAI, Anthropic, USDA FDC (all optional)
- AI provider + model selection with model list fetched from provider
- Custom system prompt
- Dietary restrictions + preferences (freeform + structured)
- Locale (English / German initially)
- Optional password gate for household deployments

## Differentiators (what makes it worth switching)

1. **Plates, not recipes.** Swap a side in one click. No override tables, no variant explosions.
2. **AI agent with real tools.** Not a suggester — an editor. Chat streams changes into your plan live.
3. **Profile that learns.** Feedback loop updates preferences automatically; the AI uses them next time.
4. **Pure Go, single binary, no CGO.** Cross-compiles for ARM. Runs on a Pi. Will outlive most SaaS competitors.
5. **Macro-first but not dogmatic.** Essentials (kcal, P, F, C, fiber, sodium) tracked by default; no overwhelming micronutrient forms.
6. **Zero vendor lock-in for AI.** BYO key for OpenAI or Anthropic. No Plantry-branded API fee.

## Scope Boundaries — What Plantry Is Not

- **Not a food log.** It plans ahead. It doesn't log ad-hoc snacks or every bite.
- **Not a pantry tracker.** Without real-world sync, a software pantry is a lie. Plantry refuses to lie.
- **Not a social recipe platform.** No sharing, no feeds, no public recipes.
- **Not a grocery delivery app.** The shopping list is a list, not a checkout.
- **Not a fitness tracker.** No workouts, no body metrics, no glucose integration.
- **Not a SaaS.** No hosted option. Self-host or don't use it.
- **Not opinionated about your diet.** Vegan, keto, omnivore, Mediterranean — model what you want, the app doesn't judge.

## Design Principles

1. **Local-first, offline-tolerant.** The app must function without internet (LLM/FDC/OFF features degrade gracefully).
2. **Every action must feel fast.** SQLite + small payloads + optimistic UI. No spinners for <200 ms operations.
3. **Empty catalogs are OK.** First-run is a blank slate. The import flow and barcode lookup fill it quickly. No seeded recipes.
4. **The AI is a tool, not a feature.** Plantry must be fully usable with AI disabled. The agent is an accelerator, not a dependency.
5. **Macros over micros.** Track the six that matter; skip the twenty that don't.
6. **Tests are the contract.** Every domain rule has a unit test; every feature has an e2e test. If it's not tested, it doesn't ship.
7. **Botanical, warm, calm.** The UI uses sage green OKLch, soft naturals, no harsh blacks. Cooking is warm; the app should feel warm.

## Success Criteria (what "done" looks like)

A user with no prior Plantry exposure should be able to, in their first hour:

1. Deploy via `docker-compose up` on a Raspberry Pi or laptop.
2. Scan or search five ingredients into the catalog without typing a nutrient value.
3. Import two recipes from URLs and resolve their ingredients in under 30 seconds each.
4. Compose four plates onto next week's calendar.
5. View a shopping list and a weekly macro summary.
6. (Optional) Configure an OpenAI key and ask the agent to fill the rest of the week.
7. Cook one of the plates, mark it `loved ❤`, and see that in the archive the next week.

If the user completes this path without reading docs, Plantry has succeeded.
