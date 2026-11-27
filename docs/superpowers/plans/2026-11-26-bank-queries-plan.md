# Bank Queries Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Staff record bank queries on a case, send proof (note and/or file), mark them resolved only when the bank accepts, and cannot move the case forward while any query is unresolved.

**Architecture:** New `bankqueries` module (own sqlc package) with `case_queries` + append-only `case_query_events`. Proof files are ordinary case documents, written through three new transaction-aware methods on the documents service. The cases module blocks forward stage moves and adds an open-query count to case rows and Needs attention; the alerts job queues due/overdue alerts. Web: a `BankQueries` box inside the Progress card (right half on desktop, above the stages on phone).

**Tech Stack:** Go 1.25, pgx v5, sqlc, golang-migrate, PostgreSQL 16; React 18 + TypeScript + Tailwind, Vitest; Playwright phone check.

**Spec:** `docs/superpowers/specs/2026-11-26-bank-queries-design.md` (mockup `mockups/round15/queries.html`).

## Global Constraints

- Commands: backend tests `cd backend && docker compose run --rm --build test go test -count=1 ./...` (local Go needs `export PATH=$HOME/.local/go/bin:$PATH`); sqlc `cd backend && ~/go/bin/sqlc generate`; web `cd web && npx vitest run` and `npm run build`; phone check per Task 10.
- Known pre-existing failures, not ours: `TestAcceptance19`, `TestCallDueTodayQueuesReminderAtOnce`.
- Statuses: `open`, `sent`, `resolved`. Shown as "Open", "Sent to bank", "Resolved". Removed = `removed_at` set (status kept).
- Body (what the bank asked) and every note/reason: trimmed, not blank where required, ≤ 500 characters.
- Proof = a note or a file (or both). File rules are the documents rules: PDF/JPG/PNG, ≤ 10 MB.
- Proof file label: `Query <n> proof`, or `Query <n> proof 2`, `3`, … if taken (never replaces an existing document).
- Every action needs `access.ActionEditRecord` (all roles); a dropped case is read-only for queries.
- Forward-move refusal text (web): "Resolve the open bank queries first." Box copy as in the mockup.
- Commit with the configured git identity, on `docs/project-foundation`; never push.

## Review Focus

1. A stored proof file left behind when the transaction fails (store write happens before commit) — the file must be discarded.
2. A proof document purged by the one-year purge — events keep working with `document_id` NULL ("file deleted").
3. Two staff acting on the same query at once (e.g. accept vs remove proof) — the query row is locked; the second gets a clear 409.
4. A case moved forward in the same instant a query is added — both lock the case row, so the move sees the new query.
5. Archive → restore of a case with queries whose proof document exists — restore order must put documents before query events.

---

### Task 1: Migration 000009

**Files:**
- Create: `backend/migrations/000009_bank_queries.up.sql`, `.down.sql`
- Modify: `backend/migrations/migrate_test.go`

**Interfaces:**
- Produces tables:
  - `case_queries(id, case_id → cases, number int, body text, raised_on date, due_on date NULL, assignee_id → users, status text, removed_at timestamptz NULL, removed_by → users NULL, created_at/by, updated_at/by)`; `UNIQUE (case_id, number)`; CHECKs: body not blank and ≤ 500 chars, status in the three values, `due_on IS NULL OR due_on >= raised_on`; index `(case_id) WHERE removed_at IS NULL AND status <> 'resolved'`.
  - `case_query_events(id, query_id → case_queries, kind text, note text NULL (≤ 500), document_id → documents ON DELETE SET NULL, at timestamptz default now(), by → users)`; kind in `raised, edited, sent, accepted, rejected, proof_removed, removed`; index `(query_id, at, id)`.
  - `alert_log.kind` CHECK extended with `query_due`, `query_overdue` (drop and re-add the constraint by its generated name `alert_log_kind_check`).

- [ ] **Step 1:** Write `TestMigration9Queries`: after up, insert a query and a `sent` event with a document; assert a blank body (23514), a bad status (23514), `due_on < raised_on` (23514) and a duplicate `(case_id, number)` (23505) are refused; delete the document and assert the event's `document_id` is NULL; insert an `alert_log` row of kind `query_due`; down then up succeeds.
- [ ] **Step 2:** Run it: FAIL (no file 000009).
- [ ] **Step 3:** Write both SQL files (down drops tables and restores the old kind check after deleting `query_%` alerts).
- [ ] **Step 4:** Run `go test ./migrations/`: PASS.
- [ ] **Step 5:** Commit "Add bank queries tables (migration 9)".

### Task 2: Documents in a caller's transaction

**Files:**
- Modify: `backend/modules/documents/upload.go`, `backend/modules/documents/queries/documents.sql`
- Test: `backend/modules/documents/tx_test.go`

**Interfaces:**
- Produces:
  - `type Stored struct{ Key, Name, ContentType string; Size int64 }`
  - `func (s *Service) StoreFile(file UploadedFile) (Stored, error)` — validates (`ValidateFile`) and puts bytes in the store.
  - `func (s *Service) DiscardFile(key string)` — best-effort delete.
  - `func (s *Service) InsertTx(ctx context.Context, tx pgx.Tx, actor access.User, caseID int64, baseLabel string, f Stored) (Document, error)` — inserts at the case's current stage with the same purge-date rule as `Upload`, picks the first free label (`baseLabel`, `baseLabel 2`, …), audits `document.uploaded`. Caller already holds the case lock.
  - `func (s *Service) DeleteTx(ctx context.Context, tx pgx.Tx, actor access.User, docID int64) (storageKey string, err error)` — deletes the row, audits `document.removed`.
- `Upload` keeps its behaviour (refactor may reuse `StoreFile`).

- [ ] **Step 1:** Tests: `TestInsertTxPicksFreeLabel` (existing "Query 1 proof" → new is "Query 1 proof 2"; old untouched), `TestInsertTxRollbackLeavesNoRow`, `TestDeleteTxRemovesRowAndAudits`, `TestStoreFileRejectsBadType`.
- [ ] **Step 2:** Run: FAIL (undefined).
- [ ] **Step 3:** Implement; sqlc generate.
- [ ] **Step 4:** `go test ./modules/documents/`: PASS.
- [ ] **Step 5:** Commit "Documents: store, insert and delete inside a caller's transaction".

### Task 3: bankqueries service — add, edit, list

**Files:**
- Create: `backend/modules/bankqueries/{bankqueries.go, queries/bankqueries.sql, bankqueriesdb/*}`; sqlc.yaml entry
- Test: `backend/modules/bankqueries/bankqueries_test.go`

**Interfaces:**
- Produces:
  - Errors: `ErrBodyRequired`, `ErrBodyTooLong`, `ErrRaisedInFuture`, `ErrDueBeforeRaised`, `ErrAssigneeInactive`, `ErrCaseNotFound`, `ErrCaseDropped`, `ErrQueryNotFound`, `ErrQueryResolved` (edit/remove a resolved query), `ErrNotOpen`, `ErrNotSent`, `ErrProofRequired`, `ErrNoteTooLong`, `ErrReasonRequired`.
  - `type NewQuery struct{ Body string; RaisedOn time.Time; DueOn *time.Time; AssigneeID int64 }`
  - `type Query struct{ ID, CaseID int64; Number int; Body string; RaisedOn time.Time; DueOn *time.Time; AssigneeID int64; AssigneeName, Status string; Proof *Proof; Events []Event }`
  - `type Proof struct{ Note string; DocumentID *int64; DocumentLabel, FileName string; At time.Time; By string }` — from the latest `sent` event, only while status is `sent`.
  - `type Event struct{ Kind, Note string; At time.Time; By string; DocumentID *int64 }`
  - `func NewService(pool *pgxpool.Pool, docs *documents.Service, rec history.Recorder) *Service`
  - `Add(ctx, actor, caseID int64, in NewQuery) (Query, error)` — locks the case (`FOR UPDATE`), number = max+1 in case, event `raised`, audit `query.added`.
  - `Edit(ctx, actor, queryID int64, in NewQuery) (Query, error)` — open or sent only; event `edited`; audit `query.edited`.
  - `List(ctx, caseID int64) ([]Query, error)` — not removed, order: unresolved first by number, then resolved by number.
  - `OpenCount(ctx, caseID) (int, error)`.
  - helper `lockQuery(ctx, q, id)` locks the case then the query (same order everywhere: case, query).
- "Today" is IST (copy `istZone` pattern).

- [ ] **Step 1:** Tests: add happy path (number 1 then 2; event `raised`; audit row); each validation error above that applies to Add; dropped case → `ErrCaseDropped`; inactive assignee → `ErrAssigneeInactive`; Edit on resolved → `ErrQueryResolved`; List order and that removed queries are hidden (insert removed row directly).
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement queries + service; sqlc generate.
- [ ] **Step 4:** `go test ./modules/bankqueries/`: PASS.
- [ ] **Step 5:** Commit "Bank queries: add, edit and list".

### Task 4: bankqueries service — proof and bank answer

**Files:** Modify `backend/modules/bankqueries/bankqueries.go`, its SQL; tests in `proof_test.go`.

**Interfaces:**
- Produces:
  - `Send(ctx, actor, queryID int64, note string, file *documents.UploadedFile) (Query, error)` — status `open` only (`ErrNotOpen`); note ≤ 500; note or file required (`ErrProofRequired`). File: `StoreFile` before the tx, `InsertTx` with base label `Query <n> proof` inside; any failure after `StoreFile` → `DiscardFile`. Event `sent` (note, document_id); status `sent`; audit `query.sent`.
  - `Accept(ctx, actor, queryID)` — `sent` only (`ErrNotSent`); status `resolved`; event `accepted`; audit `query.accepted`.
  - `Reject(ctx, actor, queryID, reason string)` — `sent` only; reason required (`ErrReasonRequired`); status `open`; event `rejected` with the reason; the proof document stays; audit `query.rejected`.
  - `RemoveProof(ctx, actor, queryID)` — `sent` only; `DeleteTx` the latest sent event's document if still present, then `DiscardFile` after commit; status `open`; event `proof_removed`; audit `query.proof_removed`.
  - `Remove(ctx, actor, queryID)` — open or sent (`ErrQueryResolved` otherwise); sets `removed_at/by`; event `removed`; audit `query.removed`. Its proof document stays in Documents.
- All of these refuse a dropped case (`ErrCaseDropped`).

- [ ] **Step 1:** Tests: send with note only, file only, both, neither (`ErrProofRequired`); send twice (`ErrNotOpen`); proof label "Query 1 proof" and on a resend after reject "Query 1 proof 2"; `TestSendDiscardsFileWhenAuditFails` (failing recorder → store has no file, no document row); accept from open (`ErrNotSent`); accept from sent → resolved; reject without reason; reject → open, old document still listed; remove proof → document row gone, file gone from MemoryStore, status open; remove resolved → `ErrQueryResolved`; remove sent → hidden from List, document kept; `List` shows `Proof` only while sent.
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./modules/bankqueries/ ./modules/documents/`: PASS.
- [ ] **Step 5:** Commit "Bank queries: send proof, bank accepted or rejected, remove".

### Task 5: Cases — block forward moves, open count, Needs attention

**Files:** Modify `backend/modules/cases/stage.go`, `queries/cases.sql`, `attention.go`, case row type (`CaseRow.OpenQueries int`); tests `backend/modules/cases/queries_block_test.go`.

**Interfaces:**
- Produces: `ErrOpenQueries = errors.New("cases: resolve the open bank queries first")`; `CountOpenCaseQueries` query (status in open/sent, not removed); `open_queries` int column in `ListCaseRows` and `AttentionCases` (same position in both so `toCaseRow(casesdb.ListCaseRowsRow(r))` still compiles).
- Rule in `ChangeStage`, after the case lock and finding `to`: if `to.SortOrder > current.SortOrder` and count > 0 → `ErrOpenQueries`. Backward, same-stage and drop are unaffected.
- `AttentionCases` WHERE gains `OR r.open_queries > 0`; reason, checked first: `"1 open bank query"` / `"<n> open bank queries"`.

- [ ] **Step 1:** Tests (insert queries rows directly with SQL): forward move refused with one `open` and with one `sent`; allowed after it is `resolved` or removed; backward move allowed with an open query; attention lists the case with reason "2 open bank queries" even when it is not stuck; `CaseRow.OpenQueries` = 2 in `List`.
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement; sqlc generate.
- [ ] **Step 4:** `go test ./modules/cases/`: PASS.
- [ ] **Step 5:** Commit "Cases: open bank queries block forward moves and need attention".

### Task 6: Alerts — query due and overdue

**Files:** Modify `backend/modules/alerts/{followup.go, queue.go, queries/alerts.sql}`; test `backend/modules/alerts/queries_test.go`.

**Interfaces:**
- Produces: `KindQueryDue = "query_due"`, `KindQueryOverdue = "query_overdue"`; `QueueQueryDue(day)`, `QueueQueryOverdue(day)` — queries with `status = 'open'`, not removed, case `active`, `due_on` = day (due) / = day before (overdue); recipient = query assignee; one alert per case, kind, due date and recipient (existing unique index). `FollowUpResult` gains `QueryDue`, `QueryOverdue`; `RunFollowUps` runs both.
- Email text: due → subject "Bank query due today", body "Case <id> for <customer> has a bank query due today."; overdue → "Bank query overdue", "Case <id> for <customer> has a bank query that is overdue."

- [ ] **Step 1:** Tests: open query due today → one `query_due` to its assignee; a `sent` query due today → none; due yesterday → one `query_overdue`; second run queues nothing new; `compose` texts for both kinds.
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement; sqlc generate.
- [ ] **Step 4:** `go test ./modules/alerts/`: PASS (except the known clock test).
- [ ] **Step 5:** Commit "Alerts: bank query due and overdue".

### Task 7: Archive — carry queries with the case

**Files:** Modify `backend/modules/archive/{queries/archive.sql, delete.go, restore.go}`; test `backend/modules/archive/queries_test.go`.

**Interfaces:** `CaseChildrenJSON` adds `queries` and `query_events`; delete removes events, then queries, **before** documents; restore inserts `queries` and `query_events` **after** documents; snapshot fields default to `[]` for older snapshots.

- [ ] **Step 1:** Test: case with a query whose `sent` event has a document → archive (delete) → restore → query, event and `document_id` are back; an old-shape snapshot without the keys restores.
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement; sqlc generate.
- [ ] **Step 4:** `go test ./modules/archive/`: PASS.
- [ ] **Step 5:** Commit "Archive: keep bank queries with the case".

### Task 8: HTTP routes and wiring

**Files:** Create `backend/modules/http/queries_handlers.go`, `queries_routes_test.go`; modify `router.go` (`Deps.Queries *bankqueries.Service`), `errors.go`, `handlers.go` (case row JSON `open_queries`), `cmd/nexflow/main.go`, `acceptance/env_test.go`, `routes_test.go`.

**Interfaces (JSON, snake_case):**
- `GET /api/cases/{id}/queries` → `{ "queries": [Query], "open_count": n }`; Query = `{id, number, body, raised_on, due_on|null, assignee:{id,name}, status, proof:{note, document:{id,label,file_name}|null, at, by}|null, events:[{kind, note, at, by, document_id|null}]}`.
- `POST /api/cases/{id}/queries` `{body, raised_on, due_on?, assignee_id}` → 201 Query.
- `PUT /api/queries/{id}` same body → Query.
- `POST /api/queries/{id}/send` multipart `note`, optional `file` → Query (same size limit as uploads).
- `POST /api/queries/{id}/accept` → Query; `POST /api/queries/{id}/reject` `{reason}` → Query; `POST /api/queries/{id}/remove-proof` → Query; `DELETE /api/queries/{id}` → 204.
- Errors: validation → 400; not found → 404; `ErrNotOpen`, `ErrNotSent`, `ErrQueryResolved`, `ErrCaseDropped`, `cases.ErrOpenQueries` → 409 with the error text after the `pkg: ` prefix, as existing errors do.

- [ ] **Step 1:** `TestQueryRoutes`: add → list shows open_count 1 → move forward 409 → send (multipart with file) → accept → move forward 200; `TestQueryRemoveRoutes`: remove-proof → 200 status open; delete → 204, list empty.
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement and wire.
- [ ] **Step 4:** Full backend suite: PASS apart from the two known failures.
- [ ] **Step 5:** Commit "HTTP: bank query routes".

### Task 9: Web — BankQueries box and dialogs

**Files:** Modify `web/src/api/types.ts`; create `web/src/pages/case/BankQueries.tsx`, `BankQueries.test.tsx`.

**Interfaces:**
- Types `BankQuery`, `QueryEvent`, `CaseQueries` matching Task 8.
- `BankQueries({ caseId, data, readOnly, onChanged, onOpenDocument })` — region "Bank queries"; heading with "<n> open" pill (red border when n > 0); "+ Add query"; per query: body, chip, "Raised <d> · <assignee>", "Due <d>" / "Due <d>, overdue" (red, IST today > due and status open); Open → "Send proof", "Edit", "Remove"; Sent → "Bank accepted", "Bank rejected", "View proof" (`<details>`, closed by default) containing note, file link (calls `onOpenDocument(id)`; "file deleted" when null) and "Remove proof"; "Resolved (n)" `<details>`. readOnly hides all buttons.
- Dialogs (reuse `components/Dialog`): `QueryDialog` (add/edit: "What did the bank ask for?", "Raised on" default today, "Assign to" from `/api/users` with `.catch`, "Due date (optional)"); `SendProofDialog` ("Note", "File or photo", hint "Add a note, a file, or both.", posts multipart; client refuses both empty with that hint as error); `RejectDialog` ("What did the bank say?", button "Reopen query"). Remove / Remove proof / Bank accepted use `window.confirm` with the mockup texts.

- [ ] **Step 1:** Tests: renders open and sent queries with the right buttons; proof hidden until "View proof" clicked; send with neither shows the error and makes no request; send posts FormData to `/api/queries/5/send`; reject posts `{reason}`; accept/remove call the routes after confirm; readOnly shows no buttons; overdue text.
- [ ] **Step 2:** Run `npx vitest run src/pages/case/BankQueries.test.tsx`: FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run: PASS.
- [ ] **Step 5:** Commit "Web: bank queries box".

### Task 10: Web — case page placement, move block, history; docs

**Files:** Modify `web/src/pages/CaseDetail.tsx`, `CaseDetail.test.tsx`, `web/e2e/phone.spec.ts`; docs `docs/00-decisions.md` (§10n), `docs/database-schema.md`, backlog (Tasks 8, 9 built), spec status.

**Interfaces:**
- CaseDetail loads `/api/cases/{id}/queries` with `.catch` (empty on failure). Progress card body becomes a grid: `min-[900px]:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]`, stages left, `BankQueries` right; below 900 px the queries come first (`order-first`).
- When `open_count > 0`, "Move to <next> →" is disabled and an alert shows "Resolve the open bank queries first. Moving back or dropping is still allowed."; "Other stage…" stays enabled (server refuses forward picks; dialog shows the error).
- Timeline gains query events: "Bank query added: <body>", "Query edited: <body>", "Query sent to bank: <body>", "Bank accepted: <body>", "Bank rejected: <body>" (remark = reason), "Query proof removed: <body>", "Query removed: <body>".
- Phone check mocks `/api/cases/1042/queries`.

- [ ] **Step 1:** Tests: queries box rendered inside the Progress region; Move button disabled with the text when open_count 1; enabled when 0; history shows "Query sent to bank: Builder NOC copy".
- [ ] **Step 2:** Run: FAIL.
- [ ] **Step 3:** Implement; update docs.
- [ ] **Step 4:** `npx vitest run`, `npm run build`, phone e2e (`docker run --rm -v "$PWD":/w -w /w --ipc=host mcr.microsoft.com/playwright:v1.48.2-jammy bash -c "npx playwright test; chown -R $(id -u):$(id -g) dist test-results playwright-report"` from `web/`): all PASS; redeploy `docker compose up --build -d db backend web` and check http://localhost:8081 returns 200.
- [ ] **Step 5:** Commit "Web: bank queries on the case page".
