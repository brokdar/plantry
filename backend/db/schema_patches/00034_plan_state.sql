-- Schema patch: informs sqlc of the columns Go migration 00034 adds (Phase 3
-- D1). The migration is Go only because sqlc reads cook_sessions from
-- schema_patches/00027, after db/migrations.
-- This file is read by sqlc only; it is NOT a goose migration.
ALTER TABLE plates ADD COLUMN state TEXT NOT NULL DEFAULT 'confirmed' CHECK (state IN ('draft','confirmed'));
ALTER TABLE plates ADD COLUMN ai_run_id INTEGER REFERENCES ai_runs(id) ON DELETE SET NULL;
ALTER TABLE plates ADD COLUMN reason TEXT;

ALTER TABLE cook_sessions ADD COLUMN state TEXT NOT NULL DEFAULT 'confirmed' CHECK (state IN ('draft','confirmed'));
ALTER TABLE cook_sessions ADD COLUMN ai_run_id INTEGER REFERENCES ai_runs(id) ON DELETE SET NULL;
ALTER TABLE cook_sessions ADD COLUMN reason TEXT;
