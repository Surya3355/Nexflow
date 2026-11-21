-- name: ListBanks :many
SELECT b.id, b.name, b.is_active,
       EXISTS (SELECT 1 FROM stages s WHERE s.bank_id = b.id AND s.is_active)::boolean AS has_own_stages
FROM banks b WHERE b.is_active ORDER BY b.name;

-- name: RenameBank :one
UPDATE banks SET name = $2, updated_at = now(), updated_by = $3
WHERE id = $1
RETURNING id, name, is_active;

-- name: InsertBank :one
INSERT INTO banks (name, created_by, updated_by)
VALUES ($1, $2, $2)
RETURNING id, name, is_active;

-- name: ListStandardStages :many
SELECT id, bank_id, code, name, sort_order, requires_document
FROM stages WHERE bank_id IS NULL AND is_active ORDER BY sort_order;

-- name: ListStagesForBank :many
SELECT id, bank_id, code, name, sort_order, requires_document
FROM stages WHERE bank_id = $1 AND is_active ORDER BY sort_order;

-- name: InsertStage :one
INSERT INTO stages (bank_id, code, name, sort_order, requires_document, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING id, bank_id, code, name, sort_order, requires_document;

-- name: SetStageRequiresDocument :one
UPDATE stages SET requires_document = $2, updated_at = now(), updated_by = $3
WHERE id = $1
RETURNING id, bank_id, code, name, sort_order, requires_document;

-- name: GetStage :one
SELECT id, bank_id, code, name, sort_order, requires_document FROM stages WHERE id = $1;

-- name: ListDropdownValues :many
SELECT id, category, value, sort_order FROM dropdown_values
WHERE category = $1 AND is_active ORDER BY sort_order, id;

-- name: InsertDropdownValue :one
INSERT INTO dropdown_values (category, value, sort_order, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
RETURNING id, category, value, sort_order;

-- name: ListProjects :many
SELECT p.id, p.name, p.is_system,
       COUNT(c.id) FILTER (WHERE c.status = 'active')::bigint AS open_cases,
       COUNT(c.id)::bigint AS total_cases
FROM projects p LEFT JOIN cases c ON c.project_id = p.id
GROUP BY p.id
ORDER BY p.is_system DESC, lower(p.name), p.id;

-- name: InsertProject :one
INSERT INTO projects (name, created_by, updated_by)
VALUES ($1, $2, $2)
RETURNING id, name, is_system;

-- name: GetProject :one
SELECT id, name, is_system FROM projects WHERE id = $1;

-- name: RenameProject :one
UPDATE projects SET name = $2, updated_at = now(), updated_by = $3
WHERE id = $1 AND NOT is_system
RETURNING id, name, is_system;

-- name: ProjectExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)::boolean;
