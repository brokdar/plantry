# Plantry report

Generated 2026-11-28 12:00 UTC, window: last 30 days.

## Problems

- chef_help: 1 of 4 runs failed (25 %)
- chef_help: p90 is 12.0 s (over 10.0 s)
- untracked AI runs: 1 (should be 0): a model call ran without a run record
- Chef spend this month is $4.28 of the $5.00 cap (86 %)
- Chef job chef_find failed 3 times

## Build

- App version: dev
- Schema version: 76
- Database size: N MB
- chef.enabled: true (stored)
- ai.provider: (none) (default)
- ai.model: fake-model (stored)
- chef.monthly_cap: 5 (default)
- ai.run_retention_days: 180 (default)
- chef.library_review_enabled: false (default)
- chef.new_dishes_per_week: 1 (default)
- discovery.daily_enabled: true (default)
- ai.programmatic_tools: true (default)

## AI

| Kind | Runs | Errors | p50 | p90 | Cost |
|---|---:|---:|---:|---:|---:|
| chef_find | 1 | 0 (0 %) | 30.0 s | 30.0 s | $4.2000 |
| chef_help | 4 | 1 (25 %) | 2.0 s | 12.0 s | $0.0400 |
| chef_import_check | 2 | 0 (0 %) | 5.0 s | 6.0 s | $0.0400 |
| generate | 1 | 0 (0 %) | 8.0 s | 8.0 s | $0.0000 |
| regenerate | 1 | 0 (0 %) | 7.0 s | 7.0 s | $0.0000 |
| untracked | 1 | 0 (0 %) | - | - | $0.0000 |

Top errors of chef_help: error.ai.provider_error 1

Models used: fake/fake-model (10)

Untracked runs: 1

Chef spend per month (capped kinds):

- 2026-11: $4.2800 of a $5.00 cap

## Chef outcomes

- Help answers: 3 (helped 1, didn't help 1, no verdict 1; verdict rate 67 %)
- Finds by state/status: rejected 1, in_library/accepted 2
- Find reject reasons: too_long 1
- Accepted finds: 2, still in the library 2, cooked 1, rated 1
- Trial dishes: 2 (kept 1, removed 1, cooked 1)
- Tips by state: applied 1, dismissed 1
- Import checks: 2 ran, 1 ended in a saved recipe (no outcome: 1)
  - changes kept: keep 1, times 1; undone: role 1
  - finishing touch: added 1, skipped 0; flags ignored: 2
- Chef jobs by kind/status: chef_attributes/done 1, chef_find/failed 3

## Planning

- Runs: generate 1, regenerate 1; chooser heuristic 1, llm 1
- Violations in the final plans (1 evaluated): kcal_below_floor 2, too_many 1

## Ratings

| Plates | Rated | Loved | Disliked | Not eaten |
|---|---:|---:|---:|---:|
| With a Chef dish | 1 | 100 % | 0 % | 0 % |
| Others | 1 | 0 % | 100 % | 0 % |
