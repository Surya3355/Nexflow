-- name: LockCaseForDocument :one
SELECT id, current_stage_id, status FROM cases WHERE id = $1 FOR UPDATE;

-- name: FindDocumentByLabel :one
SELECT id, storage_key FROM documents WHERE case_id = $1 AND label = $2 FOR UPDATE;

-- name: InsertDocument :one
INSERT INTO documents (case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, purge_after, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, uploaded_at;

-- name: ReplaceDocument :one
UPDATE documents
SET stage_id = $2, file_name = $3, content_type = $4, size_bytes = $5,
    storage_key = $6, uploaded_at = now(), uploaded_by = $7
WHERE id = $1
RETURNING uploaded_at;

-- name: GetDocument :one
SELECT id, case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, uploaded_at, uploaded_by
FROM documents WHERE id = $1;

-- name: GetCaseRequirement :one
SELECT s.requires_document,
       (SELECT count(*) FROM documents d WHERE d.case_id = c.id) AS document_count
FROM cases c
JOIN stages s ON s.id = c.current_stage_id
WHERE c.id = $1;

-- name: ListCaseDocuments :many
SELECT id, case_id, stage_id, label, file_name, content_type, size_bytes, storage_key, uploaded_at, uploaded_by
FROM documents WHERE case_id = $1 ORDER BY label, id;

-- name: CaseLabelTaken :one
SELECT EXISTS (SELECT 1 FROM documents WHERE case_id = $1 AND label = $2);

-- name: CaseStageForDocument :one
SELECT current_stage_id, status FROM cases WHERE id = $1;

-- name: DeleteDocument :one
DELETE FROM documents WHERE id = $1 RETURNING case_id, label, storage_key;
