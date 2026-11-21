# agent testdata

- `restore_golden.json`: 14 real final replies of the a2, hh1 and ho1 rounds (what the model
  wrote, before the restore) with the typed must-say facts of their saved change. `want_added`
  was read by hand; `old_added` is what the regex restore appended then.
- `chat_start.db`: the chat start database of the a1 to e5x rounds
  (`~/.local/share/plantry-dev/loop/rounds/a1-fca42e7/start-chat.db`, identical in every round),
  slimmed by `slim_chat_start.py` (run data dropped, VACUUM) to 450 KB. It is a dump of a real
  household's plan and library, not built from migrations: it carries the schema of goose
  version 53. The replay tests open a copy of it directly. After a schema change, either keep it
  (the tests only read the tables they use) or rebuild it from a newer round start database with
  the script, then re-check the pinned numbers in `replay_e048c_test.go` (Ciabatta, 1934 kcal).
