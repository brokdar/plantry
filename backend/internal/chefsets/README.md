# chefsets: Chef plate and attribute test sets

Hand-labelled test data for the Chef feature (spec section 3 attributes, section 5.3 plate fit). No production
code uses it; the attribute derivation (A1), the plate-fit engine (A4a) and the labelling canary (A3) load it.

## What is in a split

Each split is a directory with two JSON files (format below). Two splits exist:

- `dev/` is committed and embedded (`LoadDev`): 76 recipes, 11 leaves, 61 plates. Tune against it.
- **sealed** is never committed. It lives outside the repo; the orchestrator sets
  `PLANTRY_CHEF_SETS_SEALED=<dir>` and `LoadSealed` merges it over dev (sealed plates may use dev foods): 38
  more recipes (114 in all), 2 leaves, 57 plates. `TestSealedSet` runs only when the variable is set.

The recipes are the 30 of the eval library (`internal/eval/library.json`, labelled in `labels`) plus new ones
(`foods`), all over the library's German-named ingredients plus extra ones in `ingredients` (per 100 g).

## Format

`data.json`: `ingredients` (eval.Ingredient), `labels` (`{name, attrs}` for eval-library recipes), `foods`
(eval.Recipe fields plus `attrs`), `leaves` (garnishes, role `garnish`, `attrs` limited to taste, texture,
colour, fresh).

`attrs`: `cuisine` (PLATE-3 list or `neutral`), `kind`, `heroes` (1-3 ingredients of the recipe), `texture`,
`temperature`, `saucy`, `richness` (0-3), `colour`, `taste` (salt, acid, sweet, bitter, umami, heat; 0-3, zeros
omitted), `fresh` (a raw, green or citrus element), `has_starch`, `technique`, `format`. Vocabularies are the
exported slices in `chefsets.go`.

`plates.json`: `id`, `kind` (`good`, `flawed`, `classic`), `cuisine` (that of the main), `components` (food names,
the main first), `note`, `flaws` (flawed plates only: `dimension` of the ten PLATE-1 dimensions, `type` gap or
clash, `expect` in words).

## Derivable versus judgment (ATTR-4)

`DerivableFields` can be computed from ingredients and grams: `heroes`, `richness`, `saucy`, `colour`, `fresh`,
`has_starch`. `JudgmentFields` need a model or the user: `cuisine` (derivable only where a food carries a cuisine
tag in `tags`), `kind`, `texture`, `temperature`, `taste`, `technique`, `format`. The labels of derivable fields
are culinary judgment, checked loosely: heroes are ingredients, richness is within one step of the fat share of
energy (`TestRichnessFollowsFatShare`; under 150 kcal a portion counts as light, capped at 1). A1 states its own
tolerance against them.

## Consistency the tests enforce

`Validate`: vocabularies, heroes are ingredients, every ingredient and plate component exists, first component
is a main, good and classic plates carry no flaws, flawed plates carry at least one. Coverage: all ten
dimensions have a flawed plate, every PLATE-3 cuisine has a classic plate.

The label predicates in `chefsets_test.go` turn each dimension into a mechanical reading of the labels (for
example `taste_balance`: a component of richness 2 or more and no acid anywhere). They are this set's
convention, not the plate-fit engine. Rules: a flawed plate must show every dimension it is tagged with; a
classic plate must show none of the clash or strong gaps (cuisine, proportions, richness, sauce-starch, taste
balance, pairing); a good plate must show none of the ten. Coherence groups: Italian, Mediterranean and French
mix; German, Indian, Chinese, Japanese, Thai, Mexican and Middle Eastern each stand alone; `neutral` mixes
with everything. Richness counts main and starch components only (a dip or dressing does not).
Pairing style is checked for the contrast cuisines (Indian, Chinese, Japanese, Thai, Mexican, Middle
Eastern) only: no cold component and no acid of 2 or more. The set does not label
bridge-cuisine pairing flaws; A4a's own tests cover them.

## Matching sets (DATA-2)

Test data of the Chef's ingredient matching (B1), in `matching/`, loaded by `LoadMatching`:

- `de.json`: the 72 real ingredient lines of the repo's test pages (Chefkoch, Kitchen Stories, Eat This; three
  Kitchen Stories lines are English). `TestGermanSetIsTheRepoTestPages` re-extracts them from the pages.
- `en.json`: 90 real lines of 7 English recipes (BBC Good Food, Just One Cookbook, RecipeTin Eats); lines only,
  with the source URL, no page HTML.
- A line has `kind` (plain, alternatives, compound, to_taste, vague_count, container_range, branded), the
  cleaned `name`, and `expect`: the foods it must resolve to (all for a compound line, any one for `any_of`,
  none for a sub-recipe reference or water). An expected food is a leaf of the dev catalogue (`catalogue`), an FDC
  record (`fdc_query` plus `fdc_id`) or an Open Food Facts product (`off_query`, `off_lang`, `off_match`);
  `grams` is the expected weight of the whole line where one exists.
- `catalogue.json`: the 136 leaves of the dev catalogue (2026-11-27); `seeded_2026_11_16` marks the 106 of the
  seeded backup the research baseline used.
- `fdc.json`, `off.json`: search responses by query, each with `origin`. FDC `live` responses were recorded with
  DEMO_KEY (Foundation and SR Legacy, five results); `bulk` ones are built from FDC's public SR Legacy download
  (2018-04) in the search-response shape with a plain word-prefix ranking, so their order is not FDC's. OFF
  responses are live, from `cgi/search.pl` (see the PR notes: the v2 search endpoint ignores `search_terms`).
  `serving_quantity` and the product quantity fields are dropped (OFF returns them as strings, which the OFF
  client cannot decode).

`TestBaselineLocalMatchRates` pins the no-key rate of today's resolver (line parser, then local catalogue) so B1
can show it beats it.
