-- +goose Up
CREATE TABLE user_profile (
    id                    INTEGER PRIMARY KEY CHECK (id = 1),
    kcal_target           REAL,
    protein_pct           REAL,
    fat_pct               REAL,
    carbs_pct             REAL,
    dietary_restrictions  TEXT NOT NULL DEFAULT '[]',
    preferences           TEXT NOT NULL DEFAULT '{}',
    system_prompt         TEXT,
    locale                TEXT NOT NULL DEFAULT 'en',
    updated_at            TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO user_profile (id) VALUES (1);

-- +goose Down
DROP TABLE user_profile;
