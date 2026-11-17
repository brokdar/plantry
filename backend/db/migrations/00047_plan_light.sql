-- +goose Up
-- The user's light requests (a lighter meal, or a lighter day) per date, so
-- the board, the chat and every later run evaluate the plan under them.
-- scope meal names the slot (lunch|dinner); scope day has an empty slot.
CREATE TABLE plan_light (
    date  TEXT NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('meal', 'day')),
    slot  TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (date, scope, slot)
);

-- +goose Down
DROP TABLE plan_light;
