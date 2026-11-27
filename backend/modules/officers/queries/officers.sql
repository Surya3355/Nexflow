-- name: ListOfficers :many
-- Current officers, by bank name, the team's stage order, then name.
SELECT o.id, o.bank_id, b.name AS bank_name, o.team, o.name, COALESCE(o.phone, '')::text AS phone, COALESCE(o.email, '')::text AS email
FROM bank_officers o
JOIN banks b ON b.id = o.bank_id
WHERE o.removed_at IS NULL
ORDER BY b.name, b.id,
  COALESCE((SELECT s.sort_order FROM stages s WHERE s.bank_id = o.bank_id AND s.officer_team = o.team),
           (SELECT s.sort_order FROM stages s WHERE s.bank_id IS NULL AND s.officer_team = o.team), 0),
  o.name, o.id;

-- name: GetOfficer :one
SELECT o.id, o.bank_id, b.name AS bank_name, o.team, o.name, COALESCE(o.phone, '')::text AS phone, COALESCE(o.email, '')::text AS email, (o.removed_at IS NOT NULL)::boolean AS removed
FROM bank_officers o JOIN banks b ON b.id = o.bank_id
WHERE o.id = $1;

-- name: InsertOfficer :one
INSERT INTO bank_officers (bank_id, team, name, phone, email, created_by, updated_by)
VALUES ($1, $2, $3, sqlc.narg('phone'), sqlc.narg('email'), $4, $4)
RETURNING id;

-- name: UpdateOfficer :execrows
UPDATE bank_officers SET name = $2, phone = sqlc.narg('phone'), email = sqlc.narg('email'), updated_at = now(), updated_by = $3
WHERE id = $1 AND removed_at IS NULL;

-- name: RemoveOfficer :execrows
UPDATE bank_officers SET removed_at = now(), updated_at = now(), updated_by = $2
WHERE id = $1 AND removed_at IS NULL;

-- name: LockOfficer :one
SELECT name, COALESCE(phone, '')::text AS phone, COALESCE(email, '')::text AS email FROM bank_officers WHERE id = $1 AND removed_at IS NULL FOR UPDATE;
