-- name: InsertCustomer :one
-- Conflicts on mobile return no row; the caller then reads the existing customer.
INSERT INTO customers (name, mobile, email, created_by, updated_by)
VALUES ($1, $2, $3, $4, $4)
ON CONFLICT (mobile) DO NOTHING
RETURNING id, name, mobile, email, status;

-- name: GetCustomerByMobile :one
SELECT id, name, mobile, email, status FROM customers WHERE mobile = $1;

-- name: GetCustomer :one
SELECT id, name, mobile, email, status FROM customers WHERE id = $1;

-- name: UpdateCustomerFields :exec
UPDATE customers SET name = $2, mobile = $3, email = $4, updated_at = now(), updated_by = $5
WHERE id = $1;

-- name: LockCustomer :one
SELECT id, status FROM customers WHERE id = $1 FOR UPDATE;

-- name: DropCustomer :exec
UPDATE customers
SET status = 'dropped', dropped_reason_id = $2, dropped_remark = $3, dropped_at = now(),
    dropped_by = $4, updated_at = now(), updated_by = $4
WHERE id = $1;

-- name: DropCustomerCases :exec
-- Only live cases: an already-dropped case keeps its own reason (owner answer A).
UPDATE cases
SET status = 'dropped', dropped_reason_id = $2, dropped_remark = $3, dropped_at = now(),
    dropped_by = $4, dropped_via_customer = true, updated_at = now(), updated_by = $4
WHERE customer_id = $1 AND status <> 'dropped';

-- name: SetCustomerCasesDocumentsPurgeAfter :exec
UPDATE documents SET purge_after = $2
WHERE purge_after IS NULL AND case_id IN (SELECT id FROM cases WHERE customer_id = $1);

-- name: GetDropReason :one
SELECT id, value FROM dropdown_values WHERE id = $1 AND category = 'drop_reason' AND is_active;

-- name: LockCustomerDrop :one
SELECT id, status, dropped_reason_id, dropped_remark, dropped_at, dropped_by
FROM customers WHERE id = $1 FOR UPDATE;

-- name: ReactivateCustomer :exec
UPDATE customers
SET status = 'active', dropped_reason_id = NULL, dropped_remark = NULL, dropped_at = NULL,
    dropped_by = NULL, updated_at = now(), updated_by = $2
WHERE id = $1;
