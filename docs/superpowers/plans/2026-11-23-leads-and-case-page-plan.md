# Leads, Lead to Case, Case Page Layout: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** New customers start as leads (name, mobile, project, assignee) listed on Home. Staff log each call, then turn the lead into a normal case with bank, amount and unit number. The case page leads with the customer.

**Architecture:** A new `modules/leads` package owns the `leads` and `lead_contacts` tables. It imports `customers` but never `cases`. Turning a lead into a case lives in `modules/cases` (`CreateFromLead`), so the case rules stay in one place; it reads and updates `leads` rows through `casesdb` queries in the same transaction. Alerts gain an optional `lead_id`. The web app gets a Leads chip on Home, a `/leads/:id` page, one shared Add case form, and a reworked case page.

**Tech Stack:** Go 1.25, chi, pgx/v5, sqlc, golang-migrate, PostgreSQL 16; React 18 + TypeScript, Vite, Tailwind, Vitest + Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-23-leads-and-case-page-design.md` (mockups: `mockups/round15/`)

## Global Constraints

- Lead status: `open`, `converted`, `dropped`. Contact response: `interested`, `call_back`, `not_reachable`, `not_interested`. Web labels: Interested, Call back later, Not reachable, Not interested.
- A new lead's `next_call_date` is the IST day it is created. A logged contact replaces it with the contact's next call date, or clears it when that is empty. Converting a lead clears it.
- Remark is required on every contact (trimmed, not empty). Contacts are never updated or deleted, except when the archive deletes a customer.
- A converted or dropped lead accepts no contacts, no assignment and no conversion: `ErrLeadClosed`.
- One open lead per customer and project (partial unique index). Message: "This customer already has an open lead for this project."
- Unit no.: trim, upper-case, then must match `^[A-Z0-9][A-Z0-9 /-]{0,19}$`. Empty means none. Refusal message: "Unit no. can have letters, numbers, spaces, - and /, up to 20 characters."
- Add case `start_at` is `received` (default) or `logged_in`. `logged_in` needs a tracking number under the existing rule (`NormalizeTracking`, live-case uniqueness).
- Every signed-in role may create, contact, assign, drop, bring back and convert leads (`access.ActionAddRecord`, the same check as cases). Hard delete only through the customer (`access.ActionHardDelete`).
- Audit actions: `lead.created`, `lead.contacted`, `lead.assigned`, `lead.dropped`, `lead.brought_back`, `lead.converted` (entity type `lead`).
- Errors follow `errors.New("<pkg>: <message>")`, are mapped in `modules/http/errors.go`, and are shown through `friendlyMessage` on the web.
- Run `sqlc generate` after every change to a migration or `queries/*.sql`. Generated code is committed. Add a `leads` entry to `backend/sqlc.yaml`.
- Mockups in `mockups/round15/` are the look for every new or changed screen. Text sizes: the customer name heading is `text-2xl font-bold`; details values are body size (`text-base`), not `text-sm`.
- **Commits:** only when the owner has asked, and on `docs/project-foundation`. Otherwise skip the commit steps.

**Commands** (from repo root):
- Backend: `docker compose run --rm --build test go test -count=1 ./modules/<pkg>/ -run <Name> -v`. Full suite: `docker compose run --rm --build test`. `TestAcceptance19` fails already (infra/backup not in the container); ignore it.
- sqlc: `cd backend && PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH" sqlc generate`.
- Web: `cd web && npm test -- <file>`, then `npm run build`. Phone check: run `npm run test:e2e` inside `mcr.microsoft.com/playwright:v1.48.2-jammy`, then chown `dist`/`test-results` back to the user.

## Review Focus

1. **Two people press Add case on the same lead at once** (or a phone double tap): exactly one case, the second gets `ErrLeadClosed` (HTTP 409), and the button is disabled while busy. Tests in Tasks 4 and 9.
2. **Logging "Not interested" and then declining the drop prompt**: the contact is saved, the lead stays open, and the next call date follows the contact. Tests in Tasks 3 and 9.
3. **New customer with a known mobile whose customer is dropped**: the lead is created, the customer is brought back, the old cases and leads stay dropped, and no second customer appears. Test in Task 2.
4. **Restoring a case archived after its lead was converted**: the restore succeeds. It keeps `lead_id` when that lead still exists and no other case uses it, and sets it to NULL otherwise. Test in Task 5.
5. **A next call date in the past, or a date typed on another day than IST "today"**: accepted (staff log late), and the lead counts as overdue from the next IST day. Tests in Tasks 3 and 6.

---

### Task 1: Migration 000005 (leads, contacts, case columns, alert link, backfill)

**Files:**
- Create: `backend/migrations/000005_leads.up.sql`, `backend/migrations/000005_leads.down.sql`
- Test: `backend/migrations/migrate_test.go`

**Interfaces:**
- Produces (SQL):
  - `leads(id, customer_id NOT NULL → customers ON DELETE RESTRICT, project_id NOT NULL → projects, assignee_id NOT NULL → users, status CHECK IN ('open','converted','dropped') DEFAULT 'open', next_call_date date, dropped_reason_id, dropped_remark, dropped_at, dropped_by, converted_at, created_at, created_by, updated_at, updated_by)`, plus the same drop CHECK as cases.
  - Indexes: `leads_customer_idx (customer_id)`, `leads_open_call_idx (next_call_date) WHERE status = 'open'`, unique `leads_one_open_per_project_uidx (customer_id, project_id) WHERE status = 'open'`.
  - `lead_contacts(id, lead_id NOT NULL → leads, response CHECK IN (...4 values), remark text NOT NULL CHECK (btrim(remark) <> ''), next_call_date date, created_at, created_by → users)`, index `(lead_id, created_at)`.
  - `cases.lead_id bigint UNIQUE REFERENCES leads(id) ON DELETE SET NULL`, `cases.unit_number text CHECK (char_length(unit_number) <= 20)`.
  - `alert_log`: `case_id` becomes nullable; add `lead_id bigint REFERENCES leads(id)`; `CHECK (num_nonnulls(case_id, lead_id) = 1)`; `UNIQUE NULLS NOT DISTINCT (lead_id, kind, due_on, recipient_user_id)`.
  - Backfill: one open lead for every `status = 'active'` customer with no cases. Project `na_project_id()`. Assignee `COALESCE(created_by, first active admin by id)`; skip the customer if both are NULL. Next call `(now() AT TIME ZONE 'Asia/Kolkata')::date`. `created_by` is the same as the assignee.
- Down: delete alert rows with `lead_id`, drop the added constraints and columns, restore `case_id NOT NULL`, drop both tables.

- [ ] **Step 1: Write failing tests** in `migrate_test.go`:
  - `TestMigration5BackfillsLeadsForCaselessCustomers`: migrate to 4. Insert an admin, customer A (no case, `created_by` = admin), customer B with a case, and customer C dropped with no case. Migrate up. A has exactly one open lead on NA, assigned to the admin, with next call = IST today. B and C have none.
  - `TestMigration5OneOpenLeadPerProject`: a second open lead for the same customer and project fails with 23505. One open and one dropped lead are allowed.
  - `TestMigration5AlertNeedsOneTarget`: an `alert_log` row with both `case_id` and `lead_id` NULL fails. One with both set fails.
  - Extend `TestMigrateDownThenUpIsClean` so it covers version 5.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./migrations/ -run 'Migration5|DownThenUp' -v` → FAIL.
- [ ] **Step 3: Write both migration files** as described in Interfaces.
- [ ] **Step 4: Run** the Step 2 command → PASS. Run `sqlc generate` and the full backend suite → PASS. Existing `alert_log.case_id` readers now see `pgtype.Int8`; fix the compile errors in `modules/alerts` with no behaviour change.
- [ ] **Step 5: Commit** `feat: add leads and lead contacts schema with backfill`.

### Task 2: Leads service (create, add for customer, read, list, assign)

**Files:**
- Create: `backend/modules/leads/leads.go`, `backend/modules/leads/create.go`, `backend/modules/leads/list.go`, `backend/modules/leads/queries/leads.sql`, `backend/modules/leads/leadsdb/` (generated)
- Modify: `backend/sqlc.yaml`
- Test: `backend/modules/leads/create_test.go`, `backend/modules/leads/list_test.go` (copy the fixture pattern from `modules/cases/create_test.go`)

**Interfaces:**
- Consumes: `customers.NormalizeMobile`, `customers.ReactivateTx(ctx, tx, rec, actor, customerID)`, `customers.Customer`, `history.Recorder`, `history.WithTx`.
- Produces (package `leads`):
  - `type Lead struct { ID, CustomerID, ProjectID, AssigneeID int64; Status string; NextCallDate *time.Time; CreatedAt time.Time; ConvertedAt *time.Time; CaseID *int64 }`. `CaseID` is the case whose `lead_id` is this lead, if any.
  - `type Row struct { Lead; CustomerName, CustomerMobile, CustomerEmail, ProjectName, AssigneeName, LastResponse string; LastContactAt *time.Time; CallDue bool; OverdueDays int }`. `CallDue` is true when open and next call ≤ IST today. `OverdueDays` is IST today minus next call, when positive.
  - `type NewLead struct { Name, Mobile, Email string; ProjectID, AssigneeID int64 }`
  - `type CreateResult struct { Lead *Lead; Customer *customers.Customer; Existing *customers.Customer }`
  - `type Notifier interface { QueueLeadAssigned(ctx context.Context, leadID, recipientID int64) error }`, `func (s *Service) SetNotifier(n Notifier)`
  - `func NewService(pool *pgxpool.Pool, rec history.Recorder) *Service`
  - `func (s *Service) Create(ctx, actor access.User, in NewLead, now time.Time) (CreateResult, error)`: a new mobile creates the customer and lead in one transaction. A known mobile returns `Existing` and creates nothing.
  - `func (s *Service) AddForCustomer(ctx, actor, customerID, projectID, assigneeID int64, bringBack bool, now time.Time) (Lead, error)`: a dropped customer needs `bringBack`, otherwise `ErrCustomerDropped`.
  - `func (s *Service) Get(ctx, id int64, now time.Time) (Row, error)`, `func (s *Service) ListOpen(ctx, now time.Time) ([]Row, error)`, `func (s *Service) ForCustomer(ctx, customerID int64, now time.Time) ([]Row, error)` (all statuses, newest first).
  - `func (s *Service) Assign(ctx, actor, leadID, toUserID int64) (Lead, error)`: queues the notifier inside the transaction.
  - Errors: `ErrLeadNotFound`, `ErrLeadClosed` ("leads: this lead is closed"), `ErrProjectRequired` ("leads: choose a project, or NA"), `ErrProjectNotFound`, `ErrLeadExists` ("leads: this customer already has an open lead for this project"; map 23505 on the partial index), `ErrCustomerDropped` ("leads: this customer was dropped. Bring them back first").
  - `ListOpen` order: `CallDue` first by next call ascending, then the rest by `created_at` descending.

- [ ] **Step 1: Write failing tests:**
  - `TestCreateLeadMakesCustomerAndLead`: one customer row and one open lead, next call = IST day of `now` (use `now = 2026-11-23T20:00:00Z`, which is IST 24 Nov), audit `customer.created` and `lead.created`.
  - `TestCreateLeadKnownMobileCreatesNothing`: `Existing` is set, and the lead and customer counts don't change.
  - `TestAddLeadBringsBackDroppedCustomer`: a dropped customer with a dropped case. Without bringBack → `ErrCustomerDropped`. With it → customer active, new lead open, old case still dropped.
  - `TestAddLeadSameProjectTwiceRefused` → `ErrLeadExists`. A different project → ok.
  - `TestCreateLeadNeedsProject`: ProjectID 0 → `ErrProjectRequired`; unknown → `ErrProjectNotFound`.
  - `TestAssignLeadQueuesAlert`: fake notifier gets `(leadID, toUserID)`, audit `lead.assigned`; a dropped lead → `ErrLeadClosed`.
  - `TestListOpenOrder`: leads with next call yesterday, today, tomorrow and none → order yesterday, today, then newest-created of the rest. Yesterday has `OverdueDays == 1`. Converted and dropped leads are excluded.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/leads/ -v` → FAIL (package missing).
- [ ] **Step 3: Write the queries** (`InsertLead`, `GetLeadRow`, `ListOpenLeadRows`, `LeadRowsForCustomer`, `LockLead`, `SetLeadAssignee`). Last response and last contact come from a lateral join on the newest `lead_contacts` row. Add the sqlc entry and generate.
- [ ] **Step 4: Implement the service.** For the customer insert in `Create`, reuse the `customers` mobile normalisation and the `ON CONFLICT DO NOTHING` pattern (see `customers.Create`). Do it inside the lead transaction.
- [ ] **Step 5: Run** the Step 2 command → PASS.
- [ ] **Step 6: Commit** `feat: add leads service`.

### Task 3: Contact log, drop and bring back

**Files:**
- Create: `backend/modules/leads/contact.go`, `backend/modules/leads/drop.go`
- Modify: `backend/modules/leads/queries/leads.sql`
- Test: `backend/modules/leads/contact_test.go`, `backend/modules/leads/drop_test.go`

**Interfaces:**
- Produces:
  - `type Contact struct { ID int64; Response, Remark string; NextCallDate *time.Time; CreatedAt time.Time; CreatedBy string }` (CreatedBy is the user's full name)
  - `type ContactIn struct { Response, Remark string; NextCallDate *time.Time }`
  - `func (s *Service) LogContact(ctx, actor, leadID int64, in ContactIn) (Lead, error)`
  - `func (s *Service) Contacts(ctx, leadID int64) ([]Contact, error)`: newest first.
  - `func (s *Service) Drop(ctx, actor, leadID, reasonID int64, remark string) (Lead, error)`: same reason validation as case drop (`drop_reason` dropdown value must exist).
  - `func (s *Service) BringBack(ctx, actor, leadID int64, now time.Time) (Lead, error)`: dropped → open, drop fields cleared, next call = IST today. A dropped customer is brought back in the same transaction (`customers.ReactivateTx`). Refused with `ErrLeadExists` if the customer already has an open lead for that project. Refused with `ErrLeadClosed` if the lead is not dropped.
  - Errors: `ErrInvalidResponse` ("leads: choose a response"), `ErrRemarkRequired` ("leads: write a remark").

- [ ] **Step 1: Write failing tests:**
  - `TestLogContactUpdatesNextCall`: next call date 2026-11-25 → lead next call 2026-11-25. Log again with none → next call NULL. Two contacts, newest first, audit `lead.contacted`.
  - `TestLogContactNotInterestedKeepsLeadOpen`: response `not_interested`, the lead is still `open` (review focus 2).
  - `TestLogContactPastDateAccepted`: a next call date of yesterday is stored as given (review focus 5).
  - `TestLogContactValidation`: response `"maybe"` → `ErrInvalidResponse`; remark `"   "` → `ErrRemarkRequired`; a dropped lead → `ErrLeadClosed`.
  - `TestDropAndBringBackLead`: drop sets reason, remark, at and by. Bring back → open, drop fields NULL, next call = IST today, audits `lead.dropped` and `lead.brought_back`.
  - `TestBringBackLeadRevivesDroppedCustomer`: customer dropped → after bring back the customer is active.
  - `TestBringBackLeadRefusedWhenOpenDuplicate` → `ErrLeadExists`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/leads/ -run 'Contact|Drop|BringBack' -v` → FAIL.
- [ ] **Step 3: Implement.** Each write is one transaction that locks the lead row (`SELECT … FOR UPDATE`) and writes the audit entry inside it.
- [ ] **Step 4: Run** the Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat: add lead contact log, drop and bring back`.

### Task 4: Case from a lead, unit number, start at Logged in

**Files:**
- Create: `backend/modules/cases/fromlead.go`, `backend/modules/cases/unit.go`
- Modify: `backend/modules/cases/create.go`, `backend/modules/cases/queries/cases.sql`, `backend/modules/cases/list.go` (add `UnitNumber`, `LeadID` to `Case` and `CaseRow`)
- Test: `backend/modules/cases/fromlead_test.go`, `backend/modules/cases/unit_test.go`, `backend/modules/cases/create_test.go`

**Interfaces:**
- Produces (package `cases`):
  - `Case` gains `LeadID *int64` and `UnitNumber *string`. `NewCase` gains `UnitNumber string`, `StartAt string` (`""` or `"received"` or `"logged_in"`) and `TrackingNumber string`.
  - `func NormalizeUnit(s string) (string, error)`, returning `""` for an empty value. `ErrInvalidUnit` carries the Global Constraints message.
  - `ErrInvalidStart` ("cases: start at Received or Logged in").
  - `type FromLead struct { BankID int64; LoanAmount string; AssigneeID int64; UnitNumber, StartAt, TrackingNumber string }`
  - `func (s *Service) CreateFromLead(ctx, actor access.User, leadID int64, in FromLead) (Case, error)`. In one transaction it locks the lead (`FOR UPDATE`), refuses with `leads.ErrLeadClosed` unless the lead is open (cases may import `leads`, but `leads` must not import `cases`), creates the case on the lead's customer and project with `lead_id`, writes the stage events and assignment event, sets the lead to `converted` with `converted_at` and next call NULL, and writes audit entries `case.created` and `lead.converted`. A lead whose customer is dropped → `ErrCustomerDropped`.
  - `func (s *Service) SetUnitNumber(ctx, actor, caseID int64, unit string) (Case, error)`: audit `case.unit_changed`. A dropped case is refused the same way as a project change.
  - Logged in start: insert the Received event, then the Logged in event (same timestamp order), current stage = the bank's `logged_in` stage. The tracking number is required and checked as in `stage.go`.
- Refactor: move the shared insert of `Create` into `func (s *Service) createTx(ctx, tx pgx.Tx, actor, in NewCase, leadID *int64) (Case, error)`, used by both `Create` and `CreateFromLead`.

- [ ] **Step 1: Write failing tests:**
  - `TestCreateFromLeadAtReceived`: case on the lead's project, `LeadID` set, unit `b-14 ` saved as `B-14`, lead `converted`, next call NULL, one stage event.
  - `TestCreateFromLeadAtLoggedIn`: without a tracking number → `ErrTrackingRequired`. With `hl123` → stage `logged_in`, two stage events in order, tracking `HL123`.
  - `TestCreateFromLeadTwiceRefused`: the second call → `leads.ErrLeadClosed`, and only one case exists.
  - `TestCreateFromLeadConcurrent`: two goroutines convert the same lead → exactly one success and one `ErrLeadClosed` (review focus 1).
  - `TestCreateFromDroppedLeadRefused` → `leads.ErrLeadClosed`.
  - `TestNormalizeUnit`: `" b-14 "` → `"B-14"`; `"Flat 3/2"` → `"FLAT 3/2"`; `""` → `""`; `"#12"`, `"-12"` and a 21-character value → `ErrInvalidUnit`.
  - `TestCreateCaseWithUnitAndLoggedIn`: the direct `Create` accepts `UnitNumber`, `StartAt` and `TrackingNumber` with the same rules, and `StartAt "sanctioned"` → `ErrInvalidStart`.
  - `TestSetUnitNumber`: change and clear work; a dropped case is refused.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/cases/ -run 'FromLead|Unit|LoggedIn' -v` → FAIL.
- [ ] **Step 3: Implement** queries (`LockLeadForCase`, `MarkLeadConverted`, `SetCaseUnit`, extend `InsertCase` and the case selects), generate, then the service code.
- [ ] **Step 4: Run** the Step 2 command, then `./modules/cases/` in full → PASS.
- [ ] **Step 5: Commit** `feat: open a case from a lead, with unit number and Logged in start`.

### Task 5: Customer drop, delete and restore, and case restore with leads

**Files:**
- Modify: `backend/modules/customers/drop.go`, `backend/modules/customers/queries/customers.sql`, `backend/modules/archive/delete.go`, `backend/modules/archive/restore.go`, `backend/modules/archive/queries/archive.sql`
- Test: `backend/modules/customers/drop_test.go`, `backend/modules/archive/delete_test.go`, `backend/modules/archive/restore_test.go`

**Interfaces:**
- Customer drop: open leads → dropped, with the customer's reason and remark, in the same transaction. Converted leads are untouched.
- `archive.ErrCustomerHasOpenLeads` ("archive: this customer has open leads. Drop the customer first") → HTTP 409.
- The customer snapshot gains `"leads": [ {lead row JSON, "contacts": [...]} ]`. Delete removes the alert rows with those `lead_id`s, then the contacts, then the leads, before the customer row.
- Restore customer: re-inserts the leads with `OVERRIDING SYSTEM VALUE`, all with status `dropped` (drop reason from the restore `DropInput`, `converted_at` kept for the record), then their contacts. Old snapshots without `"leads"` restore as before.
- Restore case: `lead_id` is kept only if that lead exists and no live case has it. Otherwise it is set to NULL (review focus 4).

- [ ] **Step 1: Write failing tests:**
  - `TestDropCustomerDropsOpenLeads`: one open and one converted lead → open becomes dropped with the same reason, and converted stays converted.
  - `TestDeleteCustomerWithOpenLeadRefused` → `ErrCustomerHasOpenLeads`.
  - `TestDeleteCustomerArchivesLeadsAndContacts`: a dropped customer with a dropped lead, 2 contacts and one lead alert → snapshot has 1 lead with 2 contacts, and the lead, contacts and alert rows are gone.
  - `TestRestoreCustomerBringsLeadsBackDropped`: the leads and contacts are back with original IDs and status `dropped`.
  - `TestRestoreOldCustomerSnapshotWithoutLeads` → succeeds.
  - `TestRestoreCaseKeepsOrClearsLeadID`: lead exists and is unlinked → kept. Lead gone, or another case links it → NULL.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/customers/ ./modules/archive/ -v` → FAIL.
- [ ] **Step 3: Implement** the queries and code.
- [ ] **Step 4: Run** the Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat: carry leads through customer drop, delete and restore`.

### Task 6: Lead reminders and assignment emails

**Files:**
- Modify: `backend/modules/alerts/queries/alerts.sql`, `backend/modules/alerts/followup.go`, `backend/modules/alerts/queue.go`, `backend/cmd/nexflow/` (wire `leads.SetNotifier`)
- Test: `backend/modules/alerts/followup_test.go`, `backend/modules/alerts/queue_test.go`

**Interfaces:**
- `FollowUpResult` gains `LeadDue, LeadOverdue int`. `RunFollowUps` also runs `QueueLeadCallDue` (open leads, next call = today) and `QueueLeadCallOverdue` (open leads, next call = yesterday), with kinds `follow_up_due` and `follow_up_overdue` and `lead_id` set.
- `func (q *Queue) QueueLeadAssigned(ctx, leadID, recipientID int64) error` satisfies `leads.Notifier`.
- `UnsentAlerts` uses `LEFT JOIN cases` and `LEFT JOIN leads`, and takes the customer through either one. It returns `case_ref` and `lead_ref` as nullable.
- `compose(kind, customer, project string, caseID, leadID *int64)`. Lead texts:
  - assigned: subject "A lead has been assigned to you", body "Lead for <customer> (<project>) has been assigned to you in Nexflow."
  - due: subject "Call due today", body "Call <customer> about <project> today."
  - overdue: subject "Call overdue", body "The call to <customer> about <project> is overdue."
  - Case texts are unchanged.

- [ ] **Step 1: Write failing tests:**
  - `TestLeadCallDueQueuedOnce`: run twice on the same day → one alert, `LeadDue == 1`. Converted and dropped leads → none.
  - `TestLeadCallOverdueNextDay`: next call 2026-11-23, run at IST 2026-11-24 → one overdue alert (review focus 5).
  - `TestDispatchLeadAlertText`: the sender receives the lead subject and body above.
  - `TestQueueLeadAssigned`: the row has `lead_id` and NULL `case_id`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/alerts/ -v` → FAIL.
- [ ] **Step 3: Implement**, then wire the notifier in `cmd/nexflow` next to the cases notifier.
- [ ] **Step 4: Run** the Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat: lead call reminders and assignment emails`.

### Task 7: HTTP API for leads, cases, customer and search

**Files:**
- Modify: `backend/modules/http/router.go` (add `Leads *leads.Service` to `Deps`), `backend/modules/http/handlers.go` (or create `backend/modules/http/leads_handlers.go`), `backend/modules/http/errors.go`, `backend/cmd/nexflow/`
- Test: `backend/modules/http/leads_routes_test.go`, `backend/modules/http/routes_test.go`

**Interfaces (JSON, snake_case, following the existing handlers):**
- `POST /api/leads {name, mobile, email, project_id, assignee_id}` → 201 `{lead, customer}` | 200 `{existing: customer}`
- `POST /api/customers/{id}/leads {project_id, assignee_id, bring_back}` → 201 lead
- `GET /api/leads` → open lead rows. `GET /api/leads/{id}` → `{lead: row, contacts: [...]}`
- `POST /api/leads/{id}/contacts {response, remark, next_call_date|null}` → 201 lead
- `POST /api/leads/{id}/assign {assignee_id}`, `POST /api/leads/{id}/drop {reason_id, remark}`, `POST /api/leads/{id}/bring-back` → 200 lead
- `POST /api/leads/{id}/case {bank_id, loan_amount, assignee_id, unit_number, start_at, tracking_number}` → 201 case
- `POST /api/cases` gains `unit_number`, `start_at`, `tracking_number`. `POST /api/cases/{id}/unit {unit_number}` → 200 case.
- The case JSON gains `lead_id` and `unit_number`. `GET /api/customers/{id}` gains `leads` (rows). Each `GET /api/search` group gains `leads` (open rows only).
- Lead row JSON: `id, customer_id, customer_name, customer_mobile, customer_email, project_id, project_name, assignee_id, assignee_name, status, next_call_date (YYYY-MM-DD|null), last_response|null, last_contact_at|null, call_due, overdue_days, created_at, converted_at|null, case_id|null`.
- Error mapping: `ErrLeadNotFound` → 404. `ErrLeadClosed`, `ErrLeadExists` and `ErrCustomerHasOpenLeads` → 409. Validation errors (`ErrInvalidResponse`, `ErrRemarkRequired`, `ErrInvalidUnit`, `ErrInvalidStart`, the project errors) → 400. Handlers pass `time.Now()` as `now`.

- [ ] **Step 1: Write failing route tests:** an employee runs the full flow (create lead → log contact → convert at Logged in → `GET /api/cases/{id}` shows `lead_id` and `unit_number`); a second convert → 409; contact with an empty remark → 400 with the message; a known mobile → 200 `existing`; the search group includes the open lead; signed out → 401 on `GET /api/leads`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/http/ -run Lead -v` → FAIL.
- [ ] **Step 3: Implement** the handlers, routes, mappings and wiring.
- [ ] **Step 4: Run** the full backend suite → PASS (except `TestAcceptance19`).
- [ ] **Step 5: Commit** `feat: leads HTTP API`.

### Task 8: Web: new customer creates a lead, Leads on Home

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/pages/home/NewCustomerPanel.tsx`, `web/src/pages/Home.tsx`, `web/src/pages/home/TodayPanel.tsx`
- Create: `web/src/pages/home/LeadList.tsx`
- Test: `web/src/pages/Home.test.tsx`

**Interfaces:**
- Types: `LeadStatus`, `LeadResponse`, `LeadRow` (the Task 7 fields), `LeadContact {id, response, remark, next_call_date, created_at, created_by}`. `Case` gains `lead_id: number | null` and `unit_number: string | null`. `Customer` gains `leads?: LeadRow[]`. `SearchGroup` gains `leads: LeadRow[]`.
- `export const responseLabel: Record<LeadResponse, string>` in `types.ts`.
- NewCustomerPanel fields (mockup 2): Name, Mobile number, Email (optional), Project (ProjectPicker, starts empty), Assigned to (starts as me). Button **Save lead** posts `POST /api/leads` and navigates to `/leads/{id}`. On `existing` it shows the existing-customer message with **Add lead for this customer** (or **Bring back and add lead** when dropped), posting `POST /api/customers/{id}/leads`. Typed values are kept on error.
- Home: `Scope` gains `"leads"`. The **Leads · N** chip comes first. `LeadList` rows follow mockup 1: name, mobile · project · assignee; on the right a badge for the last response (**Not contacted** when none), then "Next call 24 Nov" / "Added today" / "Last call 21 Nov". Rows link to `/leads/{id}`. Needs attention (chip list and TodayPanel) also lists leads with `call_due`, with the reason "Call due today" or "Call overdue by N day(s)". The default scope stays as today.

- [ ] **Step 1: Write failing tests:** `"New customer saves a lead"` (no bank or amount fields, posts the five fields, navigates to `/leads/7`); `"known mobile offers Add lead for this customer"`; `"dropped customer offers Bring back and add lead"`; `"Leads chip lists open leads first"` (shows the count, Not contacted badge, row link); `"Needs attention includes due lead calls"` (text "Call overdue by 2 days").
- [ ] **Step 2: Run** `cd web && npm test -- Home.test.tsx` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the Step 2 command and `npm run build` → PASS.
- [ ] **Step 5: Commit** `feat(web): leads on Home and new customer as lead`.

### Task 9: Web: lead page and shared Add case form

**Files:**
- Create: `web/src/pages/LeadDetail.tsx`, `web/src/pages/lead/LogContactDialog.tsx`, `web/src/components/AddCaseDialog.tsx`, `web/src/components/AssignDialog.tsx` (moved out of `CaseDetail.tsx`; takes the POST path as a prop)
- Modify: `web/src/App.tsx` (route `/leads/:id`), `web/src/pages/CaseDetail.tsx` (use the moved AssignDialog), `web/src/pages/CustomerDetail.tsx` (replace `NewCaseDialog` with `AddCaseDialog`)
- Test: `web/src/pages/LeadDetail.test.tsx`, `web/src/components/AddCaseDialog.test.tsx`

**Interfaces:**
- `AddCaseDialog` props: `{ open; onClose; onSaved(c: Case); customerName: string; fromLead?: { id: number; projectName: string; assigneeId: number }; customerId?: number }`. With `fromLead` the project is fixed text and it posts `/api/leads/{id}/case`. Otherwise it shows ProjectPicker and posts `/api/cases`. Fields (mockup 4): Unit no., Bank, Loan amount (₹), Assigned to, Start at (Received | Logged in). Logged in shows a Tracking number field (upper-case as typed, the existing rule). Save is disabled while busy.
- LeadDetail (mockup 3):
  - top card with `h1` customer name (`text-2xl font-bold`), a `tel:` mobile link and the email;
  - Details list: Project, Assigned to (change), Next call, Added (date and by);
  - buttons for an open lead: Log contact, **Customer is in: Add case**, **+ New project**, Drop…;
  - a dropped lead shows **Bring back**; a converted lead shows "Moved to case #N" with a link, or "Its case was deleted." when `case_id` is null;
  - contact log newest first.
- LogContactDialog: response chips (4 labels), Remark (required), Next call (optional date). After saving a `not_interested` contact it opens `DropDialog` titled "Drop this lead?", which may be cancelled. Cancelling keeps the lead open.
- + New project: a small dialog with ProjectPicker and Assigned to, posting `/api/customers/{customer_id}/leads` and navigating to the new lead.

- [ ] **Step 1: Write failing tests:**
  - LeadDetail: `"shows customer name as the heading"`; `"logs a contact and shows it first"`; `"not interested offers drop and can be declined"` (no drop POST when cancelled); `"Add case converts and opens the case"`; `"dropped lead shows Bring back"`; `"converted lead links to its case"`.
  - AddCaseDialog: `"Logged in asks for tracking number"`; `"double tap sends one request"`; `"from lead shows project as text, not a picker"`.
- [ ] **Step 2: Run** `cd web && npm test -- LeadDetail.test.tsx AddCaseDialog.test.tsx` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the Step 2 command, the existing `CaseDetail.test.tsx` and `CustomerDetail.test.tsx`, and `npm run build` → PASS.
- [ ] **Step 5: Commit** `feat(web): lead page, contact log and Add case form`.

### Task 10: Web: case page customer-first, customer page and search show leads

**Files:**
- Modify: `web/src/pages/CaseDetail.tsx`, `web/src/pages/CustomerDetail.tsx`, `web/src/pages/Home.tsx` (search results)
- Test: `web/src/pages/CaseDetail.test.tsx`, `web/src/pages/CustomerDetail.test.tsx`, `web/src/pages/Home.test.tsx`

**Interfaces (mockup 5):**
- Case top card:
  - `h1` = customer name (`text-2xl font-bold`, linked to `/customers/{id}`);
  - the line under it: `tel:` mobile · bank · Case #id;
  - amount and stage badge on the right, progress bar below. The project is not in the top card.
- Details card: a `<dl>` at body text size with Project (change), Unit no. (change: a small dialog posting `/api/cases/{id}/unit`), Tracking no. (when set), Assigned to (change), Opened.
- A **Contact log** card (read-only, from `GET /api/leads/{lead_id}`) when `lead_id` is set.
- **+ New case for <first name>** opens `AddCaseDialog` for the same customer.
- Customer page: a **Leads (N)** section above Cases, rows like LeadList, plus a **+ New lead** button (same dialog as + New project).
- Search results: each group lists its open leads ("Lead · <project> · <last response>") linking to `/leads/{id}`, above its cases.

- [ ] **Step 1: Write failing tests:** `"case heading is the customer name"`; `"project shown once"` (`getAllByText("Green Villas")` has length 1); `"unit no. can be changed"`; `"contact log shows for a case from a lead"`; `"New case for Ravi opens the form"`; `"customer page lists leads above cases"`; `"search result shows the open lead"`.
- [ ] **Step 2: Run** `cd web && npm test -- CaseDetail.test.tsx CustomerDetail.test.tsx Home.test.tsx` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `npm test` (all) and `npm run build` → PASS.
- [ ] **Step 5: Commit** `feat(web): case page shows the customer first`.

### Task 11: Phone check, docs, and running app

**Files:**
- Modify: `web/e2e/phone.spec.ts` (mock `/api/leads`, `/api/leads/1`; add `/leads/1` to the 360 px no-scroll paths; extend the case mock with `lead_id` and `unit_number`), `docs/00-decisions.md` (record the round 15 decisions from the spec §8 and mark O2 superseded), `docs/database-schema.md` (leads, lead_contacts, new columns), `docs/superpowers/specs/2026-11-22-round-15-backlog.md` (mark tasks 1 and 2 built)

- [ ] **Step 1: Add the e2e cases**, then run them in the Playwright container → PASS. Signed-in `/leads/1` and `/cases/1042` have no horizontal scroll at 360 px.
- [ ] **Step 2: Update the docs** listed above.
- [ ] **Step 3: Run the full backend suite and all web tests** → PASS (except `TestAcceptance19`).
- [ ] **Step 4: Rebuild the local app**: `docker compose up --build -d db backend web`. Migration 5 applies on start. Check `curl -s -o /dev/null -w "%{http_code}" http://localhost:8081/` → 200, and that the owner's case-less customer now shows under Leads. Re-copy the mockups if the owner still needs them (`docker cp`).
- [ ] **Step 5: Commit** `docs: record round 15 leads decisions`.
