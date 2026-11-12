# Nexflow v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Commits:** do not commit or push unless the owner asks (`agent.md` §2, `CLAUDE.md`). Each task ends with a checkpoint instead of a commit.

**Goal:** Build Nexflow v1, an internal web tool that tracks customers and home-loan cases from first contact to disbursement for a small DSA business.

**Architecture:** One Go backend (modular monolith under `backend/modules/`) exposing a JSON API with chi, backed by PostgreSQL. A React single-page app in `web/` calls the API. Authorization is checked on the server for every request. Stages, banks and dropdowns are data, not code.

**Tech Stack:** Go, chi, PostgreSQL 15+ (local development uses 16), `sqlc`, `golang-migrate`, `alexedwards/scs` with PostgreSQL store, `argon2id`, React with TypeScript, Vite, Vitest, React Testing Library, Playwright. Tailwind CSS for styling. Frontend tool names are my choice and need owner confirmation before the frontend tasks start.

**Spec:** `docs/superpowers/specs/2026-11-11-nexflow-v1-design.md` (approved 2026-11-12). Business rules: `docs/00-decisions.md`. Schema: `docs/database-schema.md`. Stack: `docs/tech-stack.md`.

**Blocked, not in this plan's critical path:** hosting provider, real SMTP provider and sender domain, file storage location, production PostgreSQL version, off-server backup copy. Tasks that need them are marked **[hosting-blocked]**. Everything else can be built and tested locally.

## Global Constraints

- Web app, works well in a phone browser, no horizontal scroll (spec §12).
- Server-side authorization on every request. The React app is never trusted for access control (spec §12, `agent.md` §3).
- Passwords hashed with `argon2id`. Server-side sessions stored in PostgreSQL (`tech-stack.md` §1).
- Documents: PDF, JPG, PNG only, max 10 MB per file, only the latest version kept, private, short-lived links only (spec §4.4).
- Documents deleted automatically 1 year after the case is disbursed or dropped (spec §4.4).
- Archived copies purged 1 year after deletion. Audit log kept permanently (spec §4.5, §4.6).
- Daily backup, last 30 days kept (spec §12).
- Data hosted in an India cloud region (spec §12).
- Mobile number stored as exactly 10 digits (round 12).
- Business records never physically deleted except through the archive path (`database-schema.md` conventions).
- Stage and assignment history is append-only.
- Schema changes only through migrations (`tech-stack.md` §3).
- No technology beyond the stack without owner approval (`tech-stack.md` §3).
- Secrets never in git. `.env.example` lists names only.

## Review Focus

Inputs and conditions the spec implies that no tests would otherwise exercise. Each is pinned by a test in the task that owns the code.

1. **Pasted mobile number with +91, spaces, dashes or a leading 0** → stored as 10 digits, or refused with a clear message. (Task 4)
2. **Two staff submit the same new mobile or tracking number at the same moment** → exactly one succeeds, the other gets the duplicate warning, not a server error. (Tasks 5, 9)
3. **A file named `.pdf` or `.jpg` that is really HTML or an executable** → refused by content check, not extension. (Task 15)
4. **A download link that was tampered with, or is past its expiry** → refused, no file returned. (Task 16)
5. **A user deactivated while logged in** → their existing session stops working on the next request. (Task 7)

Also covered: "today" for dashboard counts uses India time (Asia/Kolkata), not the server's UTC date (Task 22).

---

## Phase 0: Foundation

### Task 1: Backend skeleton and health endpoint

**Files:**
- Create: `backend/go.mod`, `backend/cmd/nexflow/main.go`, `backend/internal/platform/config/config.go`, `backend/internal/platform/httpserver/router.go`, `backend/internal/platform/httpserver/router_test.go`, `.env.example`

**Interfaces:**
- Produces: `config.Load() (Config, error)` reading `NEXFLOW_DB_URL`, `NEXFLOW_HTTP_ADDR` (default `:8080`). `httpserver.NewRouter() http.Handler` with `GET /healthz`.

- [ ] **Step 1: Write the failing test** in `router_test.go`: `GET /healthz` returns 200 and body `ok`.
- [ ] **Step 2: Run** `cd backend && go test ./internal/platform/httpserver/...` → FAIL (package missing).
- [ ] **Step 3: Implement** the router with chi, the config loader, and `cmd/nexflow/main.go` that starts the server. Add `.env.example` with variable names only.
- [ ] **Step 4: Run** the same test → PASS.
- [ ] **Checkpoint:** `go vet ./...` clean.

Note: `router.go` must be a module-friendly package. Later tasks mount module routes under `/api`.

### Task 2: Test database harness

**Files:**
- Create: `backend/internal/platform/testdb/testdb.go`, `backend/internal/platform/testdb/testdb_test.go`

**Interfaces:**
- Produces: `testdb.New(t *testing.T) *pgxpool.Pool`. Creates a fresh, uniquely named database on the PostgreSQL server in `NEXFLOW_TEST_DB_URL`, runs all migrations, and drops the database when the test ends. Used by every later test that touches the database.

- [ ] **Step 1: Write the failing test:** `testdb.New` returns a pool, and `SELECT 1` succeeds.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** the helper. Migrations are loaded in Task 3, so this test stubs the migration call with an empty migration set until Task 3 is done.
- [ ] **Step 4: Run** → PASS.
- [ ] **Checkpoint:** a local PostgreSQL 16 instance is running and `NEXFLOW_TEST_DB_URL` is set. Tests that need it skip with a clear message if it is not set. Do not silently pass.

### Task 3: Schema migrations

**Files:**
- Create: `backend/migrations/000001_initial.up.sql`, `backend/migrations/000001_initial.down.sql`, `backend/migrations/migrate.go`, `backend/migrations/migrate_test.go`, `backend/sqlc.yaml`

**Interfaces:**
- Produces: `migrations.Up(db *pgxpool.Pool) error` and `migrations.Down(db *pgxpool.Pool) error`.

The up migration is the schema in `docs/database-schema.md` §8, with these changes from round 11 and 12:
- `stages.requires_document boolean not null default false`.
- `documents.stage_id bigint not null references stages(id)`.
- `customers.mobile` has `check (mobile ~ '^[0-9]{10}$')`.
- The seed data for the standard stages (`bank_id` null) in this order: `received` "Received", `logged_in` "Logged in", `under_sanction` "Under sanction", `sanctioned` "Sanctioned", `documentation` "Documentation", `disbursed` "Disbursed". All have `requires_document = false`.
- The seed data for `dropdown_values` with `category = 'drop_reason'`, in this order: "Customer not interested", "Not eligible", "Rejected by bank", "Customer chose another bank or DSA", "Customer unreachable", "Duplicate or already processed", "Other".

- [ ] **Step 1: Write the failing tests** in `migrate_test.go`:
  - `TestMigrateUpCreatesTables`: after `Up`, each table in `database-schema.md` §8 exists.
  - `TestMigrateDownThenUpIsClean`: `Down` then `Up` succeeds.
  - `TestStandardStagesSeeded`: six standard stages, in the order above, `bank_id` null.
  - `TestMobileCheckRejectsNonTenDigits`: inserting `+919876543210` into `customers` fails.
  - `TestTrackingNumberUniqueAmongLiveCases`: two cases with the same non-null tracking number fail.
- [ ] **Step 2: Run** `go test ./migrations/...` → FAIL.
- [ ] **Step 3: Write the SQL files** and `migrations.go`, using `golang-migrate` with an embedded source.
- [ ] **Step 4: Run** → PASS.
- [ ] **Step 5: Generate queries setup:** `sqlc.yaml` pointing at `migrations/` for schema and `modules/*/queries/` for queries. `sqlc generate` runs with no errors.
- [ ] **Checkpoint:** `go test ./...` passes.

---

## Phase 1: Access

### Task 4: Mobile number normalization

**Files:**
- Create: `backend/modules/customers/mobile.go`, `backend/modules/customers/mobile_test.go`

**Interfaces:**
- Produces: `customers.NormalizeMobile(input string) (string, error)`. Returns exactly 10 digits, or `ErrInvalidMobile`.

- [ ] **Step 1: Write the failing table test** `TestNormalizeMobile` with these cases:
  - `"9876543210"` → `"9876543210"`
  - `"+91 98765 43210"` → `"9876543210"`
  - `"+91-98765-43210"` → `"9876543210"`
  - `"+919876543210"` → `"9876543210"`
  - `" 98765 43210 "` → `"9876543210"`
  - `"09876543210"` → `ErrInvalidMobile` (11 digits after strip; a leading 0 is not stripped, so the user must enter the 10 digits)
  - `"12345"` → `ErrInvalidMobile`
  - `""` → `ErrInvalidMobile`
  - `"98765abc10"` → `ErrInvalidMobile`
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** Remove a leading `+91` (after trimming), then remove all spaces, dashes and other non-digits, then check the result is exactly 10 digits.
- [ ] **Step 4: Run** → PASS.

### Task 5: Permission matrix

**Files:**
- Create: `backend/modules/access/permissions.go`, `backend/modules/access/permissions_test.go`

**Interfaces:**
- Produces: `type Role string` with `RoleAdmin`, `RoleManager`, `RoleEmployee`. `type Action string` with one constant per row of spec §6 (`ActionViewAll`, `ActionAddRecord`, `ActionEditRecord`, `ActionUploadDocument`, `ActionDropRecord`, `ActionAssignCase`, `ActionHardDelete`, `ActionRestoreArchive`, `ActionManageConfig`, `ActionManageUsers`, `ActionViewReports`). `access.Can(role Role, action Action) bool`.

- [ ] **Step 1: Write the failing table test** `TestPermissionMatrix` with one row per (role, action) pair from spec §6. Expected values come from the table in §6 exactly. Example rows: `Can(RoleEmployee, ActionHardDelete) == false`, `Can(RoleManager, ActionRestoreArchive) == false`, `Can(RoleEmployee, ActionAssignCase) == true`, `Can(RoleManager, ActionManageUsers) == false`.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** as a single map, so the table in the spec and the code can be compared line by line.
- [ ] **Step 4: Run** → PASS.
- [ ] **Note:** the "every role is refused" rule is enforced in Tasks 6 to 8 and in each later endpoint's tests, which call `Can`, not in this table.

### Task 6: Users and admin-created accounts

**Files:**
- Create: `backend/modules/access/users.go`, `backend/modules/access/users_test.go`, `backend/modules/access/queries/users.sql`, `backend/modules/access/password.go`, `backend/modules/access/password_test.go`

**Interfaces:**
- Produces: `access.HashPassword(plain string) (string, error)` and `access.VerifyPassword(hash, plain string) (bool, error)` (argon2id, encoded hash). `access.CreateUser(ctx, actor User, in NewUser) (User, error)`. `NewUser{Username, FullName, Role, Password}`.
- Consumes: `access.Can` (Task 5), audit writer (Task 24, stub until then — write audit rows through an interface `history.Recorder` defined in Task 24; in this task use a no-op recorder).

- [ ] **Step 1: Write failing tests:**
  - `TestPasswordRoundTrip`: `VerifyPassword(HashPassword("x"), "x")` is true; a different password is false.
  - `TestCreateUserRequiresAdmin`: a manager and an employee each get `ErrForbidden`. An admin succeeds.
  - `TestUsernameIsLowercasedAndUnique`: `"Surya"` is stored as `"surya"`; creating a second `"SURYA"` fails.
  - `TestRoleMustBeKnown`: role `"owner"` is refused.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** the functions. The admin check calls `access.Can(actor.Role, ActionManageUsers)`.
- [ ] **Step 4: Run** → PASS.

### Task 7: Login, logout and sessions

**Files:**
- Create: `backend/modules/access/session.go`, `backend/modules/access/login.go`, `backend/modules/access/login_test.go`, `backend/modules/access/middleware.go`, `backend/modules/access/middleware_test.go`

**Interfaces:**
- Produces: `POST /api/login` (body `{username, password}`), `POST /api/logout`, middleware `access.RequireUser(next http.Handler) http.Handler` that loads the user on every request and refuses inactive users. Session store: `alexedwards/scs` with the PostgreSQL store.

- [ ] **Step 1: Write failing tests:**
  - `TestLoginWrongPasswordReturns401`.
  - `TestLoginCorrectPasswordSetsSessionCookie`.
  - `TestInactiveUserCannotLogin`.
  - `TestDeactivatedUserLosesExistingSession`: log in, set `is_active = false`, the next request with the same cookie returns 401. (Review Focus item 5.)
  - `TestLogoutInvalidatesSession`.
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.** Login gives the same error for an unknown username and a wrong password. The middleware reads `is_active` from the database on each request, not from the session.
- [ ] **Step 4: Run** → PASS.

### Task 8: Change own password

**Files:**
- Modify: `backend/modules/access/users.go`
- Create: `backend/modules/access/change_password_test.go`

**Interfaces:**
- Produces: `POST /api/me/password` (body `{current, new}`). Sets `password_changed_at`.

- [ ] **Step 1: Write failing tests:**
  - `TestChangePasswordNeedsCurrentPassword`: a wrong current password returns 401 and changes nothing.
  - `TestChangePasswordOldPasswordStopsWorking`.
  - `TestChangePasswordSetsPasswordChangedAt`.
  - `TestChangePasswordWritesAuditEntry`: `user.password_changed` is written.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 2: Configuration

### Task 9: Banks, dropdown values and stages (admin only)

**Files:**
- Create: `backend/modules/workflows/banks.go`, `backend/modules/workflows/stages.go`, `backend/modules/workflows/dropdowns.go`, `backend/modules/workflows/workflows_test.go`

**Interfaces:**
- Produces: `workflows.StagesFor(ctx, bankID int64) ([]Stage, error)`: the bank's own active stages, or the standard stages when the bank has none. `Stage{ID, BankID *int64, Code, Name, SortOrder, RequiresDocument bool}`. Admin endpoints for banks, stages and dropdown values under `/api/admin/...`.

- [ ] **Step 1: Write failing tests:**
  - `TestBankWithoutOwnStagesUsesStandardStages`.
  - `TestBankWithOwnStagesUsesThemOnly`.
  - `TestManagerCannotChangeStages`, `TestEmployeeCannotChangeStages`: both get 403.
  - `TestSetRequiresDocumentOnStage`: admin sets `requires_document = true` on a stage, and `StagesFor` returns it.
  - `TestDropReasonListIncludesOther`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 3: Customers and cases

### Task 10: Create customer, with duplicate warning

**Files:**
- Create: `backend/modules/customers/create.go`, `backend/modules/customers/create_test.go`, `backend/modules/customers/queries/customers.sql`

**Interfaces:**
- Produces: `customers.Create(ctx, actor User, in NewCustomer) (CreateResult, error)`. `NewCustomer{Name, Mobile, Email}`. `CreateResult{Customer, Existing *Customer}`. `Existing` is set when the mobile already belongs to a live customer, and no new customer is created.
- Consumes: `NormalizeMobile` (Task 4), `access.Can(…, ActionAddRecord)`, audit recorder.

- [ ] **Step 1: Write failing tests:**
  - `TestCreateCustomerNewMobile`: status `active`, mobile stored as 10 digits.
  - `TestCreateCustomerExistingMobileWarnsAndDoesNotDuplicate`: `Existing` is set, the customer count stays at 1.
  - `TestCreateCustomerPastedMobileIsNormalized`: input `"+91 98765 43210"` finds the existing `"9876543210"` customer. (Review Focus item 1.)
  - `TestConcurrentCreateSameMobileCreatesOneCustomer`: 5 goroutines submit the same new mobile. Exactly one customer is created, and the rest return `Existing`, not an error. (Review Focus item 2.) The unique index on `mobile` is the final guard.
  - `TestCreateCustomerRejectsInvalidMobile`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 11: Create case for a customer

**Files:**
- Create: `backend/modules/cases/create.go`, `backend/modules/cases/create_test.go`, `backend/modules/cases/queries/cases.sql`

**Interfaces:**
- Produces: `cases.Create(ctx, actor User, in NewCase) (Case, error)`. `NewCase{CustomerID, BankID, LoanAmount (decimal string), AssigneeID}`. Result has `CurrentStageID` set to the bank's `received` stage.
- Consumes: `workflows.StagesFor`, `access.Can(…, ActionAddRecord)`, `history.Recorder`.

- [ ] **Step 1: Write failing tests:**
  - `TestNewCaseStartsAtReceived`.
  - `TestNewCaseWritesStageAndAssignmentEvents`: one `case_stage_events` row and one `case_assignment_events` row with `from_user_id` null.
  - `TestLoanAmountMustBePositive`: `"0"` and `"-5"` refused.
  - `TestCustomerCanHaveSeveralCases`.
  - `TestNewCaseWritesAuditEntry`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 12: Stage change with tracking number and history

**Files:**
- Create: `backend/modules/cases/stage.go`, `backend/modules/cases/stage_test.go`

**Interfaces:**
- Produces: `cases.ChangeStage(ctx, actor User, caseID int64, toStageID int64, trackingNumber *string, sanction *SanctionDetails, disbursement *DisbursementDetails) (Case, error)`. Runs in one transaction: updates `current_stage_id` and `status`, and appends to `case_stage_events`.
- Consumes: tracking number uniqueness (Task 13), `access.Can(…, ActionEditRecord)`.

- [ ] **Step 1: Write failing tests:**
  - `TestForwardMoveAddsDatedEvent`.
  - `TestBackwardMoveAllowedAndRecorded` (spec §5).
  - `TestMoveToLoggedInRequiresTrackingNumber`.
  - `TestMoveToDisbursedSetsStatusCompleted`.
  - `TestStageHistoryNeverOverwritten`: after two moves there are 3 events, earlier ones unchanged.
  - `TestEmployeeCanChangeStage` (all staff can edit, spec §6).
  - `TestEachStageChangeWritesAuditEntry`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 13: Tracking number uniqueness among live cases

**Files:**
- Modify: `backend/modules/cases/stage.go`
- Create: `backend/modules/cases/tracking_test.go`

**Interfaces:**
- Consumes: the partial unique index from Task 3.

- [ ] **Step 1: Write failing tests:**
  - `TestDuplicateTrackingNumberRefused`: a second live case with the same number returns `ErrDuplicateTracking`, not a 500. (Review Focus item 2.)
  - `TestArchivedCaseDoesNotBlockTrackingNumber`: after the first case is archived (Task 19), the same number can be used. This test is completed in Task 19.
  - `TestTrackingNumberIsTrimmed`: `" 12345 "` is stored as `"12345"`.
- [ ] **Step 2–4:** run → FAIL; implement (map the unique-violation error to `ErrDuplicateTracking`); run → PASS.

### Task 14: Assign and reassign

**Files:**
- Create: `backend/modules/cases/assign.go`, `backend/modules/cases/assign_test.go`

**Interfaces:**
- Produces: `cases.Assign(ctx, actor User, caseID, toUserID int64) (Case, error)`. Appends to `case_assignment_events` with `from_user_id`. Queues an `assigned` alert (Task 27).
- Consumes: `access.Can(…, ActionAssignCase)`.

- [ ] **Step 1: Write failing tests:**
  - `TestAnyStaffCanReassignAtAnyStage`: an employee reassigns a case at Disbursed (spec §6, round 8).
  - `TestReassignmentKeepsHistory`: `from` and `to` recorded.
  - `TestCannotAssignToInactiveUser`.
  - `TestAssignQueuesAssignedAlert`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 15: Follow-up date and edits

**Files:**
- Create: `backend/modules/cases/edit.go`, `backend/modules/cases/edit_test.go`, `backend/modules/customers/edit.go`, `backend/modules/customers/edit_test.go`

**Interfaces:**
- Produces: `cases.Update(ctx, actor, caseID, CaseFields)` and `customers.Update(ctx, actor, customerID, CustomerFields)`. `follow_up_date` optional (spec §4.2). Editable fields exclude `status`, `current_stage_id`, `assignee_id`, which have their own actions.

- [ ] **Step 1: Write failing tests:**
  - `TestFollowUpDateOptional`: no date means no alert is scheduled.
  - `TestAllStaffCanEditCustomerAndCase`.
  - `TestEditDoesNotChangeStageOrAssignee`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 4: Dropping, archive and audit

### Task 16: Audit recorder

**Files:**
- Create: `backend/modules/history/audit.go`, `backend/modules/history/audit_test.go`, `backend/migrations/000002_audit_grants.up.sql`

**Interfaces:**
- Produces: `history.Recorder` interface with `Record(ctx, Entry) error`. `Entry{ActorUserID *int64, Action, EntityType string, EntityID *int64, Details any}`. The grant migration gives the application role `INSERT, SELECT` only on `audit_log`.

- [ ] **Step 1: Write failing tests:**
  - `TestAuditRowWritten`.
  - `TestApplicationRoleCannotUpdateOrDeleteAudit`: `UPDATE` and `DELETE` on `audit_log` as the app role fail.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS. Replace the no-op recorder used in Tasks 6 to 15 with this one.

### Task 17: Drop customer or case, with reason

**Files:**
- Create: `backend/modules/cases/drop.go`, `backend/modules/cases/drop_test.go`, `backend/modules/customers/drop.go`, `backend/modules/customers/drop_test.go`

**Interfaces:**
- Produces: `cases.Drop(ctx, actor, caseID, DropInput{ReasonID, Remark})` and `customers.Drop(ctx, actor, customerID, DropInput)`. `DropInput.Remark` is required when the reason is "Other".
- Consumes: `access.Can(…, ActionDropRecord)`.

- [ ] **Step 1: Write failing tests:**
  - `TestDropRequiresReason`.
  - `TestDropOtherRequiresRemark`.
  - `TestDroppedCaseStaysVisibleWithStatus`.
  - `TestDroppingCustomerDropsAllItsCases`: each case `status = dropped`, `dropped_via_customer = true`, same reason.
  - `TestDroppingCustomerLeavesAlreadyDroppedCaseUnchanged` (my call, see open point A).
  - `TestDropKeepsStageHistory`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 18: Hard delete to archive

**Files:**
- Create: `backend/modules/archive/delete.go`, `backend/modules/archive/delete_test.go`, `backend/modules/archive/queries/archive.sql`

**Interfaces:**
- Produces: `archive.DeleteCase(ctx, actor, caseID) error`, `archive.DeleteCustomer(ctx, actor, customerID) error`. A case snapshot includes a copy of its customer's details. `DeleteCustomer` is refused while the customer has live cases, and its archive snapshot lists the archived case IDs. Each writes a full JSON snapshot to `archive` with `purge_after = archived_at + 1 year`, deletes the live rows, and writes `customer.deleted` or `case.deleted` to the audit log. Both run in one transaction.
- Consumes: `access.Can(…, ActionHardDelete)`.

- [ ] **Step 1: Write failing tests:**
  - `TestEmployeeCannotHardDelete` (403).
  - `TestManagerCanHardDeleteCase`.
  - `TestDeleteCaseWritesArchiveSnapshotWithPurgeDate`: `purge_after` is 1 year after `archived_at`.
  - `TestCustomerWithCasesCannotBeDeleted`: returns `ErrCustomerHasCases` (spec §4.1, §13 check 8).
  - `TestDeleteWritesAuditEntryThatSurvivesPurge`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 19: Restore from archive

**Files:**
- Create: `backend/modules/archive/restore.go`, `backend/modules/archive/restore_test.go`

**Interfaces:**
- Produces: `archive.Restore(ctx, actor, archiveID int64) (RestoredRef, error)`. The restored record has `status = dropped` and needs a reason, from the stored snapshot or set now (spec §6 round 6).
- Consumes: `access.Can(…, ActionRestoreArchive)`.

- [ ] **Step 1: Write failing tests:**
  - `TestOnlyAdminCanRestore`: manager and employee get 403.
  - `TestRestoredItemComesBackDropped`.
  - `TestRestoreMarksArchiveRowRestored` (`restored_at`, `restored_by`).
  - `TestRestoredMobileClashIsRefused` (open point B). Restoring a customer whose mobile is now used by another live customer returns `ErrMobileInUse`.
  - `TestTrackingNumberClashOnRestoreIsRefused`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 20: Purge expired archive rows and documents

**Files:**
- Create: `backend/modules/archive/purge.go`, `backend/modules/archive/purge_test.go`

**Interfaces:**
- Produces: `archive.PurgeExpired(ctx, now time.Time) (PurgeResult, error)`. Deletes `archive` rows and documents where `purge_after < now`. Keeps audit rows.

- [ ] **Step 1: Write failing tests:**
  - `TestPurgeRemovesExpiredArchiveRow`.
  - `TestPurgeKeepsAuditEntry`.
  - `TestPurgeRemovesDocumentsOneYearAfterDisbursedOrDropped` (uses `documents.purge_after`).
  - `TestPurgeKeepsRowsBeforeExpiry`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.
- [ ] **Step 5: Schedule** the purge once a day in `main.go`, next to the backup job (Task 33).

---

## Phase 5: Documents

### Task 21: Document type and size checks

**Files:**
- Create: `backend/modules/documents/validate.go`, `backend/modules/documents/validate_test.go`, `backend/modules/documents/testdata/` (small sample files: `sample.pdf`, `sample.jpg`, `sample.png`, `fake.pdf` containing HTML, `big.pdf` of 10 MB + 1 byte generated in the test)

**Interfaces:**
- Produces: `documents.ValidateFile(name string, content []byte) (contentType string, err error)`. Accepts only PDF, JPEG and PNG, judged by the first bytes of the content, not the file name. Max 10 MB (10485760 bytes).

- [ ] **Step 1: Write failing tests:**
  - `TestAcceptsRealPDFJPGPNG`.
  - `TestRejectsHTMLNamedPDF`: `fake.pdf` returns `ErrUnsupportedType`. (Review Focus item 3.)
  - `TestRejectsExecutableNamedJPG`.
  - `TestRejectsOver10MB`: exactly 10485760 bytes accepted, 10485761 refused.
  - `TestRejectsEmptyFile`.
- [ ] **Step 2–4:** run → FAIL; implement with `http.DetectContentType` plus a PDF `%PDF-` check; run → PASS.

### Task 22: Upload with replace and stage tag

**Files:**
- Create: `backend/modules/documents/upload.go`, `backend/modules/documents/upload_test.go`, `backend/modules/documents/store.go`, `backend/modules/documents/queries/documents.sql`

**Interfaces:**
- Produces: `documents.Upload(ctx, actor, caseID int64, label string, file UploadedFile) (Document, error)`. `UploadedFile{Name string, Content []byte}`. Sets `stage_id` to the case's current stage. Same `(case_id, label)` replaces the file and removes the old stored object. `Store` interface with `Put(key string, data []byte) error`, `Get(key string) ([]byte, error)`, `Delete(key string) error`. Test store is in-memory. The real store is **[hosting-blocked]** (Task 34).
- Consumes: `ValidateFile` (Task 21), `access.Can(…, ActionUploadDocument)`.

- [ ] **Step 1: Write failing tests:**
  - `TestUploadRecordsStageAtUpload`.
  - `TestUploadIsOptionalAtEveryStage`: no upload is required to move a case.
  - `TestSameLabelReplacesFileAndDeletesOldObject`.
  - `TestLabelIsFreeText` (round 12).
  - `TestEmployeeCanUpload`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 23: Private download links

**Files:**
- Create: `backend/modules/documents/link.go`, `backend/modules/documents/link_test.go`

**Interfaces:**
- Produces: `documents.SignLink(docID int64, now time.Time) string` (HMAC-SHA256 over doc ID and expiry, with `NEXFLOW_LINK_SECRET`). `documents.OpenLink(token string, now time.Time) (docID int64, err error)`. Expiry is 10 minutes (owner, round 13). Route: `GET /api/documents/{token}`.

- [ ] **Step 1: Write failing tests:**
  - `TestValidLinkOpensDocument`.
  - `TestExpiredLinkRefused`: 11 minutes later, `ErrLinkExpired`. (Review Focus item 4.)
  - `TestTamperedLinkRefused`: a changed doc ID returns `ErrLinkInvalid`.
  - `TestDocumentNotReachableWithoutLink`: `GET /api/documents/1` without a token returns 404.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 24: Stage document requirement

**Files:**
- Create: `backend/modules/documents/requirement.go`, `backend/modules/documents/requirement_test.go`

**Interfaces:**
- Produces: `documents.RequirementStatus(ctx, caseID int64) (Status, error)` where `Status{Required bool, Satisfied bool, Warning string}`. A stage is satisfied when the case has at least one document. Missing documents give a warning, never a refusal.

- [ ] **Step 1: Write failing tests:**
  - `TestStageNotRequiringDocumentsHasNoWarning`.
  - `TestRequiredStageWithoutDocumentWarns`.
  - `TestRequiredStageWithDocumentIsSatisfied`.
  - `TestMissingDocumentDoesNotBlockStageMove`: `cases.ChangeStage` (Task 12) still succeeds.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 6: Lists, search and reports

### Task 25: Search

**Files:**
- Create: `backend/modules/cases/search.go`, `backend/modules/cases/search_test.go`

**Interfaces:**
- Produces: `cases.Search(ctx, q SearchQuery) ([]CustomerGroup, error)`. Matches name (partial, case-insensitive), mobile (after normalization, if the query has digits), tracking number (exact), bank name. `CustomerGroup{Customer, Cases []Case}`.

- [ ] **Step 1: Write failing tests:**
  - `TestSearchByName`, `TestSearchByMobileWithPlus91Prefix`, `TestSearchByTrackingNumber`, `TestSearchByBank`.
  - `TestResultsGroupedByCustomer`: two cases under one customer appear in one group.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 26: List filters

**Files:**
- Create: `backend/modules/cases/list.go`, `backend/modules/cases/list_test.go`

**Interfaces:**
- Produces: `cases.List(ctx, f ListFilter) ([]Case, error)`. `ListFilter{StageID, BankID *int64, From, To *time.Time, AssigneeID *int64}`. Filters combine with AND. Dropped items appear unless the caller filters by status (dropped filter not selected, round 3).

- [ ] **Step 1: Write failing tests:**
  - `TestEachFilterAlone`, `TestFiltersCombine`, `TestDroppedItemsStillListed`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 27: Dashboard and counting rules

**Files:**
- Create: `backend/modules/reports/dashboard.go`, `backend/modules/reports/dashboard_test.go`, `backend/modules/reports/ist.go`

**Interfaces:**
- Produces: `reports.Dashboard(ctx, now time.Time, period Period) (DashboardData, error)`. `DashboardData{NewCustomersToday, NewCasesToday int, CasesByStage map[string]int, Sanctions, Disbursements StageCount}`. `StageCount{Count int, Amount decimal string}`. `reports.TodayIST(now time.Time) (time.Time, time.Time)` returns the start and end of the India calendar day.

- [ ] **Step 1: Write failing tests:**
  - `TestTodayUsesIndiaTime`: a record created at 2026-11-12 19:00 UTC counts as 13 November IST and is not counted for 12 November. (Review Focus, dashboard boundary.)
  - `TestSanctionCountsCaseThatMovedOn`: a case that reached Sanctioned in the period and later reached Disbursed still counts in the sanction count.
  - `TestDroppedAfterSanctionCountedAndMarked`.
  - `TestCasesByStageCountsCurrentStageOnly`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 28: CSV export

**Files:**
- Create: `backend/modules/reports/csv.go`, `backend/modules/reports/csv_test.go`

**Interfaces:**
- Produces: `reports.ExportCSV(w io.Writer, rows []Row) error`. Writes a UTF-8 byte-order mark first so Excel shows Indian names correctly (my call). Route `GET /api/reports/export.csv`, same filters as the on-screen report.

- [ ] **Step 1: Write failing tests:**
  - `TestCSVStartsWithBOM`.
  - `TestCSVRowsMatchOnScreenRows`: the same `[]Row` gives the same values.
  - `TestCSVEscapesCommasAndQuotes`.
  - `TestAllStaffCanExport` (spec §6).
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 7: Alerts

### Task 29: Alert queue and sender

**Files:**
- Create: `backend/modules/alerts/queue.go`, `backend/modules/alerts/queue_test.go`, `backend/modules/alerts/sender.go`

**Interfaces:**
- Produces: `alerts.Sender` interface with `Send(ctx, to, subject, body string) error`. `alerts.Enqueue(ctx, caseID, recipientID int64, kind string, dueOn *time.Time) error` writes `alert_log` (unique on case, kind, due date, recipient). Real SMTP sender is **[hosting-blocked]**; tests use a fake sender.

- [ ] **Step 1: Write failing tests:**
  - `TestAssignedAlertGoesToNewAssignee`.
  - `TestSameAlertNotSentTwice`: unique constraint, second enqueue is a no-op.
  - `TestAlertFailureIsRetriedNotLost`: a failing sender leaves the row unsent for the next run.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 30: Follow-up due and overdue alerts

**Files:**
- Create: `backend/modules/alerts/followup.go`, `backend/modules/alerts/followup_test.go`

**Interfaces:**
- Produces: `alerts.RunFollowUps(ctx, now time.Time) (Result, error)`. Daily. For cases with `follow_up_date = today (IST)`, enqueues `follow_up_due`. For cases with `follow_up_date < today (IST)` and status active, enqueues `follow_up_overdue` once, on the day after the due date (my call, open point C).

- [ ] **Step 1: Write failing tests:**
  - `TestNoDateNoAlert` (round 6).
  - `TestDueAlertOnDueDate`.
  - `TestOverdueAlertOnceOnNextDay`.
  - `TestDroppedCaseGetsNoFollowUpAlert`.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 8: Backend API and scheduled jobs

### Task 31: HTTP routes and permission checks

**Files:**
- Create: `backend/modules/http/routes.go`, `backend/modules/http/routes_test.go`

**Interfaces:**
- Produces: all `/api` routes in one place, each handler calling the module function from Tasks 4 to 30. Every route is behind `access.RequireUser`. Admin routes under `/api/admin` use `access.RequireAction(ActionManageConfig | ActionManageUsers | ActionRestoreArchive)`.

- [ ] **Step 1: Write failing tests**, one table test with each route and each role, expecting the status from spec §6. Example: `DELETE /api/customers/1` as employee → 403, as manager → 204.
- [ ] **Step 2–4:** run → FAIL; implement the routes; run → PASS.
- [ ] **Note:** this task finishes the server-side enforcement that spec §13 check 18 and check 7 require.

### Task 32: Scheduled jobs runner

**Files:**
- Create: `backend/cmd/nexflow/jobs.go`, `backend/cmd/nexflow/jobs_test.go`

**Interfaces:**
- Produces: `jobs.Run(ctx, name string, now time.Time)` for `purge`, `followups`, `alerts-send`. The `main` process runs them once a day at 02:00 IST, and `alerts-send` every 15 minutes.

- [ ] **Step 1: Write failing tests:** each job name runs once and returns no error; an unknown name is refused.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 33: Backup job **[hosting-blocked for off-server copy]**

**Files:**
- Create: `infra/backup/backup.sh`, `infra/backup/retain.sh`, `infra/backup/README.md`

**Interfaces:**
- Produces: a daily `pg_dump` plus the uploaded-files directory, with 30 days retained (spec §12). Writes to a local directory now. The copy to a second location in India is hosting-blocked.

- [ ] **Step 1: Write a test script** `infra/backup/test_retention.sh` that creates 35 dated dummy backups and checks that `retain.sh` keeps exactly 30 newest.
- [ ] **Step 2–4:** run → FAIL; write scripts; run → PASS.
- [ ] **Checkpoint:** a restore test restores the dump into a fresh test database and the row counts match.

---

## Phase 9: Web app

Frontend choices (Vite, TypeScript, React Router, Tailwind CSS, Vitest, React Testing Library, Playwright) are my choices. Confirm with the owner before Task 34.

### Task 34: Frontend scaffold and API client

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/src/main.tsx`, `web/src/api/client.ts`, `web/src/api/client.test.ts`

**Interfaces:**
- Produces: `api.request<T>(method, path, body?) : Promise<T>`, which sends the session cookie, and throws `ApiError{status, message}` on a non-2xx response.

- [ ] **Step 1: Write failing test:** a 403 response throws `ApiError` with status 403.
- [ ] **Step 2–4:** run `npx vitest run` → FAIL; implement; run → PASS.

### Task 35: Login and phone layout

**Files:**
- Create: `web/src/pages/Login.tsx`, `web/src/pages/Login.test.tsx`, `web/e2e/phone.spec.ts`

- [ ] **Step 1: Write failing tests:** the login form shows an error on a wrong password; Playwright at 360 px width shows no horizontal scroll on the login page (spec §12).
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 36: Customer and case screens

**Files:**
- Create: `web/src/pages/Search.tsx`, `web/src/pages/CaseList.tsx`, `web/src/pages/CustomerDetail.tsx`, `web/src/pages/CaseDetail.tsx`, and matching `.test.tsx` files

- [ ] **Step 1: Write failing tests:**
  - Search results show customers with their cases grouped.
  - Case detail shows stage history in order, with each event's date.
  - Stage change shows the tracking number field when moving to "Logged in".
  - A stage marked as requiring documents shows a warning, not an error, when no file is uploaded.
  - Drop dialog requires a reason, and a remark when reason is "Other".
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 37: Admin screens

**Files:**
- Create: `web/src/pages/admin/Users.tsx`, `web/src/pages/admin/Banks.tsx`, `web/src/pages/admin/Stages.tsx`, `web/src/pages/admin/Dropdowns.tsx`, `web/src/pages/admin/Archive.tsx`, and tests

- [ ] **Step 1: Write failing tests:** a manager does not see admin links; a restore button appears only for admin; the stages screen has a "requires documents" toggle.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

### Task 38: Dashboard and reports screens

**Files:**
- Create: `web/src/pages/Dashboard.tsx`, `web/src/pages/Reports.tsx`, and tests

- [ ] **Step 1: Write failing tests:** the dashboard shows the four counts from spec §9; the CSV export button links to `/api/reports/export.csv` with the current filters.
- [ ] **Step 2–4:** run → FAIL; implement; run → PASS.

---

## Phase 10: Deployment and acceptance

### Task 39: Acceptance checks **[hosting-blocked for hosted run]**

**Files:**
- Create: `backend/acceptance/acceptance_test.go`

- [ ] **Step 1: Write one test per check in spec §13** (checks 1 to 21), named `TestAcceptance01…TestAcceptance21`. Each runs against the local test database. Check 16 and check 17 (real email) use the fake sender locally.
- [ ] **Step 2: Run** `go test ./acceptance/...` → all pass.
- [ ] **Step 3: Hosted run** against the chosen provider, once hosting is decided. Blocked until then.

---

## Owner answers (round 13) — these are settled

- **A.** Already-dropped cases keep their reason. Disbursed cases are dropped too (Task 17).
- **B.** Restore is refused when the mobile is in use (Task 19).
- **C.** Due alert on the due date, plus an overdue alert the day after (Task 30).
- **D.** Customer is top-level. Cases link to its ID. An archived case keeps a copy of its customer's details. Deleting a customer means archiving its cases first, and the customer's archive entry lists them (Task 18).
- **E.** Download link expires after 10 minutes (Task 23).
- **F.** Frontend and other tool choices are left to me (Task 34).

The original questions are kept below for reference.

## Original open points (answered above)

- **A. Customer drop and already-dropped cases.** Spec §4.1 says a customer drop drops all its cases. It does not say what happens to a case that is already dropped or already completed (Disbursed). Plan (Task 17): already-dropped cases keep their original reason; completed cases are dropped too. **Owner to confirm.**
- **B. Restoring a customer whose mobile is now in use.** Spec §4.5 covers restore but not clashes. Plan (Task 19): restore is refused with a clear message. **Owner to confirm.**
- **C. Overdue alert timing.** Spec §10 says "due or overdue" without a number of days. Plan (Task 30): one overdue alert, the day after the due date. **Owner to confirm.**
- **D. Customer hard-delete and its cases.** Spec §4.1 says only the customer's cases can be deleted first, and §4.5 says the archive keeps the customer with its cases. These two rules conflict once each case is archived separately. Plan (Task 18): each case is archived on its own, and the customer archive records the case IDs it had. **Owner to confirm.** Without an answer, Task 18 is blocked for customers with cases.
- **E. Link expiry.** "Short-lived" in the spec is set to 5 minutes (Task 23). **Owner to confirm.**
- **F. Frontend tools.** Vite, Tailwind CSS, Vitest, Playwright are my picks. Owner said "sure" to the approach; confirm before Task 34.
- **G. PostgreSQL local version.** Local development uses 16. Production version depends on the host (hosting-blocked).

## Self-review notes

- Spec coverage: §3 items 1 to 11 map to Phases 3 to 9. §6 permissions map to Tasks 5, 31. §13 checks map to Task 39. Round 11 and 12 changes map to Tasks 3, 4, 22, 24.
- Review Focus items each have a test in the owning task (Tasks 4, 10, 13, 21, 23, 7, 27).
- Interfaces named in this plan are used with the same names in later tasks.
- The plan does not commit anything. Commits happen only when the owner asks.
