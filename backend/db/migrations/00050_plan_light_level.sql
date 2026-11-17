-- +goose Up
-- A light request's level: light (a lighter meal or day) or hearty (a
-- heartier meal or day). Every stored row stays a light one.
ALTER TABLE plan_light ADD COLUMN level TEXT NOT NULL DEFAULT 'light' CHECK (level IN ('light', 'hearty'));

-- +goose Down
ALTER TABLE plan_light DROP COLUMN level;
