-- +goose Up
-- +goose StatementBegin
-- Cell availability (Phase 1, D7). A cell is (date, slot). Its mode resolves
-- as override > weekday default > 'plan'. A 'plan' default is never stored:
-- no row means 'plan'. preset_id is required iff mode = 'routine', and
-- deleting the preset drops the row so the cell falls back a level.
CREATE TABLE availability_defaults (
    weekday   INTEGER NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0 = Monday
    slot_id   INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE CASCADE,
    mode      TEXT NOT NULL CHECK (mode IN ('plan','routine','none')),
    preset_id INTEGER REFERENCES presets(id) ON DELETE CASCADE,
    CHECK ((mode = 'routine') = (preset_id IS NOT NULL)),
    PRIMARY KEY (weekday, slot_id)
);

CREATE TABLE availability_overrides (
    date       TEXT NOT NULL, -- YYYY-MM-DD
    slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE CASCADE,
    mode       TEXT NOT NULL CHECK (mode IN ('plan','routine','none')),
    preset_id  INTEGER REFERENCES presets(id) ON DELETE CASCADE,
    note       TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK ((mode = 'routine') = (preset_id IS NOT NULL)),
    PRIMARY KEY (date, slot_id)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE availability_overrides;
DROP TABLE availability_defaults;
-- +goose StatementEnd
