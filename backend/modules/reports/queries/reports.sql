-- name: CountNewCustomers :one
SELECT count(*) FROM customers WHERE created_at >= $1 AND created_at < $2;

-- name: CountNewCases :one
SELECT count(*) FROM cases
WHERE created_at >= sqlc.arg(from_ts)::timestamptz AND created_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg('bank_id')::bigint IS NULL OR bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR project_id = sqlc.narg('project_id')::bigint);

-- name: CasesByStage :many
SELECT s.code, count(c.id)::bigint AS total
FROM cases c
JOIN stages s ON s.id = c.current_stage_id
WHERE c.status <> 'dropped'
  AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint)
GROUP BY s.code
ORDER BY s.code;

-- name: StageHits :one
-- Distinct cases that reached the stage (sanctioned or disbursed) in the period.
-- Amount is the sanctioned or disbursed amount of the case; dropped cases still count.
WITH hits AS (
  SELECT DISTINCT ON (c.id) c.id, c.status,
         CASE WHEN sqlc.arg(stage_code)::text = 'sanctioned' THEN c.sanctioned_amount ELSE c.disbursed_amount END AS amt
  FROM case_stage_events e
  JOIN stages s ON s.id = e.stage_id
  JOIN cases c ON c.id = e.case_id
  WHERE s.code = sqlc.arg(stage_code)::text
    AND e.reached_at >= sqlc.arg(from_ts)::timestamptz
    AND e.reached_at < sqlc.arg(to_ts)::timestamptz
    AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
    AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint)
  ORDER BY c.id
)
SELECT count(*)::bigint AS cases,
       count(*) FILTER (WHERE status = 'dropped')::bigint AS dropped,
       COALESCE(sum(amt), 0)::text AS amount
FROM hits;

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
-- Stage moves in a range: every stage event except the Received event a case opens with.
SELECT count(*) FROM case_stage_events e
JOIN stages s ON s.id = e.stage_id
JOIN cases c ON c.id = e.case_id
WHERE s.code <> 'received'
  AND e.reached_at >= sqlc.arg(from_ts)::timestamptz AND e.reached_at < sqlc.arg(to_ts)::timestamptz
  AND (sqlc.narg('bank_id')::bigint IS NULL OR c.bank_id = sqlc.narg('bank_id')::bigint)
  AND (sqlc.narg('project_id')::bigint IS NULL OR c.project_id = sqlc.narg('project_id')::bigint);
