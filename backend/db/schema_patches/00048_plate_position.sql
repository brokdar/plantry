-- Schema patch: informs sqlc of the column Go migration 00048 adds (the plate
-- component's position on its plate). Read by sqlc only; NOT a goose migration.
ALTER TABLE plate_components ADD COLUMN position TEXT NOT NULL DEFAULT 'side' CHECK (position IN ('main','side','extra'));
