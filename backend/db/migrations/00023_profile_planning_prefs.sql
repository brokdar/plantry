-- +goose Up
-- +goose StatementBegin
-- Planning preferences on the profile, retirement of plan.window_days, and
-- setup.completed for existing installs (they already have time slots, so
-- they skip the setup wizard).
ALTER TABLE user_profile ADD COLUMN diet_type TEXT NOT NULL DEFAULT 'omnivore'
    CHECK (diet_type IN ('vegan', 'vegetarian', 'pescetarian', 'omnivore'));
ALTER TABLE user_profile ADD COLUMN meat_days_per_week_max INTEGER
    CHECK (meat_days_per_week_max IS NULL OR meat_days_per_week_max BETWEEN 0 AND 7);
ALTER TABLE user_profile ADD COLUMN household_portions REAL NOT NULL DEFAULT 1.0
    CHECK (household_portions > 0);

DELETE FROM app_settings WHERE key = 'plan.window_days';

INSERT INTO app_settings (key, value)
SELECT 'setup.completed', 'true' WHERE EXISTS (SELECT 1 FROM time_slots);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- The deleted plan.window_days row is not restored.
DELETE FROM app_settings WHERE key = 'setup.completed';
ALTER TABLE user_profile DROP COLUMN household_portions;
ALTER TABLE user_profile DROP COLUMN meat_days_per_week_max;
ALTER TABLE user_profile DROP COLUMN diet_type;
-- +goose StatementEnd
