# phase2.db

A real Phase 2-era Plantry database (goose version 33), the schema every
install had at the end of Phase 2. `migrate_phase2_test.go` copies it,
migrates the copy to head and checks the data survived.

- **Built from:** commit `c0fd395` ("Phase 2 frontend review A", #61), run
  from a temporary git worktree with `go run ./cmd/plantry`
  (`PLANTRY_AI_PROVIDER=fake`, `PLANTRY_OFF_DISABLED=1`).
- **Seeded over its HTTP API** (relative to the current week, so dates are
  from 2026-11): 3 slots; 8 leaf ingredients; 12 composed recipes with
  instructions and tags; 20 plates (one cooking session each) over last and
  this week, last week's marked cooked; 2 routines (presets); availability
  defaults plus one "none" override; a profile; setup completed; one chat
  conversation (fake provider).
- **Contents at rest:** counts are asserted in `migrate_phase2_test.go`;
  there is no secret in it (no API key, no `secret.key`).
- **Rebuild:** check out `c0fd395` in a worktree, start the server with the
  env above and a fresh `PLANTRY_DB_PATH`, run the seed script (a Python
  script posting to `/api/settings/slots`, `/api/foods`, `/api/plates`,
  `/api/cook-sessions/{id}/cooked`, `/api/presets`, `/api/availability/*`,
  `/api/profile`, `/api/setup/complete`, `/api/ai/chat`), stop the server and run
  `PRAGMA wal_checkpoint(TRUNCATE); PRAGMA journal_mode=delete;` on the file.
  Never regenerate it from a newer commit: its point is the old schema.
