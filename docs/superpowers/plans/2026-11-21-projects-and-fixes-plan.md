# Projects, Extra Cases, Tracking Rule — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add admin-managed projects (every case belongs to one, NA for none) with a Projects page and CSV, let staff add a case to an existing (or dropped) customer from New customer, and enforce letters-and-digits, upper-case tracking numbers.

**Architecture:** Projects follow the bank pattern exactly: a `projects` table, service methods in `modules/workflows`, config/admin routes, and `cases.project_id NOT NULL`. Project filtering is one more optional ID in the existing case, report and dashboard filters. Bringing back a dropped customer is a customers-package helper that runs inside the case-creation transaction.

**Tech Stack:** Go 1.25, chi, pgx/v5, sqlc, golang-migrate, PostgreSQL 16; React 18 + TypeScript, Vite, Tailwind, Vitest + Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-21-projects-and-fixes-design.md`

## Global Constraints

- NA is a seeded project with `is_system = true`; it is always listed first and can never be renamed.
- Project names: trimmed, not empty, unique ignoring case and surrounding spaces. Duplicate message: "A project with this name already exists."
- Every case has exactly one project (`cases.project_id NOT NULL`). Existing cases are set to NA by the migration.
- The project picker starts empty in every new-case form; empty submit shows "Choose a project, or NA."
- Search never matches NA by project name.
- Tracking number: trim, upper-case, then must match `^[A-Z0-9]+$`. Refusal message: "Tracking number can contain only letters and numbers." Existing stored values are not rewritten.
- Only admin adds or renames projects (`access.ActionManageConfig`), enforced in the service.
- Projects page and project lists are readable by every signed-in role.
- Error texts follow the existing pattern (`errors.New("<pkg>: <message>")`, shown through `userMessage`), mapped in `modules/http/errors.go`.
- Generated sqlc code is committed: after editing any `queries/*.sql` or migration, run `sqlc generate` in `backend/`.
- **Commits:** the working tree holds uncommitted v1 work. Run the commit steps only after the owner has said to commit (to the feature branch). Otherwise skip them.

**Commands** (from repo root):
- Backend tests: `docker compose run --rm --build test go test -count=1 ./modules/<pkg>/ -run <Name> -v` (full suite: `docker compose run --rm --build test`).
- sqlc: `cd backend && PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH" sqlc generate`.
- Web: `cd web && npm test -- <file>`; build `npm run build`; phone check `npm run test:e2e`.

## Review Focus

1. **Typing a known mobile whose customer is dropped, then pressing the button twice** (double tap on phone): must create one case and one reactivation, not two. The button is disabled while busy. Test in Task 7.
2. **Renaming a project to the same name with different case or spaces** (`"Prestige "` → `"prestige"`) must be refused as a duplicate, and renaming a project to its own exact name must succeed. Test in Task 1.
3. **Tracking number pasted with spaces inside** (`HDFC 123`) must be refused, not silently squashed, and lower case typed in the dialog shows as capitals immediately. Tests in Tasks 4 and 8.
4. **Search text that is part of "NA"** (`na`, `Na`) must not return every NA case; a real project name containing "na" (e.g. "Nandanam Heights") still matches. Test in Task 3.
5. **Restoring a case archived before this change** (snapshot without `project_id`) must succeed with NA, not fail the NOT NULL rule. Test in Task 5.

---

### Task 1: Projects table, service and admin API

**Files:**
- Create: `backend/migrations/000004_projects.up.sql`, `backend/migrations/000004_projects.down.sql`
- Modify: `backend/modules/workflows/queries/*.sql` (add project queries), `backend/modules/workflows/workflows.go` (or create `backend/modules/workflows/projects.go`), `backend/modules/http/router.go`, `backend/modules/http/handlers.go`, `backend/modules/http/errors.go`
- Test: `backend/modules/workflows/projects_test.go`, `backend/migrations/migrate_test.go`, `backend/modules/http/routes_test.go`

**Interfaces:**
- Produces (Go, package `workflows`):
  - `type Project struct { ID int64; Name string; IsSystem bool; OpenCases int64; TotalCases int64 }`
  - `func (s *Service) ListProjects(ctx context.Context) ([]Project, error)` — NA first, then by name; counts: open = `status = 'active'`.
  - `func (s *Service) CreateProject(ctx context.Context, actor access.User, name string) (Project, error)`
  - `func (s *Service) RenameProject(ctx context.Context, actor access.User, id int64, name string) (Project, error)`
  - `func (s *Service) ProjectExists(ctx context.Context, id int64) (bool, error)`
  - Errors: `ErrProjectNameRequired`, `ErrProjectNameTaken` ("…a project with this name already exists"), `ErrProjectNotFound`, `ErrProjectLocked` (rename NA).
- Produces (HTTP): `GET /api/config/projects` → `[{id, name, is_system, open_cases, total_cases}]`; `POST /api/admin/projects {name}`; `PUT /api/admin/projects/{id} {name}`. Audit actions `project.created`, `project.renamed`.
- Produces (SQL): table `projects(id, name, is_system, created_at, created_by, updated_at, updated_by)`, unique index on `lower(btrim(name))`, `cases.project_id bigint NOT NULL REFERENCES projects(id)` with index `cases_project_idx`.

- [ ] **Step 1: Write failing tests** in `projects_test.go` (use the `setup`/`actor` helpers from `banks_test.go`):
  - `TestProjectsNAFirst`: fresh DB → `ListProjects` returns one project, `Name == "NA"`, `IsSystem == true`; after creating "Zenith" and "Akshaya", order is NA, Akshaya, Zenith.
  - `TestCreateProjectTrimsAndRefusesDuplicate`: create `"  Prestige  "` → name `"Prestige"`; create `"prestige"` → `ErrProjectNameTaken`; create `"   "` → `ErrProjectNameRequired`.
  - `TestRenameProject`: rename Prestige → `"Prestige Lakeside"` ok; rename to its own name ok; rename another project to `"PRESTIGE LAKESIDE "` → `ErrProjectNameTaken`; rename NA → `ErrProjectLocked`; unknown id → `ErrProjectNotFound`.
  - `TestProjectAdminOnly`: manager and employee get `access.ErrForbidden` on create and rename.
  - `TestProjectCounts`: insert one active and one dropped case on a project (direct SQL is fine) → `OpenCases == 1`, `TotalCases == 2`.
  - In `migrate_test.go`, `TestMigration4SetsExistingCasesToNA`: migrate to version 3, insert a customer and a case, migrate up, assert the case's `project_id` is the NA id.
  - In `routes_test.go`: employee `GET /api/config/projects` → 200; employee `POST /api/admin/projects` → 403; admin duplicate → 409 with the duplicate message.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/workflows/ ./migrations/ ./modules/http/ -run 'Project|Migration4' -v` → FAIL (undefined).
- [ ] **Step 3: Write migration 000004.** Up: create table and unique index; insert `('NA', true)`; add `cases.project_id` nullable; `UPDATE cases SET project_id = (SELECT id FROM projects WHERE is_system)`; `SET NOT NULL`; create index. Grant table privileges the same way `000002_audit_grants.up.sql` does for app tables. Down: drop column, drop table.
- [ ] **Step 4: Add queries** `ListProjects`, `InsertProject`, `RenameProject` (`WHERE id = $1 AND NOT is_system`; distinguish not-found vs locked with a `GetProject` lookup), `ProjectExists`; run `sqlc generate`.
- [ ] **Step 5: Implement the service methods** following `CreateBank`/`RenameBank` (trim, permission check first, map 23505 to `ErrProjectNameTaken`, audit). Add handlers and routes; map errors in `statusFor` (taken → 409, required → 400, not found → 404, locked → 409).
- [ ] **Step 6: Run** the Step 2 command → PASS. Then the full backend suite → PASS (existing case inserts in other tests will now fail on NOT NULL until Task 2; if so, continue to Task 2 before running the full suite, and note it).
- [ ] **Step 7: Commit** (see Global Constraints) `feat: add projects with NA and admin API`.

### Task 2: Project on every case (create, change, read)

**Files:**
- Modify: `backend/modules/cases/create.go`, `backend/modules/cases/queries/cases.sql`, `backend/modules/cases/list.go`, `backend/modules/http/handlers.go`, `backend/modules/http/router.go`, `backend/modules/http/errors.go`, existing test helpers that insert cases (`backend/modules/*/…_test.go`, `backend/acceptance/`)
- Create: `backend/modules/cases/project.go`
- Test: `backend/modules/cases/project_test.go`, `backend/modules/cases/create_test.go`

**Interfaces:**
- Consumes: `projects` table and NA row from Task 1.
- Produces:
  - `NewCase` gains `ProjectID int64`; `Case` gains `ProjectID int64`; `CaseRow` gains `ProjectName string`.
  - `ErrProjectRequired` ("…choose a project, or NA"), `ErrProjectNotFound` (in `cases`).
  - `func (s *Service) ChangeProject(ctx context.Context, actor access.User, caseID, projectID int64) (Case, error)` — `ActionEditRecord`; refused on a dropped case with the existing `ErrCaseDropped`; audit `case.project_changed` with `Details: map[string]int64{"from": old, "to": new}`; no-op (no audit) when unchanged.
  - JSON: `caseJSON` and case rows include `project_id` and `project_name`. `POST /api/cases` takes `project_id`. New route `POST /api/cases/{id}/project {project_id}`.

- [ ] **Step 1: Failing tests** in `project_test.go` / `create_test.go`:
  - `TestCreateCaseRequiresProject`: `ProjectID: 0` → `ErrProjectRequired`; `ProjectID: 999999` → `ErrProjectNotFound`; valid → returned `Case.ProjectID` equals it.
  - `TestChangeProjectAudited`: change NA → Prestige → `ProjectID` updated and one `audit_log` row `case.project_changed` with details `{"from": naID, "to": prestigeID}`; changing to the same project writes no audit row.
  - `TestChangeProjectDroppedCase` → `ErrCaseDropped`.
  - `TestListRowsHasProjectName`: `ListRows` row has `ProjectName == "Prestige"`.
- [ ] **Step 2: Run** `… go test -count=1 ./modules/cases/ -run 'Project' -v` → FAIL.
- [ ] **Step 3: Implement.** Add `project_id` to `InsertCase`, `GetCase`, `ListCases`, `ListCaseRows` (join `projects`), `CasesForCustomer`; add `UpdateCaseProject`; `sqlc generate`. Validate project in `Create` before the transaction. Update every test helper and acceptance fixture that inserts a case to pass the NA id (add a shared helper per package such as `naProjectID(t, pool) int64` if one doesn't exist).
- [ ] **Step 4: Run** the Step 2 command, then the full backend suite → PASS.
- [ ] **Step 5: Commit** `feat: require a project on every case`.

### Task 3: Project in filters, reports, CSV and search

**Files:**
- Modify: `backend/modules/cases/list.go`, `backend/modules/cases/search.go`, `backend/modules/cases/queries/cases.sql`, `backend/modules/reports/rows.go`, `backend/modules/reports/csv.go`, `backend/modules/reports/dashboard.go`, `backend/modules/reports/queries/*.sql`, `backend/modules/http/handlers.go` (`caseFilter`, `dashboard`, `reportRows`, `exportCSV`)
- Test: `backend/modules/cases/list_test.go`, `backend/modules/cases/search_test.go`, `backend/modules/reports/csv_test.go`, `backend/modules/reports/dashboard_test.go`

**Interfaces:**
- Consumes: `Case.ProjectID`, `CaseRow.ProjectName` (Task 2).
- Produces: `ListFilter.ProjectID *int64`, `reports.RowFilter.ProjectID *int64`, `reports.Period.ProjectID *int64`, `reports.Row.Project string`; query parameter `project_id` on `/api/cases`, `/api/reports/dashboard`, `/api/reports/rows`, `/api/reports/export.csv`. CSV `Header` becomes `"Case ID","Customer","Mobile","Bank","Project","Stage",…` (Project right after Bank).

- [ ] **Step 1: Failing tests:**
  - `TestListFilterByProject`: two cases on different projects → filter returns only the matching one.
  - `TestSearchByProjectName`: case on "Nandanam Heights" → search `"nandanam"` finds its customer; customer "Ravi" with only an NA case → search `"na"` does not return Ravi (a customer named "Nandini" still matches by name).
  - `TestCSVHasProjectColumn`: `Header[4] == "Project"` and the row's 5th field is the project name.
  - `TestDashboardProjectFilter`: new-case count with `ProjectID` set counts only that project's cases.
- [ ] **Step 2: Run** `… go test -count=1 ./modules/cases/ ./modules/reports/ -run 'Project' -v` → FAIL.
- [ ] **Step 3: Implement.** Add `sqlc.narg(project_id)` conditions next to every `bank_id` filter. In `SearchCustomerIDs` add `LEFT JOIN projects p ON p.id = k.project_id` and `OR (p.name ILIKE sqlc.arg(name_pattern) AND NOT p.is_system)`. Parse `project_id` in `caseFilter` and the dashboard handler. `sqlc generate`.
- [ ] **Step 4: Run** Step 2 command, then full backend suite → PASS.
- [ ] **Step 5: Commit** `feat: filter, report, export and search by project`.

### Task 4: Tracking number rule

**Files:**
- Create: `backend/modules/cases/trackingnum.go`
- Modify: `backend/modules/cases/stage.go`, `backend/modules/cases/search.go`, `backend/modules/http/errors.go`
- Test: `backend/modules/cases/trackingnum_test.go`, `backend/modules/cases/tracking_test.go`

**Interfaces:**
- Produces: `func NormalizeTracking(s string) (string, error)` — returns `""` with nil error for blank input (callers decide if required); `ErrInvalidTracking` with text "…tracking number can contain only letters and numbers" (400).

- [ ] **Step 1: Failing tests:** `TestNormalizeTracking` table: `"hdfc123"→"HDFC123"`, `"  sbi9 "→"SBI9"`, `""→""`, `"HDFC 123"`, `"hdfc-123"`, `"A/1"`, `"१२३"` (non-ASCII digits) → `ErrInvalidTracking`. In `tracking_test.go`: `TestLoggedInStoresUpperTracking` (move with `"ab12"` → stored `"AB12"`); `TestDuplicateTrackingIgnoresCase` (case 1 `"AB12"`, case 2 moving with `"ab12"` → `ErrDuplicateTracking`); `TestSearchTrackingLowercase` (`"ab12"` finds the customer).
- [ ] **Step 2: Run** `… go test -count=1 ./modules/cases/ -run 'Tracking' -v` → FAIL.
- [ ] **Step 3: Implement.** Call `NormalizeTracking` in `ChangeStage` where `newTracking` is built (stage.go ~line 110), before the required check. In `Search`, pass `strings.ToUpper(text)` as `Tracking`.
- [ ] **Step 4: Run** Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat: tracking number letters and digits, upper case`.

### Task 5: Bring back a dropped customer; restore with NA

**Files:**
- Create: `backend/modules/customers/reactivate.go`
- Modify: `backend/modules/customers/queries/customers.sql`, `backend/modules/cases/create.go`, `backend/modules/archive/restore.go`, `backend/modules/http/handlers.go`, `backend/modules/http/router.go`, `backend/modules/http/errors.go`
- Test: `backend/modules/customers/reactivate_test.go`, `backend/modules/cases/create_test.go`, `backend/modules/archive/restore_test.go`

**Interfaces:**
- Produces (package `customers`):
  - `func ReactivateTx(ctx context.Context, tx pgx.Tx, rec history.Recorder, actor access.User, customerID int64) error` — clears `status/dropped_reason_id/dropped_remark/dropped_at/dropped_by`, sets `updated_*`, audits `customer.reactivated` with the previous drop values in `Details`. Returns `ErrNotDropped` if the customer is active.
  - `func (s *Service) Reactivate(ctx context.Context, actor access.User, customerID int64) (Customer, error)` — `ActionAddRecord`; wraps `ReactivateTx` in its own transaction.
- Produces (package `cases`): `NewCase.BringBack bool`; `ErrCustomerDropped` ("…this customer was dropped. Bring them back first.", 409). `Create` locks the customer row (`SELECT status … FOR UPDATE`) inside its transaction; dropped + `!BringBack` → `ErrCustomerDropped`; dropped + `BringBack` → `customers.ReactivateTx` in the same transaction; active + `BringBack` → proceed normally.
- Produces (HTTP): `POST /api/customers/{id}/reactivate` → customer JSON; `POST /api/cases` accepts `bring_back`.

- [ ] **Step 1: Failing tests:**
  - `TestReactivateClearsDrop`: dropped customer → status `active`, drop fields NULL, one `customer.reactivated` audit with the old reason id; earlier dropped cases still `dropped`.
  - `TestReactivateActiveCustomer` → `ErrNotDropped`.
  - `TestCreateCaseDroppedCustomer`: without `BringBack` → `ErrCustomerDropped` and no case row; with `BringBack` → case created and customer active.
  - `TestCreateCaseBringBackTwice` (Review Focus 1): two `Create` calls with `BringBack` run one after another → two cases (allowed, staff may open two), exactly one `customer.reactivated` audit row.
  - `TestRestoreOldCaseGetsNA` (Review Focus 5): archive snapshot JSON without `project_id` → restore succeeds, case has NA id.
- [ ] **Step 2: Run** `… go test -count=1 ./modules/customers/ ./modules/cases/ ./modules/archive/ -run 'Reactivate|Dropped|BringBack|OldCase' -v` → FAIL.
- [ ] **Step 3: Implement.** Add `ReactivateCustomer` query; in `restoreCase`, before the `INSERT … json_populate_record`, set `project_id` in the case JSON to the NA id when it is missing or null (the insert would otherwise fail NOT NULL). `sqlc generate`.
- [ ] **Step 4: Run** Step 2 command, then full backend suite → PASS.
- [ ] **Step 5: Commit** `feat: bring back dropped customers when adding a case`.

### Task 6: Web — project types, picker, admin Projects list

**Files:**
- Create: `web/src/components/ProjectPicker.tsx`, `web/src/components/ProjectPicker.test.tsx`
- Modify: `web/src/api/types.ts`, `web/src/pages/admin/BanksAndLists.tsx`
- Test: `web/src/pages/admin/BanksAndLists.test.tsx` (create)

**Interfaces:**
- Consumes: `GET /api/config/projects`, `POST/PUT /api/admin/projects` (Task 1).
- Produces:
  - `export type Project = { id: number; name: string; is_system: boolean; open_cases: number; total_cases: number }`; `Case` and `CaseRow` gain `project_id: number; project_name: string`.
  - `export function ProjectPicker(props: { projects: Project[]; value: number | null; onChange: (id: number | null) => void; label?: string; allowAll?: boolean; "aria-label"?: string }): JSX.Element` — text input with a list; filters by case-insensitive "contains"; the system project is always the first option whatever is typed; with `allowAll`, an "All projects" option (value `null`) comes before NA; only list items can be chosen (typing alone leaves `value` unchanged); keyboard: arrows move, Enter picks, Escape closes.

- [ ] **Step 1: Failing tests** (`ProjectPicker.test.tsx`): typing `"pre"` shows NA then "Prestige", not "Zenith"; typing `"zzz"` still shows NA; clicking "Prestige" calls `onChange(prestigeId)`; typing without picking leaves `onChange` uncalled; `allowAll` shows "All projects" first. `BanksAndLists.test.tsx`: Projects list shows NA first with no Rename button; Add project posts `{name}`; a 409 shows "A project with this name already exists."
- [ ] **Step 2: Run** `cd web && npm test -- ProjectPicker BanksAndLists` → FAIL.
- [ ] **Step 3: Implement** the picker with existing `input` styles from `components/ui.ts`, and a Projects section in `BanksAndLists.tsx` mirroring the Banks section (name, open cases, Rename; reuse the existing name `Dialog`).
- [ ] **Step 4: Run** Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat(web): project picker and admin projects list`.

### Task 7: Web — project on new cases; existing-customer and bring-back flow

**Files:**
- Modify: `web/src/pages/home/NewCustomerPanel.tsx`, `web/src/pages/CustomerDetail.tsx`, `web/src/pages/Home.tsx` (load projects and pass them down)
- Test: `web/src/pages/NewCase.test.tsx`, `web/src/pages/CustomerDetail.test.tsx`

**Interfaces:**
- Consumes: `ProjectPicker`, `Project` (Task 6); `POST /api/cases` with `project_id`, `bring_back`; `POST /api/customers/{id}/reactivate` (Tasks 2, 5). `POST /api/customers` already returns `{existing}` including `status`.
- Produces: `NewCustomerPanel` props gain `projects: Project[]`.

- [ ] **Step 1: Failing tests:**
  - New customer with a bank and no project → shows "Choose a project, or NA." and sends no request.
  - Known active mobile with bank and project chosen → warning shows button "Add this case to Asha"; clicking posts `/api/cases` with `customer_id` of Asha, the typed bank, project, amount, assignee, `bring_back: false`, then navigates to `/cases/:id`.
  - Known mobile, no bank → button "Open Asha" links to `/customers/:id`.
  - Known **dropped** mobile with bank → button "Bring back Asha and add this case" posts with `bring_back: true`; with no bank → "Bring back Asha" posts `/api/customers/:id/reactivate` then goes to the customer page.
  - Button is disabled while the request runs (Review Focus 1); a server error shows in the panel and the typed values remain.
  - Customer page: New case dialog requires a project; a dropped customer shows **Bring back** instead of **+ New case**, which calls reactivate and reloads.
- [ ] **Step 2: Run** `cd web && npm test -- NewCase CustomerDetail` → FAIL.
- [ ] **Step 3: Implement.** Add the picker to both case forms (empty by default). Client-side empty check before posting.
- [ ] **Step 4: Run** Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat(web): project on new cases, add case to existing customer`.

### Task 8: Web — project on case and customer pages; tracking input

**Files:**
- Modify: `web/src/pages/CaseDetail.tsx`, `web/src/pages/CustomerDetail.tsx` (case list line), `web/src/pages/case/MoveStageDialog.tsx`
- Test: `web/src/pages/CaseDetail.test.tsx`, `web/src/pages/CustomerDetail.test.tsx`

**Interfaces:**
- Consumes: `POST /api/cases/{id}/project` (Task 2), `ProjectPicker` (Task 6). There is no general case-edit dialog in the web app today, so the project change is its own small dialog (spec §4.3 "case edit" is met by this).

- [ ] **Step 1: Failing tests:** case header shows "SBI · Prestige"; "Change project" dialog posts `{project_id}` and shows the new name; customer case list item reads "SBI · Prestige · Case #12"; in MoveStageDialog, typing `"ab-12"` shows `"AB-12"` and the message "Tracking number can contain only letters and numbers." with Save disabled; typing `"ab12"` shows `"AB12"` and sends `tracking_number: "AB12"`.
- [ ] **Step 2: Run** `cd web && npm test -- CaseDetail CustomerDetail` → FAIL.
- [ ] **Step 3: Implement.** Upper-case on change (`value.toUpperCase()`); validate trimmed value against `/^[A-Z0-9]+$/` when not empty.
- [ ] **Step 4: Run** Step 2 command → PASS.
- [ ] **Step 5: Commit** `feat(web): show and change project; tracking input rule`.

### Task 9: Web — Projects page, top bar, filters on Home and Reports

**Files:**
- Create: `web/src/pages/Projects.tsx`, `web/src/pages/ProjectDetail.tsx`, `web/src/pages/Projects.test.tsx`
- Modify: `web/src/roles.ts`, `web/src/App.tsx`, `web/src/pages/Home.tsx`, `web/src/pages/home/CaseTable.tsx`, `web/src/pages/Reports.tsx`, `web/src/App.test.tsx`, `web/src/pages/Home.test.tsx`, `web/src/pages/Reports.test.tsx`, `web/e2e/*` (phone check)

**Interfaces:**
- Consumes: `GET /api/config/projects`, `GET /api/cases?project_id=`, `/api/reports/export.csv?project_id=` (Tasks 1, 3), `CaseTable` (existing), `ProjectPicker` with `allowAll` (Task 6).
- Produces: routes `/projects` and `/projects/:id`; `menuFor` returns Home, Projects, Reports (+ Admin for admin) for every role.

- [ ] **Step 1: Failing tests:** `menuFor("employee")` labels are `["Home","Projects","Reports"]`; `/projects` lists NA first with open and total counts, and the search box narrows the list; `/projects/:id` requests `/api/cases?project_id=<id>` (no assignee filter by default, for every role) and its Download CSV link is `/api/reports/export.csv?project_id=<id>`; Home and Reports send `project_id` when a project filter is chosen; CaseTable shows a Project column. Playwright at 360 px: `/projects` and the picker fit without horizontal scroll.
- [ ] **Step 2: Run** `cd web && npm test -- Projects App Home Reports` → FAIL.
- [ ] **Step 3: Implement** the two pages with the existing `page`/`card` styles and `CaseTable`; add the project filter next to Bank on Home and Reports.
- [ ] **Step 4: Run** `cd web && npm test && npm run build && npm run test:e2e` → all PASS.
- [ ] **Step 5: Commit** `feat(web): projects page, menu item and project filters`.

### Task 10: End-to-end check and docs

**Files:**
- Modify: `docs/local-testing.md` ("What to try by hand": projects, existing mobile, bring back, tracking rule)

- [ ] **Step 1: Run** full backend suite `docker compose run --rm --build test` → PASS except the known `TestAcceptance19` (backup scripts not in the test container); record the result.
- [ ] **Step 2: Rebuild and start** `docker compose up --build -d db backend web`; confirm the migration ran (`docker compose logs backend`) and existing cases show project NA.
- [ ] **Step 3: Click through** on desktop and at phone width: add project, open case with project, known mobile → add case, dropped customer → bring back, tracking `ab12` → `AB12`, `ab-12` refused, Projects page CSV opens in Excel.
- [ ] **Step 4: Update** `docs/local-testing.md`.
- [ ] **Step 5: Commit** `docs: local testing for projects and fixes`.
