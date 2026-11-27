-- name: LockCase :one
SELECT id, status FROM cases WHERE id = $1 FOR UPDATE;

-- name: ActiveUser :one
SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_active);

-- name: NextNumber :one
SELECT (COALESCE(max(number), 0) + 1)::int FROM case_queries WHERE case_id = $1;

-- name: InsertQuery :one
INSERT INTO case_queries (case_id, number, body, raised_on, due_on, assignee_id, created_by, updated_by)
VALUES ($1, $2, $3, $4, sqlc.narg('due_on'), $5, $6, $6)
RETURNING id;

-- name: QueryCaseID :one
SELECT case_id FROM case_queries WHERE id = $1;

-- name: LockQuery :one
SELECT id, case_id, number, status, body, raised_on, due_on, assignee_id, (removed_at IS NOT NULL)::boolean AS removed
FROM case_queries WHERE id = $1 FOR UPDATE;

-- name: UpdateQuery :exec
UPDATE case_queries
SET body = $2, raised_on = $3, due_on = sqlc.narg('due_on'), assignee_id = $4, updated_at = now(), updated_by = $5
WHERE id = $1;

-- name: SetQueryStatus :exec
UPDATE case_queries SET status = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: RemoveQuery :exec
UPDATE case_queries SET removed_at = now(), removed_by = $2, updated_at = now(), updated_by = $2 WHERE id = $1;

-- name: InsertEvent :exec
INSERT INTO case_query_events (query_id, kind, note, document_id, file_name, by_user)
VALUES ($1, $2, sqlc.narg('note'), sqlc.narg('document_id'), sqlc.narg('file_name'), $3);

-- name: LatestSentDocument :one
-- The proof file of the query's latest "sent" event, if it still exists.
SELECT e.document_id FROM case_query_events e
WHERE e.query_id = $1 AND e.kind = 'sent'
ORDER BY e.at DESC, e.id DESC LIMIT 1;

-- name: ListQueries :many
-- Live queries of a case: unresolved first, each group by number.
SELECT q.id, q.case_id, q.number, q.body, q.raised_on, q.due_on, q.assignee_id, u.full_name AS assignee_name, q.status
FROM case_queries q
JOIN users u ON u.id = q.assignee_id
WHERE q.case_id = $1 AND q.removed_at IS NULL
ORDER BY (q.status = 'resolved'), q.number;

-- name: ListEvents :many
-- Every event of every query of a case, removed queries included (for the case history).
SELECT e.id, e.query_id, q.number, q.body, e.kind, COALESCE(e.note, '')::text AS note, e.document_id,
       COALESCE(d.label, '')::text AS document_label, COALESCE(d.file_name, e.file_name, '')::text AS file_name,
       (e.file_name IS NOT NULL AND d.id IS NULL)::boolean AS file_deleted,
       e.at, u.full_name AS by_name
FROM case_query_events e
JOIN case_queries q ON q.id = e.query_id
JOIN users u ON u.id = e.by_user
LEFT JOIN documents d ON d.id = e.document_id
WHERE q.case_id = $1
ORDER BY e.at, e.id;

-- name: CaseExists :one
SELECT EXISTS (SELECT 1 FROM cases WHERE id = $1);

-- name: OpenCount :one
SELECT count(*)::int FROM case_queries
WHERE case_id = $1 AND removed_at IS NULL AND status <> 'resolved';
