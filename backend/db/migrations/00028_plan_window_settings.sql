-- +goose Up
-- +goose StatementBegin
-- Phase 2 D24: windows always start on the shopping day. plan.week_starts_on
-- is retired and the fixed_weekday anchor becomes next_shopping_day.
DELETE FROM app_settings WHERE key = 'plan.week_starts_on';
UPDATE app_settings SET value = 'next_shopping_day' WHERE key = 'plan.anchor' AND value = 'fixed_weekday';
-- +goose StatementEnd

-- +goose Down
-- Nothing to undo: the deleted plan.week_starts_on row and the rewritten
-- fixed_weekday anchor are not restored.
