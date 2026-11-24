-- name: InsertCustomerForLead :one
-- Conflicts on mobile return no row; the caller then reads the existing customer.
INSERT INTO customers (name, mobile, email, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
ON CONFLICT (mobile) DO NOTHING
RETURNING id, name, mobile, email, status;

-- name: GetCustomerByMobile :one
SELECT id, name, mobile, email, status FROM customers WHERE mobile = $1;

-- name: LockCustomerStatus :one
SELECT status FROM customers WHERE id = $1 FOR UPDATE;

-- name: ProjectExists :one
SELECT EXISTS (SELECT 1 FROM projects WHERE id = $1);

-- name: ActiveUserExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND is_active);

-- name: InsertLead :one
-- The lead starts with the project's first active builder contact (none for
-- NA or a project with no contacts yet).
INSERT INTO leads (customer_id, project_id, assignee_id, next_call_date, created_by, updated_by, builder_contact_id)
VALUES ($1, $2, $3, $4, $5, $5,
        (SELECT pc.id FROM project_contacts pc WHERE pc.project_id = $2 AND pc.removed_at IS NULL ORDER BY pc.id LIMIT 1))
RETURNING id;

-- name: LockLead :one
SELECT id, customer_id, project_id, status, builder_contact_id FROM leads WHERE id = $1 FOR UPDATE;

-- name: ActiveContactInProject :one
SELECT EXISTS (SELECT 1 FROM project_contacts WHERE id = $1 AND project_id = $2 AND removed_at IS NULL)::boolean;

-- name: SetLeadBuilderContact :exec
UPDATE leads SET builder_contact_id = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: SetLeadAssignee :exec
UPDATE leads SET assignee_id = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: GetLeadRow :one
SELECT l.id, l.customer_id, l.project_id, l.assignee_id, l.status, l.next_call_date, l.created_at, l.converted_at,
       cu.name AS customer_name, cu.mobile AS customer_mobile, COALESCE(cu.email, '')::text AS customer_email,
       p.name AS project_name, u.full_name AS assignee_name,
       COALESCE(last.response, '')::text AS last_response, last.created_at AS last_contact_at, c.id AS case_id,
       l.builder_contact_id, COALESCE(bc.name, '')::text AS bc_name, COALESCE(bc.phone, '')::text AS bc_phone,
       COALESCE(bc.email, '')::text AS bc_email, COALESCE(bc.label, '')::text AS bc_label, (bc.removed_at IS NOT NULL)::boolean AS bc_removed
FROM leads l
JOIN customers cu ON cu.id = l.customer_id
JOIN projects p ON p.id = l.project_id
JOIN users u ON u.id = l.assignee_id
LEFT JOIN LATERAL (
  SELECT lc.response, lc.created_at FROM lead_contacts lc WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1
) last ON true
LEFT JOIN cases c ON c.lead_id = l.id
LEFT JOIN project_contacts bc ON bc.id = l.builder_contact_id
WHERE l.id = $1;

-- name: ListOpenLeadRows :many
-- Calls due on or before today first (oldest first), then the rest newest first.
SELECT l.id, l.customer_id, l.project_id, l.assignee_id, l.status, l.next_call_date, l.created_at, l.converted_at,
       cu.name AS customer_name, cu.mobile AS customer_mobile, COALESCE(cu.email, '')::text AS customer_email,
       p.name AS project_name, u.full_name AS assignee_name,
       COALESCE(last.response, '')::text AS last_response, last.created_at AS last_contact_at, c.id AS case_id,
       l.builder_contact_id, COALESCE(bc.name, '')::text AS bc_name, COALESCE(bc.phone, '')::text AS bc_phone,
       COALESCE(bc.email, '')::text AS bc_email, COALESCE(bc.label, '')::text AS bc_label, (bc.removed_at IS NOT NULL)::boolean AS bc_removed
FROM leads l
JOIN customers cu ON cu.id = l.customer_id
JOIN projects p ON p.id = l.project_id
JOIN users u ON u.id = l.assignee_id
LEFT JOIN LATERAL (
  SELECT lc.response, lc.created_at FROM lead_contacts lc WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1
) last ON true
LEFT JOIN cases c ON c.lead_id = l.id
LEFT JOIN project_contacts bc ON bc.id = l.builder_contact_id
WHERE l.status = 'open'
ORDER BY (l.next_call_date IS NOT NULL AND l.next_call_date <= sqlc.arg(today)::date) DESC,
         CASE WHEN l.next_call_date <= sqlc.arg(today)::date THEN l.next_call_date END ASC,
         l.created_at DESC, l.id DESC;

-- name: LeadRowsForCustomer :many
SELECT l.id, l.customer_id, l.project_id, l.assignee_id, l.status, l.next_call_date, l.created_at, l.converted_at,
       cu.name AS customer_name, cu.mobile AS customer_mobile, COALESCE(cu.email, '')::text AS customer_email,
       p.name AS project_name, u.full_name AS assignee_name,
       COALESCE(last.response, '')::text AS last_response, last.created_at AS last_contact_at, c.id AS case_id,
       l.builder_contact_id, COALESCE(bc.name, '')::text AS bc_name, COALESCE(bc.phone, '')::text AS bc_phone,
       COALESCE(bc.email, '')::text AS bc_email, COALESCE(bc.label, '')::text AS bc_label, (bc.removed_at IS NOT NULL)::boolean AS bc_removed
FROM leads l
JOIN customers cu ON cu.id = l.customer_id
JOIN projects p ON p.id = l.project_id
JOIN users u ON u.id = l.assignee_id
LEFT JOIN LATERAL (
  SELECT lc.response, lc.created_at FROM lead_contacts lc WHERE lc.lead_id = l.id ORDER BY lc.created_at DESC, lc.id DESC LIMIT 1
) last ON true
LEFT JOIN cases c ON c.lead_id = l.id
LEFT JOIN project_contacts bc ON bc.id = l.builder_contact_id
WHERE l.customer_id = $1
ORDER BY l.created_at DESC, l.id DESC;

-- name: InsertContact :exec
INSERT INTO lead_contacts (lead_id, response, remark, next_call_date, created_by)
VALUES ($1, $2, $3, $4, $5);

-- name: SetLeadNextCall :exec
UPDATE leads SET next_call_date = $2, updated_at = now(), updated_by = $3 WHERE id = $1;

-- name: ListContacts :many
SELECT lc.id, lc.response, lc.remark, lc.next_call_date, lc.created_at, COALESCE(u.full_name, '')::text AS created_by
FROM lead_contacts lc
LEFT JOIN users u ON u.id = lc.created_by
WHERE lc.lead_id = $1
ORDER BY lc.created_at DESC, lc.id DESC;

-- name: GetDropReason :one
SELECT id, value FROM dropdown_values WHERE id = $1 AND category = 'drop_reason' AND is_active;

-- name: DropLead :exec
UPDATE leads
SET status = 'dropped', dropped_reason_id = $2, dropped_remark = $3, dropped_at = now(), dropped_by = $4,
    next_call_date = NULL, updated_at = now(), updated_by = $4
WHERE id = $1;

-- name: ReopenLead :exec
UPDATE leads
SET status = 'open', dropped_reason_id = NULL, dropped_remark = NULL, dropped_at = NULL, dropped_by = NULL,
    next_call_date = $2, updated_at = now(), updated_by = $3
WHERE id = $1;

-- name: LeadHasCase :one
SELECT EXISTS (SELECT 1 FROM cases WHERE lead_id = $1);

-- name: QueueLeadDueToday :exec
-- The daily job runs before office hours, so a call due today that is set
-- during the day queues its reminder now. The alert index keeps it to one.
INSERT INTO alert_log (lead_id, recipient_user_id, kind, due_on)
SELECT l.id, l.assignee_id, 'follow_up_due', l.next_call_date
FROM leads l
WHERE l.id = $1 AND l.status = 'open' AND l.next_call_date = sqlc.arg(today)::date
ON CONFLICT DO NOTHING;
