# Stored-turn replay (M0)

`TestReplayStored` (`replay_stored_test.go`) runs every stored chat tool call of
the loop rounds through the current tool layer, with no model and no network.
It is the blast-radius check for a code change: replay before and after, and
read the differences.

## What it does

For each chat run DB under `<rounds>/<round>/runs/*/run.db` it copies the run's
start DB (`start-chat[-<variant>].db`, variant from the scenario files), brings
it up to the current migrations, then replays each chat turn after the start
DB's own history, in order, in one conversation. The tool calls a model made
directly (`apply_changes`, `find_foods`, `get_plan`, `find_alternatives`,
`find_ways`, `undo_last_change`) run through `ToolSet.Execute` with the stored
arguments. The result is compared with the stored `result_json` field by field
(`change_id` is dropped, it is recorded after the stored result). Per call:
`identical`, `differs` (with the differing paths and the first diffs) or
`skipped` (`regenerate` needs the planner model). Calls after a skipped
`regenerate` see another plan and carry a note.

Not replayed: generation runs (planner, needs a model), `plan.*` steps (recorded
planner and before/after state, not tool calls).

## Run

```bash
cd backend
PLANTRY_REPLAY_ROUNDS=$HOME/.local/share/plantry-dev/loop/rounds \
PLANTRY_REPLAY_OUT=/tmp/replay.json \
go test ./internal/domain/agent -run TestReplayStored -parallel 8 -v
```

`PLANTRY_REPLAY_FULL=1` also puts each call's arguments and replayed result into the report; `python3 -I ~/.local/share/plantry-dev/loop/tools/census.py --replay /tmp/replay.json` then counts the defect classes of what the current code returns (classes e1 and e2 need planner runs and are not in it). The replay does not know the user's message, so a dish the user named (SF3) is not marked named in it.

Without `PLANTRY_REPLAY_ROUNDS` the test skips, so the normal gate is unaffected.
`PLANTRY_REPLAY_NAMES="ho1-0b8ca4c hh1-0b8ca4c"` limits the rounds (default
a1-fca42e7, a2-0b8ca4c, ho1-0b8ca4c, hh1-0b8ca4c). About 3 minutes for the four
rounds (611 calls, 8 parallel). The test logs the counts and the differing
paths per tool. Explain a report with
`python3 -I ~/.local/share/plantry-dev/loop/tools/replay_summary.py /tmp/replay.json`.

## M0 baseline (HEAD 83ea5cf)

Run on the unchanged baseline commit 0b8ca4c, rounds a2, ho1 and hh1 replay as
identical (a2: 254 of 257 calls, ho1/hh1: 80 of 82), apart from (1) one
nondeterministic line, `report.tell[11]` of the "cooked on X instead of Mon"
form: two sessions of one dish give a map-ordered pick (move-salmon-r1,
undo-move-r1, undo-move-r3), and (2) two calls in ho-replan-wed-nomeat-r1 that
follow the skipped `regenerate`. Every difference at HEAD is explained by a
commit that changed the output on purpose (round a1 ran on fca42e7, before
E050 to E052, so it also shows those):

| Category | Commit |
|---|---|
| `report.must` appears | df007da (AF-P 1) |
| `report.tell` deduped, cooking line reworded | df007da, 1d0ad24 (E051) |
| one recommended option on under-range writes, repeated dish offers | def55c9 (E050, a1 only) |
| closing options, refused writes with a checked alternative or an honest none, lower_day levers | b929b2b (AF-P 2) |
| calls after a skipped `regenerate` | harness limit |
