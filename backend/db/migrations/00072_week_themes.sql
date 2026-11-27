-- +goose Up
-- Chef D3: week themes (THEME-2..5). A theme belongs to one planning window
-- (window_start, a shopping day) and ends with it; at most two per window
-- (enforced in Go). whole_week is set only when the user asked for the whole
-- week. One theme suggestion per window at most (THEME-4).
CREATE TABLE week_themes (
    id           INTEGER PRIMARY KEY,
    window_start TEXT    NOT NULL,
    kind         TEXT    NOT NULL CHECK (kind IN ('cuisine', 'ingredient', 'mood')),
    value        TEXT    NOT NULL,
    source       TEXT    NOT NULL CHECK (source IN ('user', 'chat', 'suggestion', 'again')),
    whole_week   INTEGER NOT NULL DEFAULT 0 CHECK (whole_week IN (0, 1)),
    UNIQUE (window_start, kind, value)
);
CREATE TABLE week_theme_suggestions (
    window_start TEXT PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('cuisine', 'ingredient', 'mood')),
    value        TEXT NOT NULL,
    state        TEXT NOT NULL CHECK (state IN ('open', 'accepted', 'declined')),
    decided_at   TEXT
);
-- THEME-6: New dishes per week replaces the explore quota, value kept.
UPDATE app_settings SET key = 'chef.new_dishes_per_week' WHERE key = 'plan.explore_per_week';

-- +goose Down
UPDATE app_settings SET key = 'plan.explore_per_week' WHERE key = 'chef.new_dishes_per_week';
DROP TABLE week_theme_suggestions;
DROP TABLE week_themes;
