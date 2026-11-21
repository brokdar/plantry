-- +goose Up
-- The energy band of a time slot: the share of the day's energy its meal should
-- hold, in percent of the day's target. Both NULL: the default band by the
-- slot's name, else by its kind. Both set otherwise.
ALTER TABLE time_slots ADD COLUMN band_min_pct REAL CHECK (band_min_pct IS NULL OR (band_min_pct >= 0 AND band_min_pct <= 100));
ALTER TABLE time_slots ADD COLUMN band_max_pct REAL CHECK (band_max_pct IS NULL OR (band_max_pct >= 0 AND band_max_pct <= 100));

-- +goose Down
ALTER TABLE time_slots DROP COLUMN band_max_pct;
ALTER TABLE time_slots DROP COLUMN band_min_pct;
