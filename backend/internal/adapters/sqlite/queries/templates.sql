-- name: CreateTemplate :one
INSERT INTO templates (name, scope) VALUES (?, ?) RETURNING *;

-- name: GetTemplate :one
SELECT * FROM templates WHERE id = ?;

-- name: UpdateTemplateName :one
UPDATE templates SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTemplate :execresult
DELETE FROM templates WHERE id = ?;

-- name: ListTemplates :many
SELECT * FROM templates ORDER BY name, id;

-- name: ListTemplatesByScope :many
SELECT * FROM templates WHERE scope = ? ORDER BY name, id;

-- name: CreateTemplateEntry :one
INSERT INTO template_entries (template_id, food_id, portions, sort_order, day_offset, slot_id, note)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteTemplateEntriesByTemplate :execresult
DELETE FROM template_entries WHERE template_id = ?;

-- name: ListTemplateEntriesByTemplate :many
SELECT * FROM template_entries WHERE template_id = ? ORDER BY sort_order, id;

-- name: CountTemplatesUsingFood :one
SELECT COUNT(*) FROM template_entries WHERE food_id = ?;
