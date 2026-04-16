-- name: GetProfile :one
SELECT * FROM user_profile WHERE id = 1;

-- name: UpsertProfile :one
UPDATE user_profile
SET kcal_target           = ?,
    protein_pct           = ?,
    fat_pct               = ?,
    carbs_pct             = ?,
    dietary_restrictions  = ?,
    preferences           = ?,
    system_prompt         = ?,
    locale                = ?,
    updated_at            = datetime('now')
WHERE id = 1
RETURNING *;
