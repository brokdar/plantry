-- name: ListWeekThemes :many
SELECT * FROM week_themes WHERE window_start = ? ORDER BY id;

-- name: InsertWeekTheme :one
INSERT INTO week_themes (window_start, kind, value, source, whole_week)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteWeekTheme :execrows
DELETE FROM week_themes WHERE id = ? AND window_start = ?;

-- name: GetThemeSuggestion :one
SELECT * FROM week_theme_suggestions WHERE window_start = ?;

-- name: InsertThemeSuggestion :exec
INSERT INTO week_theme_suggestions (window_start, kind, value, state)
VALUES (?, ?, ?, 'open')
ON CONFLICT (window_start) DO NOTHING;

-- name: DecideThemeSuggestion :execrows
UPDATE week_theme_suggestions SET state = ?, decided_at = ?
WHERE window_start = ? AND state = 'open';

-- name: RecentThemeDecisions :many
SELECT state, decided_at FROM week_theme_suggestions
WHERE decided_at IS NOT NULL
ORDER BY decided_at DESC
LIMIT 2;

-- name: RecentThemeValues :many
SELECT t.value FROM week_themes t WHERE t.window_start >= sqlc.arg(since)
UNION
SELECT s.value FROM week_theme_suggestions s WHERE s.window_start >= sqlc.arg(since);
