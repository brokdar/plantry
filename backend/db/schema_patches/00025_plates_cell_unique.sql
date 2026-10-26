-- Schema patch: informs sqlc that plates.skipped was dropped and
-- (date, slot_id) made unique in Go migration 00025.
-- This file is read by sqlc only; it is NOT a goose migration.
ALTER TABLE plates DROP COLUMN skipped;
CREATE UNIQUE INDEX ux_plates_date_slot ON plates(date, slot_id);
