-- name: InsertUser :one
INSERT INTO users (username, full_name, password_hash, role, email, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, username, full_name, role, is_active, password_changed_at;

-- name: GetUserByUsername :one
SELECT id, username, full_name, password_hash, role, is_active, password_changed_at, locked_until
FROM users WHERE username = $1;

-- name: GetUserByID :one
SELECT id, username, full_name, password_hash, role, is_active, password_changed_at
FROM users WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $2, password_changed_at = now(), updated_at = now(), updated_by = $3,
    failed_logins = 0, locked_until = NULL
WHERE id = $1;

-- name: ListActiveUsers :many
SELECT id, username, full_name, role FROM users WHERE is_active ORDER BY full_name, id;

-- name: ListTeam :many
-- Every user, active or not, with the number of active cases assigned to them.
SELECT u.id, u.username, u.full_name, u.role, COALESCE(u.email, '')::text AS email, u.is_active,
       (SELECT count(*) FROM cases c WHERE c.assignee_id = u.id AND c.status = 'active')::int AS open_cases
FROM users u
ORDER BY u.is_active DESC, u.full_name, u.id;

-- name: RecordLoginFailure :one
-- One more wrong password. The 5th in a row locks the user until lock_until and
-- starts the count again, so after the lock ends there are 5 more tries.
UPDATE users
SET failed_logins = CASE WHEN failed_logins + 1 >= sqlc.arg(max_failures)::int THEN 0 ELSE failed_logins + 1 END,
    locked_until  = CASE WHEN failed_logins + 1 >= sqlc.arg(max_failures)::int THEN sqlc.arg(lock_until)::timestamptz ELSE locked_until END
WHERE id = sqlc.arg(id)
RETURNING locked_until;

-- name: ClearLoginFailures :exec
UPDATE users SET failed_logins = 0, locked_until = NULL
WHERE id = $1 AND (failed_logins <> 0 OR locked_until IS NOT NULL);
