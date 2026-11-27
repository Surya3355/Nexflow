# HDFC Stages and Part Disbursement: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HDFC cases run on HDFC's own 11 stages, banks with part payments record several Part/Full payments, the sanctioned amount overwrites the loan amount, and reports count sanctions and disbursements by stage role across banks.

**Architecture:** Each stage gets a `role` (start, tracking, sanction, disbursement); every rule that today checks a stage code checks the role instead. The HDFC list is a template in Go code that admin applies to a bank from the Banks screen. Payments live in a new `case_disbursements` table used by all banks; `banks.part_payments` decides whether Part is offered.

**Tech Stack:** Go 1.25, pgx/sqlc (`~/go/bin/sqlc generate` in `backend/`), PostgreSQL 16, golang-migrate; React 18 + TypeScript, Tailwind, Vitest.

**Spec:** `docs/superpowers/specs/2026-11-25-hdfc-stages-design.md`

## Global Constraints

- HDFC stages, in order (code / name / role): `received` Received / —; `link_sent` Link sent / start; `application_submitted` Application submitted / tracking; `dovr` DOVR / —; `appraisal` Appraisal / —; `sanctioned` Sanctioned / sanction; `upload_documents` Upload documents / —; `under_fixing` Under fixing / —; `fixed` Fixed / —; `signing` Signing / —; `disbursement` Disbursement / disbursement.
- Standard roles: `received` start, `logged_in` tracking, `sanctioned` sanction, `disbursed` disbursement.
- Standard → HDFC case map: received→link_sent, logged_in→application_submitted, under_sanction→dovr, sanctioned→sanctioned, documentation→upload_documents, disbursed→disbursement.
- Payment kind values `part` / `full`; labels "Part" / "Full". At most one `full` per case.
- Remaining = (sanctioned amount, else loan amount) − total paid, never below 0.
- Sanction: amount required; saving sets `sanctioned_amount` and `loan_amount` to it.
- Remark on a stage move: optional, trimmed, max 500 characters.
- No payout calculation. No stage add/rename/reorder screen. HDFC → Standard switch not offered.
- Permissions: add payment = anyone with `ActionEditRecord`; edit or delete a payment = admin or manager; bank settings (stage list, part payments) = admin only (`ActionManageConfig`).
- Commits only when the owner asks.
- **Commands** (repo root): Backend `docker compose run --rm --build test go test -count=1 ./modules/<pkg>/ -run <Name> -v`; full suite `docker compose run --rm --build test` (`TestAcceptance19` already fails; ignore). Always `--build`. After editing `queries/*.sql` or migrations: `cd backend && ~/go/bin/sqlc generate`. Web: `cd web && npm test -- <file>`, then `npm run build`.

## Review Focus

1. Double tap on "Add payment" with Full → only one Full row, case completed once (unique index + row lock). Test in Task 4.
2. Moving a completed case back and then into Disbursement again → completes again with no payment prompt, no second Full. Test in Task 3.
3. A case at Disbursement whose payments are all deleted by a manager → stays active, remaining = full amount. Test in Task 4.
4. Switching a bank to HDFC while it has dropped and completed cases → every case lands on a mapped HDFC stage, history rows untouched. Test in Task 2.
5. Archived then restored case keeps its payments; tracking clash rules unchanged. Test in Task 6.

---

### Task 1: Migration 000007 (roles, part payments, remark, payments table)

**Files:**
- Create: `backend/migrations/000007_hdfc_stages.up.sql`, `backend/migrations/000007_hdfc_stages.down.sql`
- Test: `backend/migrations/migrate_test.go`

**Interfaces:**
- Produces: `stages.role text NULL CHECK (role IN ('start','tracking','sanction','disbursement'))`, unique index `stages_role_uidx ON stages (bank_id, role) NULLS NOT DISTINCT WHERE role IS NOT NULL`; `banks.part_payments boolean NOT NULL DEFAULT false`; `case_stage_events.remark text NULL CHECK (char_length(remark) <= 500)`; table `case_disbursements(id identity PK, case_id bigint NOT NULL FK cases, amount numeric(14,2) NOT NULL CHECK (amount > 0), paid_on date NOT NULL, kind text NOT NULL CHECK (kind IN ('part','full')), created_at/by, updated_at/by)`, index on `(case_id, paid_on)`, index on `paid_on`, unique `case_disbursements_full_uidx ON (case_id) WHERE kind='full'`. Grants to `nexflow_app` following 000005/000006. Data: roles set on the four standard stages; every case with `disbursed_amount IS NOT NULL` gets one `full` row (paid_on = `disbursement_date`, else the date it reached `disbursed`, else `updated_at::date`). Columns `disbursed_amount`/`disbursement_date` are **dropped** (payments become the only source). Down reverses (restores the two columns from the full rows).

- [ ] **Step 1: Write failing tests** `TestMigration7RolesAndPayments` (standard stages have the four roles; a second `sanction` role on standard stages violates the unique index; an old case with disbursed_amount 500000 has exactly one `full` row of 500000; inserting two `full` rows for one case fails with 23505) and extend `TestDownThenUp` to cover 7.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./migrations/ -run 'Migration7|DownThenUp' -v` → FAIL.
- [ ] **Step 3: Write** both SQL files; run `sqlc generate` and fix compile errors from the dropped columns by deleting `SetCaseDisbursement` and the columns' reads (later tasks replace them).
- [ ] **Step 4: Run** Step 2 → PASS; `go build ./...` in the test container compiles.

### Task 2: Workflows: roles, HDFC template, bank settings

**Files:**
- Modify: `backend/modules/workflows/workflows.go`, `backend/modules/workflows/queries/workflows.sql`
- Create: `backend/modules/workflows/stagelists.go`
- Test: `backend/modules/workflows/stagelists_test.go`

**Interfaces:**
- Produces:
  - `Stage.Role string` ("" for none); `Bank.PartPayments bool`, `Bank.StageList string` ("standard"|"hdfc", derived: has own stages → "hdfc").
  - Role consts `RoleStart, RoleTracking, RoleSanction, RoleDisbursement`.
  - `func StageWithRole(stages []Stage, role string) (Stage, bool)`.
  - `func (s *Service) SetBankStageList(ctx, actor access.User, bankID int64, list string) error` — admin only; `list` must be "hdfc" (`ErrInvalidStageList` otherwise, `ErrBankHasOwnStages` if already own); in one tx locks the bank row, inserts the 11 template stages for the bank, moves every case of the bank (any status) by the constant map, sets `part_payments = true`, audits `bank.stage_list_changed`.
  - `func (s *Service) SetBankPartPayments(ctx, actor access.User, bankID int64, on bool) error` — admin only, audited `bank.part_payments_changed`.
  - `func (s *Service) BankPartPayments(ctx, bankID int64) (bool, error)`.
  - `func (s *Service) StageLists(ctx) ([]StageList, error)` where `StageList{BankID *int64; Name string; Stages []Stage}` — standard list first, then each bank with own stages (for the admin Stages screen).

- [ ] **Step 1: Write failing tests** `TestSetBankStageListCreatesHDFCStages` (11 stages, names/order/roles per Global Constraints, part_payments true), `TestSetBankStageListMovesCases` (cases at each standard stage, including one dropped and one completed, land on mapped stage; `case_stage_events` count unchanged), `TestSetBankStageListAdminOnly`, `TestSetBankStageListTwiceRefused`, `TestSetBankPartPayments`, `TestStageWithRole`.
- [ ] **Step 2: Run** `... ./modules/workflows/ -run 'StageList|PartPayments|StageWithRole' -v` → FAIL.
- [ ] **Step 3: Implement** in `stagelists.go`; template as a Go slice; add queries `SetBankPartPayments`, `LockBank`, `MoveBankCasesStage(bank_id, from_stage_id, to_stage_id)`; add `role` to stage SELECTs and `part_payments` to `ListBanks`; `sqlc generate`.
- [ ] **Step 4: Run** Step 2 → PASS, plus `./modules/workflows/` whole package → PASS.

### Task 3: Stage moves by role (tracking, sanction overwrite, remark, first payment)

**Files:**
- Modify: `backend/modules/cases/stage.go`, `backend/modules/cases/queries/cases.sql`, every caller of `ChangeStage` (tests in `cases`, `archive`, `reports`, `alerts`, `http`)
- Create: `backend/modules/cases/payments.go` (shared payment insert used here and in Task 4)
- Test: `backend/modules/cases/stage_role_test.go`

**Interfaces:**
- Consumes: `workflows.StageWithRole`, `Stage.Role`, `Service.BankPartPayments` (Task 2).
- Produces:
  - `type StageMove struct { ToStageID int64; TrackingNumber *string; Sanction *SanctionDetails; Payment *PaymentInput; Remark string }`
  - `type PaymentInput struct { Amount string; Date time.Time; Kind string }`
  - `func (s *Service) ChangeStage(ctx, actor access.User, caseID int64, m StageMove) (Case, error)` (replaces the old 7-arg form; `DisbursementDetails` removed).
  - `func (s *Service) insertPaymentTx(ctx, q *casesdb.Queries, actor access.User, caseID, bankID int64, p PaymentInput) (completed bool, err error)` — refuses `part` when bank part payments off (`ErrPartNotAllowed`), refuses second full (`ErrAlreadyFullyPaid`, also mapped from 23505), refuses amount ≤ 0, kind not part/full (`ErrInvalidPaymentKind`).
  - Errors: `ErrTrackingRequired` text → "cases: tracking number is required at this stage"; `ErrSanctionAmountRequired`; `ErrPaymentRequired` ("cases: enter the first payment to move to disbursement"); `ErrRemarkTooLong`.
- Rules: tracking-role target needs a tracking number; sanction-role target needs `Sanction` with an amount and writes `sanctioned_amount` and `loan_amount` (audit details carry `old_loan_amount`); disbursement-role target with no Full on record needs `Payment` if the case has no payments yet; status becomes `completed` only when a Full row exists after the move; remark saved on the stage event.

- [ ] **Step 1: Write failing tests** `TestTrackingRoleOnHDFCApplicationSubmitted`, `TestSanctionOverwritesLoanAmount` (loan 5000000, sanction 4500000 → both 4500000), `TestSanctionNeedsAmount`, `TestRemarkSavedAndOptional`, `TestRemarkTooLong` (501 chars), `TestDisbursementNeedsFirstPayment`, `TestPartPaymentKeepsCaseActive` (HDFC), `TestPartRefusedWhenBankOff` (standard bank), `TestFullPaymentCompletes`, `TestMoveBackAndReenterRecompletesWithoutPrompt` (Review Focus 2).
- [ ] **Step 2: Run** `... ./modules/cases/ -run 'Role|Sanction|Remark|Disbursement|Payment|Reenter' -v` → FAIL.
- [ ] **Step 3: Implement**; replace `stageLoggedIn`/`stageDisbursed` checks with roles; add query `SetCaseLoanAmountFromSanction`, `InsertCaseStageEventAt` gains `remark`; update all callers to `StageMove{...}` (mechanical).
- [ ] **Step 4: Run** whole `./modules/cases/` → PASS.

### Task 4: Payments on an existing case

**Files:**
- Modify: `backend/modules/cases/payments.go`, `backend/modules/cases/queries/cases.sql`, `backend/modules/cases/list.go` (Case gets payment summary)
- Test: `backend/modules/cases/payments_test.go`

**Interfaces:**
- Produces:
  - `type Payment struct { ID int64; Amount string; PaidOn time.Time; Kind string; CreatedBy int64; CreatedAt time.Time }`
  - `type PaymentSummary struct { Payments []Payment; TotalPaid string; Remaining string; FullyPaid bool; OverPaid bool }`
  - `func (s *Service) Payments(ctx, caseID int64) (PaymentSummary, error)`
  - `func (s *Service) AddPayment(ctx, actor, caseID int64, p PaymentInput) (Case, error)` — case must be at its disbursement-role stage and not dropped (`ErrNotAtDisbursement`, `ErrCaseDropped`); locks the case; Full completes it and sets purge date like today.
  - `func (s *Service) UpdatePayment(ctx, actor, caseID, paymentID int64, p PaymentInput) (Case, error)` and `DeletePayment(ctx, actor, caseID, paymentID int64) (Case, error)` — admin/manager only (`access.ErrForbidden`); audited with before/after; removing or changing the only Full to Part sets a completed case back to `active` and clears the purge date.
  - `Case.LoanAmount` unchanged; `Case.PartPayments bool` (from bank) for the UI.

- [ ] **Step 1: Write failing tests** `TestAddPaymentAtDisbursementOnly`, `TestSummaryRemainingUsesSanctioned` (sanctioned 3500000, parts 1000000+1500000 → remaining 1000000), `TestRemainingFallsBackToLoanAmount`, `TestOverPaidFlagNotBlocked` (remaining "0", OverPaid true), `TestConcurrentFullOnlyOnce` (two goroutines add Full → one succeeds, other `ErrAlreadyFullyPaid`), `TestEmployeeCannotEditOrDelete`, `TestDeleteFullReopensCase`, `TestDeleteAllPaymentsLeavesActive` (Review Focus 3), `TestPaymentChangesAudited`.
- [ ] **Step 2: Run** `... ./modules/cases/ -run 'Payment|Remaining|OverPaid|Concurrent|Reopens' -v` → FAIL.
- [ ] **Step 3: Implement**; queries `ListCasePayments`, `UpdateCasePayment`, `DeleteCasePayment`, `CaseHasFullPayment`.
- [ ] **Step 4: Run** whole `./modules/cases/` → PASS.

### Task 5: New cases start by role; stuck alert and attention by role

**Files:**
- Modify: `backend/modules/cases/create.go`, `backend/modules/cases/queries/cases.sql:183`, `backend/modules/alerts` stuck query if it filters by code
- Test: `backend/modules/cases/create_role_test.go`

**Interfaces:**
- Produces: `NewCase.StartAt` accepts "" (bank's start stage), "start", or "tracking"; old values "received"/"logged_in" still accepted as aliases. For a bank with own stages only the start stage is allowed (`ErrInvalidStart` for "tracking"). New HDFC case has one stage event, at Link sent. `CreateFromLead` uses the same path.
- Stuck/attention: skip cases whose current stage has role `disbursement` (replaces `stage_code <> 'disbursed'`).

- [ ] **Step 1: Write failing tests** `TestHDFCCaseStartsAtLinkSent`, `TestHDFCCaseTrackingStartRefused`, `TestStandardStartChoicesUnchanged`, `TestLeadToHDFCCaseStartsAtLinkSent`, `TestPartPaidCaseNotStuck`.
- [ ] **Step 2: Run** `... ./modules/cases/ ./modules/leads/ ./modules/alerts/ -run 'LinkSent|TrackingStart|StartChoices|NotStuck' -v` → FAIL.
- [ ] **Step 3: Implement**; move stuck filter to a join on `stages.role`.
- [ ] **Step 4: Run** `./modules/cases/ ./modules/leads/ ./modules/alerts/` → PASS.

### Task 6: Reports by role, disbursement report; archive carries payments

**Files:**
- Modify: `backend/modules/reports/dashboard.go`, `backend/modules/reports/queries/reports.sql`, `backend/modules/archive/queries/archive.sql`, `backend/modules/archive/restore.go`, archive's delete path
- Create: `backend/modules/reports/disbursements.go`
- Test: `backend/modules/reports/disbursements_test.go`, `backend/modules/archive/payments_test.go`

**Interfaces:**
- Produces:
  - Sanctions: `StageHits` joins on `s.role = 'sanction'`; amount = `sanctioned_amount`.
  - Disbursements tile: `StageCount{Count: cases with ≥1 payment paid_on in period, Amount: sum of those payments, Dropped: of those, dropped}`.
  - `CountStageMoves` excludes each case's first stage event (not the `received` code).
  - `Dashboard.CasesByStage` becomes `[]StageTotal{BankID *int64; BankName string; StageID int64; StageName string; SortOrder int; Total int}` ordered by bank name (standard-stage banks under "Standard stages"), then sort order.
  - `type DisbursementRow struct { CaseID int64; Customer, Mobile, Bank, Project, Tracking, Assignee string; PaidInPeriod, TotalPaid, Sanctioned, Remaining string; LastPaidOn time.Time; Kind string /* "Part"|"Full" */; Dropped bool }`
  - `func (s *Service) DisbursementRows(ctx, p Period, bank, project pgtype.Int8) ([]DisbursementRow, error)` — one row per case with a payment in the period.
  - Archive snapshot child `payments` (from `case_disbursements`), deleted on archive, restored on restore; old snapshots without it restore with none.

- [ ] **Step 1: Write failing tests** `TestSanctionsCountHDFCStage`, `TestDisbursementTileSumsPaymentsInPeriod` (parts on 05 Nov and 18 Nov, period Nov → count 1, amount = both), `TestDisbursementRowsOneRowPerCase`, `TestDroppedCaseWithPaymentCountedAndMarked`, `TestStageMovesSkipFirstEvent`, `TestCasesByStageGroupedByBank`, `TestArchiveRestoreKeepsPayments` (Review Focus 5), `TestRestoreOldSnapshotWithoutPayments`.
- [ ] **Step 2: Run** `... ./modules/reports/ ./modules/archive/ -v` → FAIL.
- [ ] **Step 3: Implement**; `sqlc generate`.
- [ ] **Step 4: Run** Step 2 → PASS.

### Task 7: HTTP

**Files:**
- Modify: `backend/modules/http/handlers.go`, `backend/modules/http/router.go`, `backend/modules/http/errors.go`
- Test: `backend/modules/http/hdfc_routes_test.go`

**Interfaces:**
- Produces:
  - `GET /api/config/banks` items add `part_payments`, `stage_list`.
  - Stage JSON adds `role` ("" when none).
  - `PATCH /api/admin/banks/{id}` body `{ "stage_list"?: "hdfc", "part_payments"?: bool }` → 204.
  - `GET /api/admin/stage-lists` → `[{bank_id, name, stages: [...]}]`.
  - `POST /api/cases/{id}/stage` body adds `remark` and `payment: {amount, date, kind}`; `disbursement` field removed; `sanction.amount` required.
  - `GET /api/cases/{id}` adds `part_payments`, `payments: [{id, amount, paid_on, kind}]`, `total_paid`, `remaining`, `fully_paid`, `over_paid`.
  - `POST /api/cases/{id}/payments`, `PATCH /api/cases/{id}/payments/{paymentID}`, `DELETE /api/cases/{id}/payments/{paymentID}` → updated case JSON.
  - History items add `remark`.
  - Dashboard `cases_by_stage` becomes the `[]StageTotal` array (`bank_id, bank_name, stage_id, stage_name, sort_order, total`).
  - `GET /api/reports/disbursements?from&to&bank_id&project_id` → `{columns, rows}`; `GET /api/reports/disbursements.csv` same rows. Columns: Case ID, Customer, Mobile, Bank, Project, Tracking no., Assigned to, Paid in period, Total paid, Last payment, Type, Sanctioned, Remaining, Status.
  - New errors → 400 (validation), 403 (forbidden), 409 (`ErrAlreadyFullyPaid`, `ErrBankHasOwnStages`).

- [ ] **Step 1: Write failing tests** one per new/changed route (status code + key JSON fields), including employee 403 on PATCH/DELETE payment and on PATCH bank.
- [ ] **Step 2: Run** `... ./modules/http/ -v` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** full backend suite → PASS (except `TestAcceptance19`).

### Task 8: Web: case page, move dialog, Add case

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/pages/case/MoveStageDialog.tsx`, `web/src/pages/CaseDetail.tsx`, `web/src/components/AddCaseDialog.tsx`, `web/src/pages/home/CaseTable.tsx:15`
- Create: `web/src/pages/case/Disbursements.tsx`
- Test: `web/src/pages/case/MoveStageDialog.test.tsx`, `web/src/pages/case/Disbursements.test.tsx`, existing `CaseDetail.test.tsx`, `AddCaseDialog` tests

**Interfaces:**
- Consumes: Task 7 JSON.
- Produces: `Stage.role`; `Bank.part_payments`, `Bank.stage_list`; `CaseData.payments/total_paid/remaining/fully_paid/over_paid/part_payments`.
- Behaviour:
  - MoveStageDialog switches on `to.role` (tracking → tracking field; sanction → form, amount required, message "Enter the sanctioned amount."; disbursement → payment amount, date, and Part/Full radio only when `part_payments`, required unless the case already has payments). Optional "Remark" textarea on every move, maxLength 500.
  - `Disbursements` block on CaseDetail when the case has payments or is at the disbursement stage: rows "05 Nov 2026 · ₹10,00,000 · Part", lines "Total paid", "Remaining"; warning "Total paid is more than the sanctioned amount." when `over_paid`; "Add payment" button only at the disbursement stage and not `fully_paid`; Edit/Delete per row only for admin/manager.
  - AddCaseDialog: "Start at" select shown only for banks with `stage_list === "standard"`; HDFC-list banks send `start_at: ""`.
  - CaseTable "done" = `status === "completed"`.
  - Case history shows the remark under the stage line when present.

- [ ] **Step 1: Write failing tests** for each behaviour above (role-driven fields, Part hidden for SBI, remark sent, Add payment hidden when fully paid, Edit hidden for employee, over-paid warning, start select hidden for HDFC).
- [ ] **Step 2: Run** `cd web && npm test -- MoveStageDialog Disbursements CaseDetail AddCaseDialog` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** Step 2 → PASS; `npm run build` → OK.

### Task 9: Web: admin and reports

**Files:**
- Modify: `web/src/pages/admin/BanksAndLists.tsx`, `web/src/pages/admin/Stages.tsx`, `web/src/pages/Reports.tsx`
- Test: `BanksAndLists.test.tsx`, `Stages.test.tsx`, `Reports.test.tsx`

**Interfaces:**
- Consumes: Task 7 routes.
- Behaviour:
  - Banks table: "Stage list" column shows "Standard" or "HDFC"; for a Standard bank a "Use HDFC stages" button with confirm text "Move this bank's cases to the HDFC stages? This cannot be undone." → PATCH `stage_list`. "Part payments" checkbox per bank → PATCH `part_payments`.
  - Stages screen: one section per stage list from `/api/admin/stage-lists`, each with the existing "Requires documents" checkbox.
  - Reports: "Cases by stage" grouped under a bank heading; new "Disbursements" table (one row per case, Type column "Part"/"Full", Dropped marked) with "Download CSV" linking `/api/reports/disbursements.csv?…`.

- [ ] **Step 1: Write failing tests** for each behaviour above.
- [ ] **Step 2: Run** `cd web && npm test -- BanksAndLists Stages Reports` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** whole web suite → PASS; `npm run build` → OK.

### Task 10: Final check and local redeploy

- [ ] **Step 1:** Full backend suite and full web suite → PASS (except `TestAcceptance19`); phone e2e in the Playwright image → PASS.
- [ ] **Step 2:** `docker compose up --build -d db backend web`; confirm migration 7 applied; on the local app, switch the test HDFC bank to HDFC stages and walk one case from Link sent to two part payments and a Full.
- [ ] **Step 3:** Update `docs/database-schema.md` (new columns and table) and mark Task 10 built in the round 15 backlog.
