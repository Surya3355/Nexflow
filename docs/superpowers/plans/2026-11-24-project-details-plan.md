# Project Details and Builder Contacts: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin records each project's type, address and its own list of builder contacts; every lead and case points to one contact of its project, shown with tap-to-call and changeable by staff.

**Architecture:** Migration 000006 adds `projects.type/address`, a `project_contacts` table, and `builder_contact_id` on `leads` and `cases` with a composite FK `(project_id, builder_contact_id)` so the database refuses another project's contact. `modules/workflows` owns projects and contacts (create and edit as one whole-form save in one transaction, project row locked). `modules/leads` and `modules/cases` read `project_contacts` through their own sqlc queries to prefill and validate the contact. The web gets a project form in Admin, details on the project page, and one shared contact card + picker on the lead page, case page and Add case.

**Tech Stack:** Go 1.25, chi, pgx/v5, sqlc, golang-migrate, PostgreSQL 16; React 18 + TypeScript, Vite, Tailwind, Vitest + Testing Library, Playwright.

**Spec:** `docs/superpowers/specs/2026-11-24-project-details-design.md`

## Global Constraints

- Project types (stored → label, in this order): `high_rise` → High-rise apartments, `apartments` → Apartments, `plots` → Plots, `villas` → Villas. Fixed in code; no admin editing.
- Every save (create or edit) of a non-NA project needs: name, type, address (trimmed, not empty) and at least one active contact. Old incomplete projects stay usable for leads and cases, but cannot be saved incomplete.
- NA (`is_system`) cannot be edited and never has type, address or contacts.
- Contact: name required (trimmed), phone required via `customers.NormalizeMobile` (10 digits), email optional (trimmed, no format check, same as customers), label optional (trimmed, ≤ 40 chars). Same phone twice among one project's **active** contacts is refused; across projects it is allowed.
- Contacts are never hard-deleted. Removing sets `removed_at`. Removed contacts never appear in pickers; leads and cases that point to one keep it and show "(removed)".
- "First active contact" = lowest `id` among the project's contacts with `removed_at IS NULL`.
- Only admin writes projects and contacts (`access.ActionManageConfig`). All staff set a lead's or case's contact (`access.ActionEditRecord`). A dropped lead or case cannot change contact (`leads.ErrLeadClosed` for dropped or converted leads; `cases.ErrCaseDropped`).
- Error messages (web shows them through `friendlyMessage`):
  - `workflows: choose a project type` · `workflows: enter the project address` · `workflows: add at least one contact` · `workflows: contact name is required` · `workflows: this number is already a contact on this project` · `workflows: contact not found on this project` · `workflows: the NA project cannot be changed` (rename `ErrProjectLocked`'s text to this).
  - `cases: choose a builder contact` / `leads: choose a builder contact` (required when the project has active contacts and none is given) · `cases: pick a current contact of this project` / `leads: pick a current contact of this project` (wrong project, removed, or any contact on NA).
- Audit actions: `project.created`, `project.updated` (entity `project`); `case.builder_contact_changed`, `lead.builder_contact_changed` with details `{"from": id|null, "to": id|null}`. `case.project_changed` details gain `contact_from`, `contact_to`.
- Not in reports or CSV.
- Errors follow `errors.New("<pkg>: <message>")`, are mapped in `modules/http/errors.go`.
- Run `sqlc generate` after every change to a migration or `queries/*.sql`. Generated code is committed.
- **Commits:** only when the owner has asked, on `docs/project-foundation`. Otherwise skip the commit steps.

**Commands** (from repo root):
- Backend: `docker compose run --rm --build test go test -count=1 ./modules/<pkg>/ -run <Name> -v`. Full suite: `docker compose run --rm --build test`. `TestAcceptance19` fails already (infra/backup not in the container); ignore it. Always pass `--build`.
- sqlc: `cd backend && PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH" sqlc generate`.
- Web: `cd web && npm test -- <file>`, then `npm run build`. Phone check: `npm run test:e2e` inside `mcr.microsoft.com/playwright:v1.48.2-jammy`, then chown `dist`/`test-results` back to the user.

## Review Focus

1. **Admin removes a contact and adds a new one with the same phone in one save** (person re-entered with a new label): allowed, because the uniqueness rule only counts active contacts and removals are applied before inserts. Test in Task 2.
2. **Add case on a lead whose contact was removed after the lead was made**: the form prefills the project's first active contact, and the server refuses the removed id if it is sent anyway. Tests in Tasks 4 and 7.
3. **Case moved to NA or to an old project with no contacts**: contact becomes NULL, no error, and the case page shows no contact block. Test in Task 4.
4. **Two admins save the same project at once, each removing a different one of its two contacts**: the project row lock makes the second save see the first's result and be refused with "add at least one contact" (or keep its own list if it still has an active one). Test in Task 2.
5. **A deleted case or lead restored after its contact was removed**: restore succeeds and the contact shows "(removed)". Test in Task 4.

---

### Task 1: Migration 000006 (project details, contacts, contact links)

**Files:**
- Create: `backend/migrations/000006_project_details.up.sql`, `backend/migrations/000006_project_details.down.sql`
- Modify: `docs/database-schema.md` (projects, project_contacts, leads, cases sections)
- Test: `backend/migrations/migrate_test.go`

**Interfaces:**
- Produces (SQL):
  - `projects.type text NULL CHECK (type IN ('high_rise','apartments','plots','villas'))`, `projects.address text NOT NULL DEFAULT ''`, `CHECK (NOT is_system OR (type IS NULL AND address = ''))`.
  - `project_contacts(id bigint identity PK, project_id bigint NOT NULL REFERENCES projects(id), name text NOT NULL CHECK (btrim(name) <> ''), phone text NOT NULL CHECK (phone ~ '^[0-9]{10}$'), email text, label text CHECK (char_length(label) <= 40), removed_at timestamptz, created_at, created_by → users, updated_at, updated_by → users)`.
  - `UNIQUE (project_id, id)` named `project_contacts_project_id_uidx`; partial unique index `project_contacts_active_phone_uidx ON project_contacts (project_id, phone) WHERE removed_at IS NULL`.
  - `leads.builder_contact_id bigint`, `cases.builder_contact_id bigint`, each with `FOREIGN KEY (project_id, builder_contact_id) REFERENCES project_contacts (project_id, id)` (default MATCH SIMPLE: NULL contact skips the check). Constraint names `leads_builder_contact_fk`, `cases_builder_contact_fk`.
  - No backfill.
- Down: drop the two FKs and columns, drop `project_contacts`, drop the projects CHECK and columns.

- [ ] **Step 1: Write failing tests** in `migrate_test.go`:
  - `TestMigration6ContactFromOtherProjectRefused`: two projects A, B; contact on B; inserting a case with `project_id = A, builder_contact_id = <B's contact>` fails with SQLSTATE `23503`.
  - `TestMigration6ActivePhoneUnique`: same phone twice active on A → `23505`; after setting `removed_at` on the first, the second insert succeeds; same phone on B succeeds.
  - `TestMigration6NAHasNoType`: `UPDATE projects SET type='villas' WHERE is_system` fails with `23514`.
  - Extend the existing down-then-up test to run through 000006.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./migrations/ -run 'Migration6|DownThenUp' -v` → FAIL.
- [ ] **Step 3: Write** both migration files as above; update `docs/database-schema.md`.
- [ ] **Step 4: Run** Step 2 → PASS. Run `sqlc generate` and the full backend suite → PASS (fix compile errors from new generated fields with no behaviour change).
- [ ] **Step 5: Commit** `git add backend/migrations docs/database-schema.md backend/modules/*/*db && git commit -m "Add project details and builder contacts schema"`

### Task 2: Projects with details and contacts (workflows)

**Files:**
- Modify: `backend/modules/workflows/projects.go`, `backend/modules/workflows/queries/workflows.sql`
- Test: `backend/modules/workflows/projects_test.go`

**Interfaces:**
- Produces (Go, package `workflows`):
  ```go
  var ProjectTypes = []string{"high_rise", "apartments", "plots", "villas"}
  type Project struct {
      ID int64; Name string; IsSystem bool; OpenCases, TotalCases int64
      Type *string; Address string; ActiveContacts int64
      Contacts []ProjectContact // filled by GetProject, CreateProject, SaveProject only
  }
  type ProjectContact struct { ID int64; Name, Phone, Email, Label string; RemovedAt *time.Time }
  type ProjectInput struct { Name, Type, Address string; Contacts []ContactInput }
  type ContactInput struct { ID int64 /* 0 = new */; Name, Phone, Email, Label string }
  func (s *Service) CreateProject(ctx, actor access.User, in ProjectInput) (Project, error)
  func (s *Service) SaveProject(ctx, actor access.User, id int64, in ProjectInput) (Project, error) // replaces RenameProject
  func (s *Service) GetProject(ctx, id int64) (Project, error) // Contacts = active only, by id
  ```
  Errors: `ErrProjectTypeRequired`, `ErrProjectAddressRequired`, `ErrProjectContactRequired`, `ErrContactNameRequired`, `ErrContactPhoneTaken`, `ErrContactNotFound`, plus existing ones and `customers.ErrInvalidMobile`.
- `ListProjects` rows gain `Type`, `Address`, `ActiveContacts`.
- SaveProject algorithm (one transaction): `SELECT ... FROM projects WHERE id=$1 FOR UPDATE` (not found → `ErrProjectNotFound`, system → `ErrProjectLocked`); validate all input (normalise each phone; duplicate phone within the input → `ErrContactPhoneTaken`; an input `ID` not an active contact of this project → `ErrContactNotFound`); update name/type/address; mark removed every active contact whose id is absent from the input; update listed ones; insert new ones; then count active contacts — 0 → `ErrProjectContactRequired` (rolls back). Map 23505 on `project_contacts_active_phone_uidx` → `ErrContactPhoneTaken`, on `projects_name_uidx` → `ErrProjectNameTaken`. Audit `project.updated`.
- CreateProject: same validation, insert project + contacts in one transaction, audit `project.created`.

- [ ] **Step 1: Write failing tests** (replace the old rename tests):
  - `TestCreateProjectNeedsDetails`: missing type → `ErrProjectTypeRequired`; type `"flats"` → `ErrProjectTypeRequired`; blank address → `ErrProjectAddressRequired`; no contacts → `ErrProjectContactRequired`; contact phone `"123"` → `customers.ErrInvalidMobile`; blank contact name → `ErrContactNameRequired`; two contacts same phone → `ErrContactPhoneTaken`.
  - `TestCreateProjectSavesContacts`: valid input with 2 contacts → `GetProject` returns type, address and both contacts in id order, phone normalised (`"+91 98765-43210"` → `"9876543210"`).
  - `TestSaveProjectOldIncompleteMustComplete`: project inserted with name only (old style); `SaveProject` with just a new name → `ErrProjectTypeRequired`; with type+address+1 contact → OK.
  - `TestSaveProjectRemovesMissingContacts`: save with contact 1 omitted → contact 1 has `removed_at` set, not in `GetProject`, still in table; `ActiveContacts` in `ListProjects` = 1.
  - `TestSaveProjectLastContactRefused`: saving with an empty list → `ErrProjectContactRequired` and nothing changed (name unchanged too).
  - `TestSaveProjectRemoveAndReaddSamePhone` (Review Focus 1): omit contact 1 and add a new one with contact 1's phone → OK.
  - `TestSaveProjectForeignContactID`: input id of another project's contact → `ErrContactNotFound`.
  - `TestSaveProjectSamePhoneOtherProject`: allowed.
  - `TestSaveProjectConcurrentRemovals` (Review Focus 4): project with contacts X, Y; two goroutines save at once, one keeps only X, the other keeps only Y; afterwards ≥ 1 active contact and the loser got `ErrProjectContactRequired` or its list was applied after the winner's (assert active count ≥ 1).
  - `TestProjectNALocked`: `SaveProject` on NA → `ErrProjectLocked`; `TestProjectWritesAdminOnly`: manager and employee → `access.ErrForbidden`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/workflows/ -run 'Project' -v` → FAIL.
- [ ] **Step 3: Implement** the queries and service as above; run `sqlc generate`.
- [ ] **Step 4: Run** Step 2 → PASS.
- [ ] **Step 5: Commit** `git commit -am "Projects keep type, address and their own contacts"`

### Task 3: Builder contact on leads

**Files:**
- Modify: `backend/modules/leads/leads.go`, `create.go`, `list.go`, `queries/leads.sql`
- Create: `backend/modules/leads/builder_contact.go`
- Test: `backend/modules/leads/builder_contact_test.go`

**Interfaces:**
- Consumes: `project_contacts` (Task 1).
- Produces:
  ```go
  type BuilderContact struct { ID int64; Name, Phone, Email, Label string; Removed bool }
  // Lead gains: BuilderContactID *int64. Row gains: BuilderContact *BuilderContact (LEFT JOIN, includes removed).
  func (s *Service) SetBuilderContact(ctx, actor access.User, leadID, contactID int64) (Lead, error)
  var ErrBuilderContactRequired = errors.New("leads: choose a builder contact")
  var ErrBuilderContactInvalid  = errors.New("leads: pick a current contact of this project")
  ```
- Query `FirstActiveContact(project_id) → id` (`ORDER BY id LIMIT 1`, no rows = none), `ActiveContactInProject(project_id, id) → bool`.
- `insertLead` sets `builder_contact_id` to the first active contact (NULL when none). Used by `Create` and `AddForCustomer`.
- `SetBuilderContact`: locks the lead; not open → `ErrLeadClosed`; `contactID` must be active and of the lead's project → else `ErrBuilderContactInvalid`; same id → no change; audit `lead.builder_contact_changed`.

- [ ] **Step 1: Write failing tests:** `TestNewLeadGetsFirstActiveContact` (contacts ids 5,7, 5 removed → lead gets 7); `TestNewLeadNAHasNoContact`; `TestNewLeadOldProjectNoContact`; `TestSetBuilderContact` (changes, audit row with from/to); `TestSetBuilderContactRefusesRemovedOrForeign` → `ErrBuilderContactInvalid`; `TestSetBuilderContactClosedLead` (dropped → `ErrLeadClosed`); `TestLeadRowShowsRemovedContact` (`Row.BuilderContact.Removed == true`).
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/leads/ -run 'BuilderContact|FirstActive|NoContact|RemovedContact' -v` → FAIL.
- [ ] **Step 3: Implement**; `sqlc generate`.
- [ ] **Step 4: Run** Step 2 and the whole leads package → PASS.
- [ ] **Step 5: Commit** `git commit -am "Leads point to a builder contact"`

### Task 4: Builder contact on cases (create, from lead, change, project change, archive)

**Files:**
- Modify: `backend/modules/cases/create.go`, `fromlead.go`, `project.go`, `assign.go` (load), `list.go` (rows), `queries/cases.sql`
- Create: `backend/modules/cases/builder_contact.go`
- Test: `backend/modules/cases/builder_contact_test.go`, `backend/modules/archive/leads_test.go` (one added test)

**Interfaces:**
- Produces:
  ```go
  // NewCase and FromLead gain: BuilderContactID int64 (0 = none given).
  // Case gains: BuilderContactID *int64, BuilderContact *BuilderContact (LEFT JOIN in GetCase, includes removed).
  type BuilderContact struct { ID int64; Name, Phone, Email, Label string; Removed bool }
  func (s *Service) SetBuilderContact(ctx, actor access.User, caseID, contactID int64) (Case, error)
  var ErrBuilderContactRequired = errors.New("cases: choose a builder contact")
  var ErrBuilderContactInvalid  = errors.New("cases: pick a current contact of this project")
  ```
- Rule in `prepare` (shared by Create and CreateFromLead): if the project has an active contact, `BuilderContactID` must be one of its active contacts (0 → `ErrBuilderContactRequired`, other → `ErrBuilderContactInvalid`); if it has none (NA or old), it must be 0 (else `ErrBuilderContactInvalid`). `insertTx` stores it (NULL for 0).
- `ChangeProject`: when the project actually changes, set `builder_contact_id` to the new project's first active contact (NULL if none) in the same update; audit details add `contact_from`, `contact_to`.
- `SetBuilderContact`: same shape as the leads one; dropped → `ErrCaseDropped`; audit `case.builder_contact_changed`.
- Archive needs no code change (`jsonb_populate_record` carries the column); the test proves it.

- [ ] **Step 1: Write failing tests:** `TestCreateCaseNeedsContactWhenProjectHasOne`; `TestCreateCaseNAContactRefused` (NA + contact id → invalid; NA + 0 → OK); `TestCreateCaseRemovedContactRefused` (Review Focus 2); `TestFromLeadStoresContact`; `TestChangeProjectResetsContact` (to project with contacts → its first active; to NA → NULL; to old project → NULL — Review Focus 3; audit details carry `contact_from`/`contact_to`); `TestSetCaseBuilderContact` + dropped → `ErrCaseDropped`; `TestGetCaseShowsRemovedContact`. In `archive/leads_test.go`: `TestRestoreCaseKeepsRemovedContact` (Review Focus 5): case with contact, delete case, remove contact, restore → `builder_contact_id` kept, `Removed == true`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/cases/ ./modules/archive/ -run 'Contact' -v` → FAIL.
- [ ] **Step 3: Implement**; `sqlc generate`. Update existing case-creation tests and fixtures that use a project with contacts to pass a contact id.
- [ ] **Step 4: Run** Step 2, then the full backend suite → PASS.
- [ ] **Step 5: Commit** `git commit -am "Cases point to a builder contact"`

### Task 5: HTTP routes and JSON

**Files:**
- Modify: `backend/modules/http/router.go`, `handlers.go`, `leads_handlers.go`, `errors.go`
- Test: `backend/modules/http/projects_routes_test.go`, `leads_routes_test.go`, `routes_test.go`

**Interfaces:**
- Routes:
  - `GET /api/config/projects` → each item adds `type` (string|null), `address`, `active_contacts`.
  - `GET /api/config/projects/{id}` (all staff) → project JSON + `contacts: [{id,name,phone,email,label}]` (active only). 404 when missing.
  - `POST /api/admin/projects` body `{name,type,address,contacts:[{name,phone,email,label}]}` → 201 with contacts.
  - `PUT /api/admin/projects/{id}` body `{name,type,address,contacts:[{id?,name,phone,email,label}]}` → 200 (calls `SaveProject`; replaces the rename handler).
  - `POST /api/leads/{id}/builder-contact` and `POST /api/cases/{id}/builder-contact` body `{contact_id}` → the lead row / case JSON.
  - `POST /api/cases` and `POST /api/leads/{id}/case` accept `builder_contact_id` (number, optional).
- JSON: case JSON, case rows and lead rows add `builder_contact_id` (number|null) and `builder_contact` (`{id,name,phone,email,label,removed}` | null).
- errors.go: all new errors → 400, except `ErrContactPhoneTaken` → 409, `ErrContactNotFound` → 400.

- [ ] **Step 1: Write failing tests:** employee and manager get 403 on POST/PUT `/api/admin/projects`; admin create round-trip returns `contacts` with normalised phones; PUT with an empty contact list → 400 "add at least one contact"; GET `/api/config/projects/{id}` as employee → 200 with active contacts only; employee `POST /api/cases/{id}/builder-contact` with a foreign id → 400; lead row JSON has `builder_contact.removed`.
- [ ] **Step 2: Run** `docker compose run --rm --build test go test -count=1 ./modules/http/ -v` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** Step 2 and the full backend suite → PASS.
- [ ] **Step 5: Commit** `git commit -am "API for project details and builder contacts"`

### Task 6: Web: Admin project form

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/pages/admin/BanksAndLists.tsx`
- Create: `web/src/api/projectTypes.ts`, `web/src/pages/admin/ProjectForm.tsx`
- Test: `web/src/pages/admin/BanksAndLists.test.tsx`, `web/src/pages/admin/ProjectForm.test.tsx`

**Interfaces:**
- Produces:
  ```ts
  export type ProjectType = "high_rise" | "apartments" | "plots" | "villas";
  export const PROJECT_TYPES: { value: ProjectType; label: string }[]; // order and labels from Global Constraints
  export type BuilderContact = { id: number; name: string; phone: string; email: string; label: string; removed?: boolean };
  export type Project = { id; name; is_system; open_cases; total_cases; type: ProjectType | null; address: string; active_contacts: number; contacts?: BuilderContact[] };
  export function ProjectForm(props: { project?: Project; onClose: () => void; onSaved: () => void }): JSX.Element
  ```
- ProjectForm: Name, Type (radio group in the order above), Address, a contact list (each row: label · name · phone, Edit, Remove) and "+ Add contact" (inline fields: Name, Mobile, Email optional, Label optional). Nothing is sent until **Save**; one POST (new) or PUT (edit) with the whole list. **Cancel** discards. Save disabled while sending. When editing, it loads `GET /api/config/projects/{id}`.
- BanksAndLists: "+ Add project" and per-project **Edit** open ProjectForm (replaces OneFieldDialog for projects); list shows type label and "N contacts"; a project with `type === null` or `active_contacts === 0` (not NA) shows "Add type and contact".

- [ ] **Step 1: Write failing tests:** form refuses Save locally with no contact ("Add at least one contact"); removing a contact then Cancel sends nothing; Save sends one PUT with the remaining contacts and the new one (no `id` on new); server error text is shown; list shows "Add type and contact" for an old project and "2 contacts" for a complete one.
- [ ] **Step 2: Run** `cd web && npm test -- ProjectForm BanksAndLists` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** Step 2 → PASS; `npm run build` → OK.
- [ ] **Step 5: Commit** `git commit -am "Admin edits project type, address and contacts"`

### Task 7: Web: contact on project page, lead page, case page and Add case

**Files:**
- Create: `web/src/components/BuilderContactCard.tsx`, `web/src/components/BuilderContactPicker.tsx`
- Modify: `web/src/api/types.ts` (Case, CaseRow, LeadRow gain `builder_contact_id`, `builder_contact`), `web/src/pages/ProjectDetail.tsx`, `web/src/pages/LeadDetail.tsx`, `web/src/pages/CaseDetail.tsx`, `web/src/components/AddCaseDialog.tsx`
- Test: `web/src/pages/LeadDetail.test.tsx`, `CaseDetail.test.tsx`, `web/src/components/AddCaseDialog.test.tsx`

**Interfaces:**
- ```ts
  export function BuilderContactCard(p: { contact: BuilderContact | null; onChange?: () => void }): JSX.Element | null
  // label · name, phone as <a href="tel:+91{phone}">, email as mailto when set, "(removed)" when removed; "change" button when onChange given; renders null when contact is null and onChange is absent.
  export function BuilderContactPicker(p: { open: boolean; projectId: number; current: number | null; onClose: () => void; onPick: (contactId: number) => Promise<void> }): JSX.Element
  // loads GET /api/config/projects/{projectId}; radio list of active contacts.
  ```
- ProjectDetail: under the title, type label, address, and each active contact (tap-to-call).
- LeadDetail: "Builder contact" row in the details list (hidden for NA); change → picker → `POST /api/leads/{id}/builder-contact`; no change button when the lead is not open.
- CaseDetail: under Project in Details, same pattern with `POST /api/cases/{id}/builder-contact`; no change button when dropped.
- AddCaseDialog: `fromLead` gains `projectId` and `builderContactId`. A "Builder contact" select listing the project's active contacts, prefilled with the lead's contact if it is still active, else the first one; reloads when the project changes (no-lead mode); hidden when the project has no active contacts; sends `builder_contact_id`.

- [ ] **Step 1: Write failing tests:** lead page shows contact name and a `tel:+919876543210` link; removed contact shows "(removed)"; NA lead shows no contact row; change on case page posts the chosen id and refreshes; Add case from a lead whose contact is removed prefills the first active contact (Review Focus 2) and sends its id; Add case on NA has no contact select and sends no id; dropped case shows no change button.
- [ ] **Step 2: Run** `cd web && npm test -- LeadDetail CaseDetail AddCaseDialog` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** the whole web suite → PASS; `npm run build` → OK.
- [ ] **Step 5: Commit** `git commit -am "Show and change the builder contact on leads and cases"`

### Task 8: Full check and phone run

**Files:**
- Modify: `web/e2e/phone.spec.ts`

- [ ] **Step 1: Extend** the phone test: admin adds a project with type Villas, address and one contact; staff add a lead on it and see the contact with a call link at 390 px width without sideways scrolling.
- [ ] **Step 2: Run** the full backend suite, web suite, `npm run build`, and the phone test in the Playwright image → all PASS (except the known `TestAcceptance19`).
- [ ] **Step 3: Apply** migration 6 to the local compose app (`docker compose up --build -d db backend web`) and open Admin › Banks & lists to confirm old projects show "Add type and contact".
- [ ] **Step 4: Commit** `git commit -am "Phone check for project contacts"`
