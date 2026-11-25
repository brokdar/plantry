-- +goose Up
-- +goose StatementBegin
-- Chef tips (TIP): one row per tip shown on a plate. plate_id and recipe_id
-- (the plate's main, the cooldown key) are kept as NULL when the plate or
-- food goes, so Insights still count the tip. change is the checked change
-- as JSON (domain/tips.Op); state moves shown -> applied | dismissed |
-- withdrawn (the plate no longer needs it), and applied -> shown on Undo.
CREATE TABLE chef_tips (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    plate_id     INTEGER REFERENCES plates(id) ON DELETE SET NULL,
    recipe_id    INTEGER REFERENCES foods(id) ON DELETE SET NULL,
    window_start TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    dimension    TEXT    NOT NULL,
    change       TEXT    NOT NULL,
    state        TEXT    NOT NULL DEFAULT 'shown'
        CHECK (state IN ('shown', 'applied', 'dismissed', 'withdrawn')),
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);
CREATE INDEX idx_chef_tips_window ON chef_tips (window_start);
CREATE INDEX idx_chef_tips_state ON chef_tips (state, updated_at);
-- +goose StatementEnd

-- +goose Down
DROP TABLE chef_tips;
