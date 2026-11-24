-- name: EnqueueAlert :execrows
INSERT INTO alert_log (case_id, recipient_user_id, kind, due_on)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: UnsentAlerts :many
-- An alert belongs to a case or to a lead; the customer comes through either.
SELECT a.id, a.case_id, a.kind, a.due_on, a.attempts,
       u.username, COALESCE(u.email, '')::text AS email, u.full_name,
       c.id AS case_ref, l.id AS lead_ref, cu.name AS customer_name, COALESCE(p.name, '')::text AS project_name
FROM alert_log a
JOIN users u ON u.id = a.recipient_user_id
LEFT JOIN cases c ON c.id = a.case_id
LEFT JOIN leads l ON l.id = a.lead_id
LEFT JOIN projects p ON p.id = l.project_id
JOIN customers cu ON cu.id = COALESCE(c.customer_id, l.customer_id)
WHERE a.sent_at IS NULL AND u.is_active
ORDER BY a.queued_at, a.id
LIMIT $1;

-- name: MarkAlertSent :exec
UPDATE alert_log SET sent_at = now(), attempts = attempts + 1, last_error = NULL WHERE id = $1;

-- name: MarkAlertFailed :exec
UPDATE alert_log SET attempts = attempts + 1, last_error = $2 WHERE id = $1;

-- name: QueueFollowUpDue :execrows
-- Active cases whose follow-up falls on the given IST day (spec §10).
INSERT INTO alert_log (case_id, recipient_user_id, kind, due_on)
SELECT c.id, c.assignee_id, 'follow_up_due', c.follow_up_date
FROM cases c
WHERE c.follow_up_date = sqlc.arg(due_day)::date AND c.status = 'active'
ON CONFLICT DO NOTHING;

-- name: QueueFollowUpOverdue :execrows
-- Active cases whose follow-up was the given day: queued once, on the next day.
INSERT INTO alert_log (case_id, recipient_user_id, kind, due_on)
SELECT c.id, c.assignee_id, 'follow_up_overdue', c.follow_up_date
FROM cases c
WHERE c.follow_up_date = sqlc.arg(due_day)::date AND c.status = 'active'
ON CONFLICT DO NOTHING;

-- name: EnqueueLeadAlert :execrows
INSERT INTO alert_log (lead_id, recipient_user_id, kind, due_on)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING;

-- name: QueueLeadCallDue :execrows
-- Open leads whose next call falls on the given IST day.
INSERT INTO alert_log (lead_id, recipient_user_id, kind, due_on)
SELECT l.id, l.assignee_id, 'follow_up_due', l.next_call_date
FROM leads l
WHERE l.next_call_date = sqlc.arg(due_day)::date AND l.status = 'open'
ON CONFLICT DO NOTHING;

-- name: QueueLeadCallOverdue :execrows
-- Open leads whose next call was the given day or earlier: one overdue alert
-- per call date, so a date logged late still gets its reminder.
INSERT INTO alert_log (lead_id, recipient_user_id, kind, due_on)
SELECT l.id, l.assignee_id, 'follow_up_overdue', l.next_call_date
FROM leads l
WHERE l.next_call_date <= sqlc.arg(due_day)::date AND l.status = 'open'
ON CONFLICT DO NOTHING;
