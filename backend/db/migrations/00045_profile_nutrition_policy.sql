-- +goose Up
-- Nutrition policy: the goal mode and the optional body data behind the
-- kcal-target estimate. Existing rows keep working: goal defaults to
-- maintain, the body fields stay empty.
ALTER TABLE user_profile ADD COLUMN goal TEXT NOT NULL DEFAULT 'maintain'
    CHECK (goal IN ('maintain', 'lose', 'gain'));
ALTER TABLE user_profile ADD COLUMN weight_kg REAL CHECK (weight_kg IS NULL OR weight_kg > 0);
ALTER TABLE user_profile ADD COLUMN height_cm REAL CHECK (height_cm IS NULL OR height_cm > 0);
ALTER TABLE user_profile ADD COLUMN birth_year INTEGER;
ALTER TABLE user_profile ADD COLUMN sex TEXT CHECK (sex IS NULL OR sex IN ('female', 'male'));
ALTER TABLE user_profile ADD COLUMN activity_level REAL CHECK (activity_level IS NULL OR activity_level IN (1.4, 1.6, 1.8));

-- +goose Down
ALTER TABLE user_profile DROP COLUMN activity_level;
ALTER TABLE user_profile DROP COLUMN sex;
ALTER TABLE user_profile DROP COLUMN birth_year;
ALTER TABLE user_profile DROP COLUMN height_cm;
ALTER TABLE user_profile DROP COLUMN weight_kg;
ALTER TABLE user_profile DROP COLUMN goal;
