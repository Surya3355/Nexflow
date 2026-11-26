-- name: CountNewCustomers :one
SELECT count(*) FROM customers WHERE created_at >= $1 AND created_at < $2;

-- name: CountNewCases :one
SELECT count(*) FROM cases
WHERE created_at >= sqlc.arg(from_ts)::timestamptz AND created_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg('bank_id')::bigint IS NULL OR bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR project_id = sqlc.narg('project_id')::bigint);

-- name: CasesByStage :many
-- Open cases per bank and current stage; each bank's own stages in order.
SELECT b.id AS bank_id, b.name AS bank_name, s.id AS stage_id, s.code AS stage_code, s.name AS stage_name,
       s.sort_order, count(c.id)::bigint AS total
FROM cases c
JOIN stages s ON s.id = c.current_stage_id
JOIN banks b ON b.id = c.bank_id
WHERE c.status <> 'dropped'
  AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint)
GROUP BY b.id, b.name, s.id, s.code, s.name, s.sort_order
ORDER BY b.name, b.id, s.sort_order;

-- name: SanctionHits :one
-- Distinct cases that reached a sanction-role stage in the period, any bank.
-- Amount is the sanctioned amount; dropped cases still count (O8).
WITH hits AS (
  SELECT DISTINCT ON (c.id) c.id, c.status, c.sanctioned_amount AS amt
  FROM case_stage_events e
  JOIN stages s ON s.id = e.stage_id
  JOIN cases c ON c.id = e.case_id
  WHERE s.role = 'sanction'
    AND e.reached_at >= sqlc.arg(from_ts)::timestamptz
    AND e.reached_at < sqlc.arg(to_ts)::timestamptz
    AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
    AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint)
  ORDER BY c.id
)
SELECT count(*)::bigint AS cases,
       count(*) FILTER (WHERE status = 'dropped')::bigint AS dropped,
       COALESCE(sum(amt), 0)::numeric(14,2)::text AS amount
FROM hits;

-- name: DisbursementHits :one
-- Cases with at least one payment in the period; amount is the sum of those payments.
WITH hits AS (
  SELECT c.id, c.status, sum(d.amount) AS amt
  FROM case_disbursements d
  JOIN cases c ON c.id = d.case_id
  WHERE d.paid_on >= (sqlc.arg(from_ts)::timestamptz AT TIME ZONE 'Asia/Kolkata')::date
    AND d.paid_on <= ((sqlc.arg(to_ts)::timestamptz - interval '1 microsecond') AT TIME ZONE 'Asia/Kolkata')::date
    AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
    AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint)
  GROUP BY c.id, c.status
)
SELECT count(*)::bigint AS cases,
       count(*) FILTER (WHERE status = 'dropped')::bigint AS dropped,
       COALESCE(sum(amt), 0)::numeric(14,2)::text AS amount
FROM hits;

-- name: DisbursementRows :many
-- One row per case with a payment in the period (owner: never one row per payment).
WITH period AS (
  SELECT d.case_id, sum(d.amount) AS paid_in_period
  FROM case_disbursements d
  WHERE d.paid_on >= (sqlc.arg(from_ts)::timestamptz AT TIME ZONE 'Asia/Kolkata')::date
    AND d.paid_on <= ((sqlc.arg(to_ts)::timestamptz - interval '1 microsecond') AT TIME ZONE 'Asia/Kolkata')::date
  GROUP BY d.case_id
), totals AS (
  SELECT d.case_id, sum(d.amount) AS total_paid, max(d.paid_on) AS last_paid_on,
         bool_or(d.kind = 'full') AS fully_paid
  FROM case_disbursements d WHERE d.case_id IN (SELECT case_id FROM period)
  GROUP BY d.case_id
)
SELECT k.id, cu.name AS customer_name, cu.mobile, b.name AS bank_name, p.name AS project_name,
       COALESCE(k.tracking_number, '')::text AS tracking_number, u.full_name AS assignee_name, k.status,
       pe.paid_in_period::numeric(14,2)::text AS paid_in_period,
       t.total_paid::numeric(14,2)::text AS total_paid,
       COALESCE(k.sanctioned_amount, k.loan_amount)::numeric(14,2)::text AS sanctioned,
       GREATEST(COALESCE(k.sanctioned_amount, k.loan_amount) - t.total_paid, 0)::numeric(14,2)::text AS remaining,
       t.last_paid_on::text AS last_paid_on, t.fully_paid
FROM period pe
JOIN totals t ON t.case_id = pe.case_id
JOIN cases k ON k.id = pe.case_id
JOIN customers cu ON cu.id = k.customer_id
JOIN banks b ON b.id = k.bank_id
JOIN projects p ON p.id = k.project_id
JOIN users u ON u.id = k.assignee_id
WHERE (sqlc.narg('bank_id')::bigint IS NULL OR k.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR k.project_id = sqlc.narg('project_id')::bigint)
ORDER BY t.last_paid_on DESC, k.id DESC;

-- name: ReportRows :many
-- One row per case, with the same optional filters as the case list (spec §8).
SELECT k.id, cu.name AS customer_name, cu.mobile, b.name AS bank_name, p.name AS project_name, s.name AS stage_name,
       k.status, u.full_name AS assignee_name, k.loan_amount::text AS loan_amount,
       COALESCE(k.tracking_number, '')::text AS tracking_number,
       k.created_at, k.follow_up_date
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
ORDER BY k.created_at DESC, k.id DESC;

-- name: CountStageMoves :one
-- Stage moves in a range: every stage event except each case's first, the stage it opened at.
SELECT count(*) FROM case_stage_events e
JOIN cases c ON c.id = e.case_id
WHERE e.id <> (SELECT min(f.id) FROM case_stage_events f WHERE f.case_id = e.case_id)
  AND e.reached_at >= sqlc.arg(from_ts)::timestamptz AND e.reached_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint);
