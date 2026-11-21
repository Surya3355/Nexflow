-- name: CaseRowJSON :one
SELECT row_to_json(c)::text AS doc FROM cases c WHERE c.id = $1;

-- name: CustomerRowJSON :one
SELECT row_to_json(cu)::text AS doc FROM customers cu WHERE cu.id = $1;

-- name: CaseChildrenJSON :one
SELECT jsonb_build_object(
  'stage_events', COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM case_stage_events e WHERE e.case_id = $1), '[]'::jsonb),
  'assignment_events', COALESCE((SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM case_assignment_events e WHERE e.case_id = $1), '[]'::jsonb),
  'documents', COALESCE((SELECT jsonb_agg(to_jsonb(d) ORDER BY d.id) FROM documents d WHERE d.case_id = $1), '[]'::jsonb),
  'alerts', COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM alert_log a WHERE a.case_id = $1), '[]'::jsonb)
)::text AS doc;

-- name: DeleteCaseAlerts :exec
DELETE FROM alert_log WHERE case_id = $1;

-- name: DeleteCaseDocuments :exec
DELETE FROM documents WHERE case_id = $1;

-- name: DeleteCaseStageEvents :exec
DELETE FROM case_stage_events WHERE case_id = $1;

-- name: DeleteCaseAssignmentEvents :exec
DELETE FROM case_assignment_events WHERE case_id = $1;

-- name: DeleteCaseRow :exec
DELETE FROM cases WHERE id = $1;

-- name: DeleteCustomerRow :exec
DELETE FROM customers WHERE id = $1;

-- name: CountLiveCases :one
SELECT count(*) FROM cases WHERE customer_id = $1;

-- name: ArchivedCaseIDs :many
SELECT entity_id FROM archive
WHERE entity_type = 'case' AND restored_at IS NULL
  AND (snapshot->'case'->>'customer_id')::bigint = sqlc.arg(customer_id)::bigint
ORDER BY entity_id;

-- name: InsertArchive :exec
-- archived_at and purge_after come from the same transaction clock: purge is exactly one year later.
INSERT INTO archive (entity_type, entity_id, snapshot, archived_by, archived_at, purge_after)
VALUES (sqlc.arg(entity_type), sqlc.arg(entity_id), sqlc.arg(snapshot)::jsonb, sqlc.arg(archived_by), now(), now() + interval '1 year');

-- name: ListArchive :many
SELECT id, entity_type, entity_id, archived_at, purge_after, restored_at
FROM archive ORDER BY archived_at DESC, id DESC LIMIT 500;
