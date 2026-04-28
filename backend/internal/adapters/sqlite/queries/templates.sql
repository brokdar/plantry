-- name: CreateTemplate :one
INSERT INTO templates (name) VALUES (?) RETURNING *;

-- name: GetTemplate :one
SELECT * FROM templates WHERE id = ?;

-- name: UpdateTemplateName :one
UPDATE templates SET name = ? WHERE id = ? RETURNING *;

-- name: DeleteTemplate :execresult
DELETE FROM templates WHERE id = ?;

-- name: ListTemplates :many
SELECT * FROM templates ORDER BY name, id;

-- name: CreateTemplateComponent :one
INSERT INTO template_components (template_id, food_id, portions, sort_order, day_offset)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: DeleteTemplateComponentsByTemplate :execresult
DELETE FROM template_components WHERE template_id = ?;

-- name: ListTemplateComponentsByTemplate :many
SELECT * FROM template_components WHERE template_id = ? ORDER BY sort_order, id;

-- name: CountTemplatesUsingFood :one
SELECT COUNT(*) FROM template_components WHERE food_id = ?;
