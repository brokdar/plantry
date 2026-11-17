-- +goose Up
-- Optional protein factor (g per kg body weight) overriding the goal's
-- default; the planner needs weight_kg as well to use it.
ALTER TABLE user_profile ADD COLUMN protein_g_per_kg REAL
    CHECK (protein_g_per_kg IS NULL OR (protein_g_per_kg >= 0.8 AND protein_g_per_kg <= 2.2));

-- +goose Down
ALTER TABLE user_profile DROP COLUMN protein_g_per_kg;
