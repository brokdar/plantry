-- name: ListPlanLight :many
SELECT date, scope, slot, level, target_kcal FROM plan_light
WHERE date >= ? AND date <= ?
ORDER BY date, scope, slot;

-- name: DeletePlanLightDate :exec
DELETE FROM plan_light WHERE date = ?;

-- name: DeletePlanLightBefore :exec
DELETE FROM plan_light WHERE date < ?;

-- name: CreatePlanLight :exec
INSERT OR IGNORE INTO plan_light (date, scope, slot, level, target_kcal) VALUES (?, ?, ?, ?, ?);
