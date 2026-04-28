-- Schema patch: informs sqlc of the date column added to plates in Go migration 00014.
-- This file is read by sqlc only; it is NOT a goose migration.
ALTER TABLE plates ADD COLUMN date TEXT NOT NULL;
