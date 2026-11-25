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
    diet_type             = ?,
    meat_days_per_week_max = ?,
    household_portions    = ?,
    goal                  = ?,
    weight_kg             = ?,
    protein_g_per_kg      = ?,
    height_cm             = ?,
    birth_year            = ?,
    sex                   = ?,
    activity_level        = ?,
    updated_at            = datetime('now')
WHERE id = 1
RETURNING *;

-- name: UpdateKitchen :one
UPDATE user_profile
SET minutes_weekday = ?,
    minutes_weekend = ?,
    skill           = ?,
    equipment       = ?,
    heat            = ?,
    leanings        = ?,
    updated_at      = datetime('now')
WHERE id = 1
RETURNING *;
