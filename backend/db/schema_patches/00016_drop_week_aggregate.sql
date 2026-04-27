-- Schema patch: informs sqlc that week_id and day were removed from plates
-- and the weeks table was dropped in Go migration 00016.
-- This file is read by sqlc only; it is NOT a goose migration.
ALTER TABLE plates DROP COLUMN week_id;
ALTER TABLE plates DROP COLUMN day;
DROP TABLE IF EXISTS weeks;
