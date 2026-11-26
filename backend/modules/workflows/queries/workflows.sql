-- name: ListBanks :many
SELECT b.id, b.name, b.is_active, b.part_payments,
       EXISTS (SELECT 1 FROM stages s WHERE s.bank_id = b.id AND s.is_active)::boolean AS has_own_stages
FROM banks b WHERE b.is_active ORDER BY b.name;

-- name: RenameBank :one
UPDATE banks SET name = $2, updated_at = now(), updated_by = $3
WHERE id = $1
RETURNING id, name, is_active, part_payments;

-- name: InsertBank :one
INSERT INTO banks (name, created_by, updated_by)
VALUES ($1, $2, $2)
RETURNING id, name, is_active, part_payments;

-- name: ListStandardStages :many
SELECT id, bank_id, code, name, sort_order, requires_document, COALESCE(role, '')::text AS role
FROM stages WHERE bank_id IS NULL AND is_active ORDER BY sort_order;

-- name: ListStagesForBank :many
SELECT id, bank_id, code, name, sort_order, requires_document, COALESCE(role, '')::text AS role
FROM stages WHERE bank_id = $1 AND is_active ORDER BY sort_order;

-- name: InsertStage :one
INSERT INTO stages (bank_id, code, name, sort_order, requires_document, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6)
RETURNING id, bank_id, code, name, sort_order, requires_document, COALESCE(role, '')::text AS role;

-- name: SetStageRequiresDocument :one
UPDATE stages SET requires_document = $2, updated_at = now(), updated_by = $3
WHERE id = $1
RETURNING id, bank_id, code, name, sort_order, requires_document, COALESCE(role, '')::text AS role;

-- name: GetStage :one
SELECT id, bank_id, code, name, sort_order, requires_document, COALESCE(role, '')::text AS role FROM stages WHERE id = $1;

-- name: ListDropdownValues :many
SELECT id, category, value, sort_order FROM dropdown_values
WHERE category = $1 AND is_active ORDER BY sort_order, id;

-- name: InsertDropdownValue :one
INSERT INTO dropdown_values (category, value, sort_order, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
RETURNING id, category, value, sort_order;

-- name: ListProjects :many
SELECT p.id, p.name, p.is_system, p.type, p.address,
       (SELECT COUNT(*) FROM cases c WHERE c.project_id = p.id AND c.status = 'active')::bigint AS open_cases,
       (SELECT COUNT(*) FROM cases c WHERE c.project_id = p.id)::bigint AS total_cases,
       (SELECT COUNT(*) FROM project_contacts pc WHERE pc.project_id = p.id AND pc.removed_at IS NULL)::bigint AS active_contacts
FROM projects p
ORDER BY p.is_system DESC, lower(p.name), p.id;

-- name: InsertProject :one
INSERT INTO projects (name, type, address, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
RETURNING id, name, is_system, type, address;

-- name: GetProject :one
SELECT id, name, is_system, type, address FROM projects WHERE id = $1;

-- name: LockProject :one
SELECT id, name, is_system, type, address FROM projects WHERE id = $1 FOR UPDATE;

-- name: UpdateProject :one
UPDATE projects SET name = $2, type = $3, address = $4, updated_at = now(), updated_by = $5
WHERE id = $1 AND NOT is_system
RETURNING id, name, is_system, type, address;

-- name: ListActiveProjectContacts :many
SELECT id, name, phone, email, label, removed_at FROM project_contacts
WHERE project_id = $1 AND removed_at IS NULL ORDER BY id;

-- name: InsertProjectContact :exec
INSERT INTO project_contacts (project_id, name, phone, email, label, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $6);

-- name: UpdateProjectContact :exec
UPDATE project_contacts SET name = $3, phone = $4, email = $5, label = $6, updated_at = now(), updated_by = $7
WHERE id = $1 AND project_id = $2 AND removed_at IS NULL;

-- name: RemoveProjectContact :exec
UPDATE project_contacts SET removed_at = now(), updated_at = now(), updated_by = $3
WHERE id = $1 AND project_id = $2 AND removed_at IS NULL;

-- name: ProjectExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1)::boolean;

-- name: LockBank :one
SELECT b.id, b.part_payments,
       EXISTS (SELECT 1 FROM stages s WHERE s.bank_id = b.id)::boolean AS has_own_stages
FROM banks b WHERE b.id = $1 FOR UPDATE OF b;

-- name: InsertBankStage :one
INSERT INTO stages (bank_id, code, name, sort_order, role, created_by, updated_by)
VALUES ($1, $2, $3, $4, sqlc.narg('role'), $5, $5)
RETURNING id;

-- name: MoveBankCasesStage :exec
UPDATE cases SET current_stage_id = sqlc.arg(to_stage_id), updated_at = now(), updated_by = sqlc.arg(updated_by)
WHERE bank_id = sqlc.arg(bank_id) AND current_stage_id = sqlc.arg(from_stage_id);

-- name: SetBankPartPayments :execrows
UPDATE banks SET part_payments = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: GetBankPartPayments :one
SELECT part_payments FROM banks WHERE id = $1;

-- name: ListBanksWithOwnStages :many
SELECT b.id, b.name FROM banks b
WHERE EXISTS (SELECT 1 FROM stages s WHERE s.bank_id = b.id AND s.is_active)
ORDER BY b.name;
