# UI Redesign and Role-Based Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the 9 form-style screens with 6 quieter screens (Login, Home, Customer, Case, Reports, Admin) shown by role, plus the server data they need.

**Architecture:** Server changes are small additions inside existing modules (`cases`, `reports`, `access`, `workflows`) with sqlc queries and chi routes in `modules/http`. The web app gets a `components/` folder of shared pieces; pages are rewritten in place, and Search, CaseList, NewCustomer and Dashboard pages are folded into Home and Reports.

**Tech Stack:** Go 1.25, chi, sqlc (pgx/v5), PostgreSQL, scs sessions; React 18, react-router 6, Tailwind 3, Vitest + Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-18-ui-redesign-design.md` (visual reference: `mockups/home-wide.html`, `customer.html`, `case.html`, `reports.html`, `admin.html`, `login.html`).

## Global Constraints

- Roles → menu / landing / Home default: admin → Home, Reports, Admin / `/admin` / All cases; manager → Home, Reports / `/` / All cases; employee → Home, Reports / `/` / My cases.
- Hiding pages is not security: every server permission check stays as it is.
- Needs attention rules: active case whose current stage `requires_document` and has 0 documents; or same stage for **7 or more days** and stage code is not `disbursed`. Dropped and completed cases never appear.
- Money is a decimal string end to end; dates display in India time (`Asia/Kolkata`).
- Passwords: 8+ characters (existing `minPasswordLen`).
- Home shows the first **50** rows, then "Show more" (adds 50).
- Search box debounce **300 ms**.
- Page width up to **1320px**; side panel at ≥1100px; phone layout at <760px; no horizontal scroll at 360px.
- No new npm or Go dependencies.
- **No commits** unless the owner asks (owner instruction). Work stays on branch `docs/project-foundation`.

**Commands**
- Go tests (no local Go; use compose): `docker compose run --rm --build test go test -count=1 ./modules/<pkg>/ -run <Name>` from repo root.
- sqlc: `cd backend && ~/go/bin/sqlc generate`.
- Web unit tests: `cd web && npx vitest run <file>`; all: `npm test`; types: `npx tsc -b`.
- Phone check: `cd web && npm run test:e2e`.

## Review Focus

1. A case whose bank has its own stages: progress bar and "next stage" must use that bank's stage list, not the standard one — Task 1 test `TestListRowsUsesBankStageCount`, Task 10 test "uses the bank's own stages".
2. A user whose password was reset by admin while signed in elsewhere must be signed out on the next request; but a user changing their **own** password must stay signed in — Task 5 tests.
3. New customer saved but first case fails (e.g. bad amount): customer must stay saved and the panel must link to them — Task 8 test "keeps the customer when the case fails".
4. Duplicate mobile in New customer: no second customer, no case created — Task 8 test "does not open a case for an existing mobile".
5. Non-admin typing `/admin` in the address bar lands on Home; admin signing in lands on `/admin` but can still open `/` — Task 7 tests.

---

### Task 1: Richer case list rows

**Files:**
- Modify: `backend/modules/cases/queries/cases.sql` (add `ListCaseRows`), regenerate `casesdb`
- Modify: `backend/modules/cases/list.go`
- Modify: `backend/modules/http/handlers.go` (`listCases`)
- Test: `backend/modules/cases/list_test.go`, `backend/modules/http/routes_test.go`

**Interfaces:**
- Produces: `type CaseRow struct { Case; CustomerName, CustomerMobile, BankName, StageName, StageCode string; StageOrder, StageCount int; AssigneeName string; StageSince time.Time; NeedsDocument bool }`
- Produces: `func (s *Service) ListRows(ctx context.Context, f ListFilter) ([]CaseRow, error)` (same filters and order as `List`).
- Produces JSON on `GET /api/cases`: existing `caseJSON` fields plus `customer_name, customer_mobile, bank_name, stage_name, stage_code, stage_order, stage_count, assignee_name, stage_since, needs_document`.

- [ ] **Step 1: Write failing tests** in `list_test.go`:
  - `TestListRowsCarriesNames`: using `twoCases`, row for A has `CustomerName` = fixture customer name, `BankName` = fixture bank, `StageName == "Received"`, `StageOrder == 1`, `StageCount == 6`, `AssigneeName` = fixture employee's full name, `StageSince` equal to the Received event time.
  - `TestListRowsUsesBankStageCount`: add 3 bank stages for SBI (`INSERT INTO stages (bank_id, code, name, sort_order) ...`), create a case at SBI; its row has `StageCount == 3`.
  - `TestListRowsNeedsDocument`: set Received `requires_document = true`; A has `NeedsDocument == true`; after inserting a `documents` row for A it is `false`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/cases/ -run TestListRows` → FAIL (`ListRows` undefined).
- [ ] **Step 3: Add query `ListCaseRows :many`**: same WHERE/ORDER as `ListCases`, joined to customers, banks, stages (current), users (assignee); `stage_count` = count of active stages for the bank if it has any, else of standard stages; `stage_since` = max `reached_at` of `case_stage_events` for the case and current stage; `needs_document` = `s.requires_document AND NOT EXISTS (documents for case)`. Run sqlc.
- [ ] **Step 4: Implement `ListRows`** and switch `listCases` handler to it; add the extra JSON keys (`stage_since` RFC3339).
- [ ] **Step 5: Add** a route test in `routes_test.go`: `GET /api/cases` as employee returns a row with non-empty `customer_name` and `stage_count` 6.
- [ ] **Step 6: Run** `docker compose run --rm --build test go test -count=1 ./modules/cases/ ./modules/http/` → PASS.

### Task 2: Needs attention

**Files:**
- Create: `backend/modules/cases/attention.go`, `backend/modules/cases/attention_test.go`
- Modify: `backend/modules/cases/queries/cases.sql` (add `AttentionCases`), `backend/modules/http/handlers.go`, `backend/modules/http/router.go`

**Interfaces:**
- Consumes: `CaseRow` (Task 1).
- Produces: `type AttentionItem struct { CaseRow; Reason string }`; `const StuckAfter = 7 * 24 * time.Hour`; `func (s *Service) Attention(ctx context.Context, assigneeID *int64, now time.Time) ([]AttentionItem, error)`.
- Produces: `GET /api/cases/attention?assignee_id=` → list of case-row JSON plus `reason`. Reasons (exact copy): `"No documents at this stage"`, `"In <Stage name> for <N> days"`. When both apply, the documents reason wins. Order: oldest `StageSince` first.

- [ ] **Step 1: Write failing tests** in `attention_test.go`:
  - `TestAttentionMissingDocument`: stage requires document, none uploaded → item with reason `"No documents at this stage"`.
  - `TestAttentionStuckSevenDays`: back-date the Received event by 7 days (`UPDATE case_stage_events SET reached_at = now() - interval '7 days'`) → reason `"In Received for 7 days"`; at 6 days → not listed.
  - `TestAttentionSkipsDroppedAndDisbursed`: dropped case back-dated 30 days and a disbursed case back-dated 30 days → neither listed.
  - `TestAttentionFiltersByAssignee`: with `assigneeID` of colleague, only colleague's case is returned.
- [ ] **Step 2: Run** `... ./modules/cases/ -run TestAttention` → FAIL.
- [ ] **Step 3: Implement** query (reuse the `ListCaseRows` select; WHERE `status = 'active'` AND (needs_document OR stage_since <= now - 7 days AND stage code <> 'disbursed')) and `Attention`, computing days as whole days from `StageSince` to `now`.
- [ ] **Step 4: Register** `r.Get("/api/cases/attention", h.caseAttention)` before `/api/cases/{id}`; add route test: employee gets 200 and a JSON array.
- [ ] **Step 5: Run** `... ./modules/cases/ ./modules/http/` → PASS.

### Task 3: Dashboard counts for the period and today's moves

**Files:**
- Modify: `backend/modules/reports/dashboard.go`, `backend/modules/reports/queries/reports.sql` (add `CountStageMoves`), `backend/modules/http/handlers.go` (`dashboard`)
- Test: `backend/modules/reports/dashboard_test.go` (existing file; add cases)

**Interfaces:**
- Produces: `DashboardData` gains `NewCustomers int`, `NewCases int` (in the period) and `StageMovesToday int` (stage events reached today IST, excluding each case's first Received event).
- Produces JSON keys `new_customers`, `new_cases`, `stage_moves_today` on `GET /api/reports/dashboard` (existing keys stay).
- Produces: `Dashboard(ctx, now, p Period, bankID *int64)`; when `bankID` is set, case counts, cases-by-stage and sanctions/disbursements count only that bank's cases (new-customer counts ignore bank). Handler reads `bank_id`.

- [ ] **Step 1: Write failing tests** `TestDashboardPeriodCounts` (customer created 10 days ago is counted for a 30-day period and not for a 5-day one), `TestDashboardStageMovesToday` (create a case and move it to Logged in today → `StageMovesToday == 1`) and `TestDashboardBankFilter` (cases at two banks; filtering by one bank counts only its cases).
- [ ] **Step 2: Run** `... ./modules/reports/ -run TestDashboard` → FAIL.
- [ ] **Step 3: Implement** using existing `CountNewCustomers` / `CountNewCases` with the period, and a new `CountStageMoves` query over `case_stage_events` joined to stages where code <> 'received'.
- [ ] **Step 4: Add JSON keys**; run `... ./modules/reports/ ./modules/http/` → PASS.

### Task 4: Team list for admin

**Files:**
- Modify: `backend/modules/access/queries/*.sql` (add `ListTeam`), `backend/modules/access/users.go`, `backend/modules/http/handlers.go`, `backend/modules/http/router.go`
- Test: `backend/modules/access/users_test.go`, `backend/modules/http/routes_test.go`

**Interfaces:**
- Produces: `type TeamMember struct { User; Email string; OpenCases int }`; `func (s *Service) ListTeam(ctx context.Context, actor User) ([]TeamMember, error)` → `ErrForbidden` unless `Can(actor.Role, ActionManageUsers)`. Includes inactive users; `OpenCases` counts cases with `status = 'active'` assigned to the user.
- Produces: `GET /api/admin/users` → `[{id, username, full_name, role, email, is_active, open_cases}]`.

- [ ] **Step 1: Failing tests**: `TestListTeamCountsOpenCases` (user with 2 active and 1 dropped case → `OpenCases == 2`); `TestListTeamAdminOnly` (manager → `ErrForbidden`); route test: employee gets 403, admin gets 200.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `... ./modules/access/ ./modules/http/` → PASS.

### Task 5: Admin password reset and signing out old sessions

**Files:**
- Modify: `backend/modules/access/users.go`, `backend/modules/access/session.go`, `backend/modules/access/login.go` (return `PasswordChangedAt` from `loadActive`), `backend/modules/http/handlers.go`, `backend/modules/http/router.go`
- Test: `backend/modules/access/users_test.go`, `backend/modules/access/middleware_test.go`, `backend/modules/access/change_password_test.go`

**Interfaces:**
- Produces: `func (s *Service) ResetPassword(ctx context.Context, actor User, userID int64, next string) error` → `ErrForbidden` unless `ActionManageUsers`; `ErrWeakPassword` under 8; `ErrUserNotFound`; audit action `"user.password_reset"`, entity `user`/userID.
- Produces: session key `sessionAuthAtKey = "auth_at"` (Unix seconds) set at login. `RequireUser` destroys the session and returns 401 when the user's `PasswordChangedAt` is after `auth_at`. `changePassword` handler re-puts `auth_at = now` after success so the user's own session survives.
- Produces: `POST /api/admin/users/{id}/password` body `{"password": "..."}` → 204.

- [ ] **Step 1: Failing tests**:
  - `TestResetPasswordAdminOnly` (manager → `ErrForbidden`), `TestResetPasswordWeak` (7 chars → `ErrWeakPassword`), `TestResetPasswordAudited`.
  - Middleware: `TestResetSignsOutOtherSessions` — user signs in (client A), admin resets password, client A's next `GET /api/me` → 401; signing in with the new password works.
  - `TestOwnPasswordChangeKeepsSession` — user changes own password, same client's `GET /api/me` → 200.
- [ ] **Step 2: Run** `... ./modules/access/ -run 'Reset|OwnPassword'` → FAIL.
- [ ] **Step 3: Implement**; compare at second precision (`auth_at < PasswordChangedAt.Unix()` → reject). Note: sessions created before this change have `auth_at = 0` and users with a `password_changed_at` will be asked to sign in once; acceptable.
- [ ] **Step 4: Route + route test** (employee 403, admin 204). **Step 5: Run** `... ./modules/access/ ./modules/http/` → PASS.

### Task 6: Rename bank and "own stages" flag

**Files:**
- Modify: `backend/modules/workflows/queries/workflows.sql` (`ListBanks` adds `has_own_stages`; add `RenameBank`), `backend/modules/workflows/workflows.go`, `backend/modules/http/handlers.go`, `backend/modules/http/router.go`
- Test: `backend/modules/workflows/workflows_test.go` (existing package tests), `backend/modules/http/routes_test.go`

**Interfaces:**
- Produces: `Bank` gains `HasOwnStages bool`; `func (s *Service) RenameBank(ctx context.Context, actor access.User, id int64, name string) (Bank, error)` → `ErrForbidden` unless `ActionManageConfig`; trims name; empty → existing empty-name error used by `CreateBank`; duplicate → existing duplicate error; audit `"bank.renamed"`.
- Produces: `PUT /api/admin/banks/{id}` body `{"name": "..."}` → bank JSON; `GET /api/config/banks` items gain `has_own_stages`.

- [ ] **Step 1: Failing tests**: `TestRenameBank`, `TestRenameBankDuplicate`, `TestRenameBankAdminOnly`, `TestListBanksHasOwnStages` (bank with one bank-specific stage → true, others false).
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement.** **Step 4: Run** `... ./modules/workflows/ ./modules/http/` → PASS.

### Task 7: Shared web pieces, role menu and landing page

**Files:**
- Create: `web/src/components/TopBar.tsx`, `Chip.tsx`, `ProgressBar.tsx`, `Dialog.tsx`, `SidePanel.tsx`, `Badge.tsx`, `ChangePasswordDialog.tsx`
- Create: `web/src/roles.ts`
- Modify: `web/src/App.tsx`, `web/src/api/types.ts`, `web/src/pages/Login.tsx` (restyle, call `onSignedIn` unchanged), `web/src/App.test.tsx`

**Interfaces:**
- Produces (`roles.ts`): `menuFor(role: Role): { to: string; label: string }[]`, `landingFor(role: Role): "/" | "/admin"`, `defaultScope(role: Role): "mine" | "all"`.
- Produces (`types.ts`): `CaseRow` (Task 1 JSON), `AttentionItem = CaseRow & { reason: string }`, `TeamMember`, `Bank` gains `has_own_stages?: boolean`, `DashboardData` with Task 3 keys.
- Produces components: `<ProgressBar done={number} total={number} />` (aria-label `"Stage {done} of {total}"`), `<Dialog open title onClose>{children}</Dialog>` (uses `<dialog>` semantics: `role="dialog"`, `aria-label={title}`), `<SidePanel open title onClose>`, `<Chip active onClick>`, `<Badge tone="info"|"ok"|"warn"|"bad"|"grey">`.
- Top bar: logo, `menuFor` links (nav `aria-label="Main"`), avatar button `aria-label="Account"` opening **Change password** and **Sign out**.

- [ ] **Step 1: Failing tests** in `App.test.tsx` (keep the three existing ones):
  - "admin lands on Admin after sign-in" (start at `/`, sign in via Login form as admin → Admin heading shown).
  - "admin can still open Home" (admin at `/` after landing → Home heading shown when clicking Home link).
  - "manager and employee see Home and Reports only".
  - "non-admin opening /admin is sent to Home".
  - "Change password dialog posts to /api/me/password".
- [ ] **Step 2: Run** `npx vitest run src/App.test.tsx` → FAIL.
- [ ] **Step 3: Implement** `roles.ts`, components, `TopBar`, routes: `/` Home, `/customers/:id`, `/cases/:id`, `/reports`, `/admin` (admin only), `*` → `/`. Remove `/cases`, `/customers/new`, `/dashboard` routes. Landing redirect happens only right after sign-in (Login `onSignedIn` → `navigate(landingFor(role))`), not on every visit to `/`.
- [ ] **Step 4: Run** `npx vitest run src/App.test.tsx && npx tsc -b` → PASS.

### Task 8: Home (replaces Search, Cases, New customer)

**Files:**
- Create: `web/src/pages/Home.tsx`, `web/src/pages/Home.test.tsx`, `web/src/pages/home/CaseTable.tsx`, `web/src/pages/home/TodayPanel.tsx`, `web/src/pages/home/NewCustomerPanel.tsx`
- Delete: `web/src/pages/Search.tsx`, `CaseList.tsx`, `NewCustomer.tsx` and their tests (their assertions move into `Home.test.tsx` where the behaviour remains: grouped search, filters, duplicate mobile)

**Interfaces:**
- Consumes: `CaseRow`, `AttentionItem`, `defaultScope`, components from Task 7; `GET /api/cases`, `/api/cases/attention`, `/api/search`, `/api/reports/dashboard`, `/api/config/banks`, `/api/config/banks/{id}/stages`, `/api/users`, `POST /api/customers`, `POST /api/cases`.
- Produces: `export function Home({ user }: { user: SignedInUser })`.

- [ ] **Step 1: Failing tests** in `Home.test.tsx`:
  - "employee starts on My cases" (request to `/api/cases` has `assignee_id=<me>`); "manager starts on All cases" (no `assignee_id`).
  - "shows customer, bank, stage and amount in each row" and the progress bar `"Stage 3 of 6"`.
  - "Needs attention chip lists attention cases with their reason".
  - "typing searches after a pause and shows grouped results" (fake timers, 300 ms).
  - "Show more reveals the next 50" (60 rows → 50 visible, then 60).
  - New customer panel: "creates customer then case and opens the case"; "keeps the customer when the case fails" (case POST 400 → status message with link to customer); "does not open a case for an existing mobile" (response `existing` → link shown, no `POST /api/cases`); "saves just the customer when bank is empty".
- [ ] **Step 2: Run** `npx vitest run src/pages/Home.test.tsx` → FAIL.
- [ ] **Step 3: Implement** to match `mockups/home-wide.html` (columns ≥760px, compact rows below; Today and Needs attention side panel ≥1100px, above the list 760–1099px, hidden <760px). Chips: My cases / All cases / Needs attention with counts; Bank, Stage, Assigned to, Date as small popover chips.
- [ ] **Step 4: Run** `npx vitest run src/pages/Home.test.tsx && npx tsc -b` → PASS.

### Task 9: Customer page

**Files:**
- Modify: `web/src/pages/CustomerDetail.tsx`; Test: `web/src/pages/CustomerDetail.test.tsx` (create)

**Interfaces:**
- Consumes: `PUT /api/customers/{id}`, `POST /api/customers/{id}/drop`, `POST /api/cases`, `GET /api/cases?...` is not needed (customer JSON already has `cases`); Task 7 components.

- [ ] **Step 1: Failing tests**: header shows name, mobile, email and a `tel:` Call link; "Edit saves name, mobile, email"; "New case dialog opens a case"; "Drop customer requires a reason; Other requires a remark"; New case and Drop buttons hidden for a dropped customer.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** per `mockups/customer.html`. **Step 4: Run** `npx vitest run src/pages/CustomerDetail.test.tsx` → PASS.

### Task 10: Case page

**Files:**
- Modify: `web/src/pages/CaseDetail.tsx`, `web/src/pages/CaseDetail.test.tsx`; Create: `web/src/pages/case/MoveStageDialog.tsx`

**Interfaces:**
- Consumes: existing case endpoints; `POST /api/cases/{id}/stage` body `{stage_id, tracking_number?, sanction?: {amount, rate_percent, interest_type, tenure_months, date}, disbursement?: {amount, date}}`; `POST /api/cases/{id}/assign` `{assignee_id}`.
- Produces: `MoveStageDialog({ stages, current, target, onMoved })`.

- [ ] **Step 1: Failing tests** (keep existing assertions that still apply: history shown, upload, drop, missing-document warning, server warning):
  - "Move to next stage button names the next stage" (`Move to Sanctioned →` when current is Under sanction).
  - "uses the bank's own stages" (bank stage list of 3 → next button names stage 2 of that list).
  - "Logged in asks for tracking number"; "Sanctioned asks for sanction details and posts them"; "Disbursed asks for amount and date".
  - "Other stage lets you pick any stage, including going back".
  - "change assignee posts to /assign".
  - No move or drop controls on a dropped case.
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** per `mockups/case.html`. **Step 4: Run** `npx vitest run src/pages/CaseDetail.test.tsx` → PASS.

### Task 11: Reports (merges Dashboard)

**Files:**
- Modify: `web/src/pages/Reports.tsx`, `web/src/pages/Reports.test.tsx`
- Delete: `web/src/pages/Dashboard.tsx`, `Dashboard.test.tsx` (tile assertions move to Reports)

**Interfaces:**
- Consumes: `GET /api/reports/dashboard?from&to&bank_id`, `/api/reports/rows`, `/api/reports/export.csv`.
- Period chips map to IST dates: Today = today..today; This week = Monday..today; This month = 1st..today (default); Custom shows two date inputs.

- [ ] **Step 1: Failing tests**: "defaults to This month and sends from/to"; "tiles show period counts with today below"; "stage bars list open cases by stage"; "Export CSV link carries the same filters"; "Custom shows date inputs".
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** per `mockups/reports.html`. **Step 4: Run** `npx vitest run src/pages/Reports.test.tsx` → PASS.

### Task 12: Admin tabs

**Files:**
- Modify: `web/src/pages/admin/AdminHome.tsx`, `Stages.tsx`, `Archive.tsx`; Create: `web/src/pages/admin/Team.tsx`, `BanksAndLists.tsx`, `web/src/pages/admin/AdminHome.test.tsx`; keep `Stages.test.tsx`, `Archive.test.tsx` passing.

**Interfaces:**
- Consumes: `GET /api/admin/users`, `POST /api/admin/users`, `POST /api/admin/users/{id}/password`, `GET /api/config/banks`, `POST /api/admin/banks`, `PUT /api/admin/banks/{id}`, `GET /api/config/dropdowns/drop_reason`, `POST /api/admin/dropdowns` (category fixed to `drop_reason` in the UI).
- Tabs: Team (default), Banks & lists, Stages, Archive; tab in the URL hash (`#team`, `#banks`, `#stages`, `#archive`).

- [ ] **Step 1: Failing tests**: "Team lists users with role and open cases"; "Add person dialog posts and refreshes the list"; "Reset password posts the new password and confirms"; "Banks tab shows Own stages / Standard stages"; "Rename bank"; "Add drop reason"; "tab follows the URL hash".
- [ ] **Step 2: Run** → FAIL. **Step 3: Implement** per `mockups/admin.html`. **Step 4: Run** `npx vitest run src/pages/admin/` → PASS.

### Task 13: Phone check and full verification

**Files:**
- Modify: `web/e2e/phone.spec.ts`

- [ ] **Step 1: Add** an e2e test: signed-in Home at 360px has no horizontal scroll (reuse the existing login flow in the spec file; if the e2e setup has no signed-in fixture, mock `/api/me` and `/api/cases` with `page.route`).
- [ ] **Step 2: Run** `npm run test:e2e` → PASS.
- [ ] **Step 3: Run everything**: `docker compose run --rm --build test` → all Go tests PASS; `cd web && npm test && npx tsc -b && npm run build` → PASS.
- [ ] **Step 4: Manual check** with `docker compose up --build`: sign in as admin (lands on Admin), as manager and employee (Home, correct default chip); move a case through every stage once.
