-- name: InsertCase :one
INSERT INTO cases (customer_id, bank_id, loan_amount, current_stage_id, assignee_id, created_by, updated_by, project_id,
                   tracking_number, lead_id, unit_number, builder_contact_id)
VALUES ($1, $2, $3, $4, $5, $6, $6, $7, $8, $9, $10, $11)
RETURNING id;

-- name: InsertCaseStageEvent :exec
INSERT INTO case_stage_events (case_id, stage_id, reached_at, changed_by)
VALUES ($1, $2, $3, $4);

-- name: InsertCaseAssignmentEvent :exec
INSERT INTO case_assignment_events (case_id, from_user_id, to_user_id, changed_by, changed_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetCase :one
SELECT id, customer_id, bank_id, loan_amount, tracking_number, current_stage_id, assignee_id, status, created_at, follow_up_date, project_id,
       lead_id, unit_number, builder_contact_id
FROM cases WHERE id = $1;

-- name: LockCase :one
SELECT id, customer_id, bank_id, current_stage_id, status, tracking_number, assignee_id, project_id, builder_contact_id
FROM cases WHERE id = $1 FOR UPDATE;

-- name: GetUserActive :one
SELECT is_active FROM users WHERE id = $1;

-- name: SetCaseAssignee :exec
UPDATE cases SET assignee_id = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: InsertCaseAssignmentEventAt :exec
INSERT INTO case_assignment_events (case_id, from_user_id, to_user_id, changed_by, changed_at)
VALUES ($1, $2, $3, $4, $5);

-- name: SetCaseStage :exec
UPDATE cases
SET current_stage_id = $2,
    status = $3,
    tracking_number = COALESCE(sqlc.narg('tracking_number'), tracking_number),
    updated_at = now(),
    updated_by = $4
WHERE id = $1;

-- name: SetCaseSanction :exec
UPDATE cases
SET sanctioned_amount = $2, interest_rate = $3, interest_type = $4,
    tenure_months = $5, sanction_date = $6, updated_at = now(), updated_by = $7
WHERE id = $1;

-- name: SetCaseDisbursement :exec
UPDATE cases
SET disbursed_amount = $2, disbursement_date = $3, updated_at = now(), updated_by = $4
WHERE id = $1;

-- name: SetCaseDocumentsPurgeAfter :exec
UPDATE documents SET purge_after = $2 WHERE case_id = $1 AND purge_after IS NULL;

-- name: ClearCaseDocumentsPurgeAfter :exec
UPDATE documents SET purge_after = NULL WHERE case_id = $1;

-- name: InsertCaseStageEventAt :exec
INSERT INTO case_stage_events (case_id, stage_id, reached_at, changed_by)
VALUES ($1, $2, $3, $4);

-- name: UpdateCaseFields :exec
UPDATE cases SET loan_amount = $2, follow_up_date = $3, updated_at = now(), updated_by = $4
WHERE id = $1;

-- name: DropCase :exec
UPDATE cases
SET status = 'dropped', dropped_reason_id = $2, dropped_remark = $3, dropped_at = now(),
    dropped_by = $4, dropped_via_customer = false, updated_at = now(), updated_by = $4
WHERE id = $1;

-- name: GetDropReason :one
SELECT id, value FROM dropdown_values WHERE id = $1 AND category = 'drop_reason' AND is_active;

-- name: SearchCustomerIDs :many
SELECT DISTINCT c.id, c.name FROM customers c
LEFT JOIN cases k ON k.customer_id = c.id
LEFT JOIN banks b ON b.id = k.bank_id
LEFT JOIN projects p ON p.id = k.project_id
WHERE c.name ILIKE sqlc.arg(name_pattern)
   OR c.mobile = sqlc.arg(mobile)
   OR k.tracking_number = sqlc.arg(tracking)
   OR b.name ILIKE sqlc.arg(name_pattern)
   -- NA is never matched: most cases are NA and "na" is in many names.
   OR (p.name ILIKE sqlc.arg(name_pattern) AND NOT p.is_system)
ORDER BY c.name, c.id;

-- name: CustomerSummary :one
SELECT id, name, mobile, COALESCE(email, '')::text AS email, status FROM customers WHERE id = $1;

-- name: CasesForCustomer :many
SELECT id, customer_id, bank_id, loan_amount, tracking_number, current_stage_id, assignee_id, status, created_at, follow_up_date, project_id,
       lead_id, unit_number, builder_contact_id
FROM cases WHERE customer_id = $1 ORDER BY created_at, id;

-- name: ListCases :many
-- Each filter is optional (NULL means no filter). Filters combine with AND.
-- Dropped cases are listed too, with their status.
SELECT id, customer_id, bank_id, loan_amount, tracking_number, current_stage_id, assignee_id, status, created_at, follow_up_date, project_id,
       lead_id, unit_number, builder_contact_id
FROM cases
WHERE (sqlc.narg('stage_id')::bigint IS NULL OR current_stage_id = sqlc.narg('stage_id')::bigint)
  AND (sqlc.narg('bank_id')::bigint IS NULL OR bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR project_id = sqlc.narg('project_id')::bigint)
  AND (sqlc.narg('assignee_id')::bigint IS NULL OR assignee_id = sqlc.narg('assignee_id')::bigint)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR created_at < sqlc.narg('to_ts')::timestamptz)
ORDER BY created_at DESC, id DESC;

-- name: CaseStageHistory :many
SELECT e.id, e.reached_at, s.code AS stage_code, s.name AS stage_name, u.full_name AS changed_by_name
FROM case_stage_events e
JOIN stages s ON s.id = e.stage_id
JOIN users u ON u.id = e.changed_by
WHERE e.case_id = $1
ORDER BY e.reached_at, e.id;

-- name: ListCaseRows :many
-- ListCases plus the names and stage progress the case list shows.
-- stage_count uses the bank's own stages when it has any, else the standard stages.
SELECT k.id, k.customer_id, k.bank_id, k.loan_amount, k.tracking_number, k.current_stage_id,
       k.assignee_id, k.status, k.created_at, k.follow_up_date, k.project_id, k.lead_id, k.unit_number, k.builder_contact_id,
       cu.name AS customer_name, cu.mobile AS customer_mobile, b.name AS bank_name, p.name AS project_name,
       s.name AS stage_name, s.code AS stage_code,
       (SELECT count(*) FROM stages x
         WHERE x.is_active AND x.bank_id IS NOT DISTINCT FROM s.bank_id AND x.sort_order <= s.sort_order)::int AS stage_order,
       (SELECT count(*) FROM stages x
         WHERE x.is_active
           AND x.bank_id IS NOT DISTINCT FROM
               (CASE WHEN EXISTS (SELECT 1 FROM stages y WHERE y.bank_id = k.bank_id AND y.is_active)
                     THEN k.bank_id END))::int AS stage_count,
       u.full_name AS assignee_name,
       COALESCE((SELECT max(e.reached_at) FROM case_stage_events e
                  WHERE e.case_id = k.id AND e.stage_id = k.current_stage_id), k.created_at)::timestamptz AS stage_since,
       (s.requires_document AND NOT EXISTS (SELECT 1 FROM documents d WHERE d.case_id = k.id))::boolean AS needs_document
FROM cases k
JOIN customers cu ON cu.id = k.customer_id
JOIN banks b ON b.id = k.bank_id
JOIN projects p ON p.id = k.project_id
JOIN stages s ON s.id = k.current_stage_id
JOIN users u ON u.id = k.assignee_id
WHERE (sqlc.narg('stage_id')::bigint IS NULL OR k.current_stage_id = sqlc.narg('stage_id')::bigint)
  AND (sqlc.narg('bank_id')::bigint IS NULL OR k.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR k.project_id = sqlc.narg('project_id')::bigint)
  AND (sqlc.narg('assignee_id')::bigint IS NULL OR k.assignee_id = sqlc.narg('assignee_id')::bigint)
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR k.created_at >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR k.created_at < sqlc.narg('to_ts')::timestamptz)
  AND (sqlc.narg('customer_id')::bigint IS NULL OR k.customer_id = sqlc.narg('customer_id')::bigint)
ORDER BY k.created_at DESC, k.id DESC;

-- name: AttentionCases :many
-- Active cases that need attention: the current stage requires a document and
-- none is uploaded, or the case reached its current stage on or before stuck_before
-- and that stage is not Disbursed. Oldest first.
SELECT * FROM (
SELECT k.id, k.customer_id, k.bank_id, k.loan_amount, k.tracking_number, k.current_stage_id,
       k.assignee_id, k.status, k.created_at, k.follow_up_date, k.project_id, k.lead_id, k.unit_number, k.builder_contact_id,
       cu.name AS customer_name, cu.mobile AS customer_mobile, b.name AS bank_name, p.name AS project_name,
       s.name AS stage_name, s.code AS stage_code,
       (SELECT count(*) FROM stages x
         WHERE x.is_active AND x.bank_id IS NOT DISTINCT FROM s.bank_id AND x.sort_order <= s.sort_order)::int AS stage_order,
       (SELECT count(*) FROM stages x
         WHERE x.is_active
           AND x.bank_id IS NOT DISTINCT FROM
               (CASE WHEN EXISTS (SELECT 1 FROM stages y WHERE y.bank_id = k.bank_id AND y.is_active)
                     THEN k.bank_id END))::int AS stage_count,
       u.full_name AS assignee_name,
       COALESCE((SELECT max(e.reached_at) FROM case_stage_events e
                  WHERE e.case_id = k.id AND e.stage_id = k.current_stage_id), k.created_at)::timestamptz AS stage_since,
       (s.requires_document AND NOT EXISTS (SELECT 1 FROM documents d WHERE d.case_id = k.id))::boolean AS needs_document
FROM cases k
JOIN customers cu ON cu.id = k.customer_id
JOIN banks b ON b.id = k.bank_id
JOIN projects p ON p.id = k.project_id
JOIN stages s ON s.id = k.current_stage_id
JOIN users u ON u.id = k.assignee_id
WHERE k.status = 'active'
  AND (sqlc.narg('assignee_id')::bigint IS NULL OR k.assignee_id = sqlc.narg('assignee_id')::bigint)
) r
WHERE r.needs_document
   OR (r.stage_since <= sqlc.arg('stuck_before')::timestamptz AND r.stage_code <> 'disbursed')
ORDER BY r.stage_since, r.id;

-- name: UpdateCaseProject :one
-- The case takes the new project's first active builder contact (none for NA
-- or a project with no contacts yet).
UPDATE cases AS k SET project_id = sqlc.arg(project_id)::bigint, updated_at = now(), updated_by = sqlc.arg(updated_by),
       builder_contact_id = (SELECT pc.id FROM project_contacts pc
                             WHERE pc.project_id = sqlc.arg(project_id)::bigint AND pc.removed_at IS NULL ORDER BY pc.id LIMIT 1)
WHERE k.id = sqlc.arg(id)
RETURNING k.builder_contact_id;

-- name: ProjectHasActiveContact :one
SELECT EXISTS (SELECT 1 FROM project_contacts WHERE project_id = $1 AND removed_at IS NULL)::boolean;

-- name: ActiveContactInProject :one
SELECT EXISTS (SELECT 1 FROM project_contacts WHERE id = $1 AND project_id = $2 AND removed_at IS NULL)::boolean;

-- name: GetBuilderContact :one
SELECT id, name, phone, COALESCE(email, '')::text AS email, COALESCE(label, '')::text AS label, (removed_at IS NOT NULL)::boolean AS removed
FROM project_contacts WHERE id = $1;

-- name: SetCaseBuilderContact :exec
UPDATE cases SET builder_contact_id = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: LockCustomerStatus :one
SELECT status FROM customers WHERE id = $1 FOR UPDATE;

-- name: LockLeadForCase :one
SELECT id, customer_id, project_id, status FROM leads WHERE id = $1 FOR UPDATE;

-- name: MarkLeadConverted :exec
UPDATE leads SET status = 'converted', converted_at = now(), next_call_date = NULL, updated_at = now(), updated_by = $2
WHERE id = $1;

-- name: SetCaseUnit :exec
UPDATE cases SET unit_number = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: BankExists :one
SELECT EXISTS (SELECT 1 FROM banks WHERE id = $1);
