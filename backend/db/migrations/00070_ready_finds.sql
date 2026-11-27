-- +goose Up
-- Chef C3: a ready find the planner placed records the window it went into
-- (PRO-5 "What the assistant did", Undo per entry). The planning session
-- (P11, FIND-1) is one row: the time it stays open until.
ALTER TABLE recipe_suggestions ADD COLUMN placed_on TEXT;
CREATE TABLE chef_planning_session (
    id    INTEGER PRIMARY KEY CHECK (id = 1),
    until TEXT    NOT NULL
);

-- +goose Down
DROP TABLE chef_planning_session;
ALTER TABLE recipe_suggestions DROP COLUMN placed_on;
