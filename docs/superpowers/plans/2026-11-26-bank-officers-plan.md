# Bank officers and officer per stage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each bank keeps a list of officers per team; staff pick the officer when a case reaches a stage that has a team, and the case page shows who handles it.

**Architecture:** Stages get an optional `officer_team` (like Task 10's `role`). A new `officers` module manages `bank_officers`. Case picks live in an append-only `case_officers` table (latest row per case and team is current; all rows are the history), written by the cases module in the same transaction as moves and case creation.

**Tech Stack:** Go 1.25, pgx, sqlc, golang-migrate, PostgreSQL 16; React 18 + TypeScript + Tailwind, Vitest; Playwright phone test.

**Spec:** `docs/superpowers/specs/2026-11-26-bank-officers-design.md`

## Global Constraints

- Team codes and names: `tdep` TDEP, `dovr` DOVR, `appraisal` Appraisal, `fixing` Fixing, `officer` Officer.
- Team per stage: HDFC `link_sent`→tdep, `dovr`→dovr, `appraisal`→appraisal, `under_fixing`→fixing; standard `received`→officer. Each team at most once per stage list.
- Officer phone: 10 digits (`^[0-9]{10}$`), same check as builder contacts; name trimmed, not blank, ≤ 80 characters.
- Any signed-in role can add, edit, remove officers (`access.ActionAddRecord`). Picking follows `ActionEditRecord`. Dropped cases cannot change officers.
- Picking is optional everywhere; an empty pick never clears an existing officer.
- No officer columns in CSV exports. No Needs-attention rule.
- Tests: backend `docker compose run --rm --build test go test -count=1 ./...` (from repo root); web `cd web && npx vitest run` and `npm run build`; sqlc `cd backend && ~/go/bin/sqlc generate`.
- Owner commits only on request: no per-task commits.

## Review Focus

1. An officer id from another bank, another team, or a removed officer sent to any pick endpoint → refused with a clear message (tests in Task 3).
2. Re-adding an officer with the mobile of a removed officer in the same bank+team → allowed (Task 2 test).
3. A team the case's stage list does not have (e.g. `officer` on an HDFC case) → refused (Task 3 test).
4. Archive then restore of a case with officer picks → picks come back; old snapshots without them restore (Task 4 test).
5. Double tap on "+ New officer" save → one officer (button disabled while busy; Task 6 test).

---

### Task 1: Migration 8 and stage teams

**Files:**
- Create: `backend/migrations/000008_bank_officers.up.sql`, `.down.sql`
- Modify: `backend/migrations/migrate_test.go`, `backend/modules/workflows/{workflows.go,stagelists.go,queries/workflows.sql}`, regenerate sqlc
- Test: `backend/modules/workflows/stagelists_test.go`

**Interfaces:**
- Produces: `workflows.Stage.OfficerTeam string`; constants `TeamTDEP, TeamDOVR, TeamAppraisal, TeamFixing, TeamOfficer`; `workflows.TeamName(code string) string`; tables `bank_officers`, `case_officers`.

- [ ] **Step 1: Write failing tests.** `TestMigration8Officers`: after up, standard `received` has `officer_team='officer'`; a second stage of the same list with `officer` violates `stages_officer_team_uidx`; inserting `bank_officers` with phone `12345` fails the check; down then up works. `TestHDFCStageListHasTeams`: `SetBankStageList` → `StagesFor` gives link_sent=tdep, dovr=dovr, appraisal=appraisal, under_fixing=fixing, others "". `TestStandardStagesHaveOfficerTeam`.
- [ ] **Step 2: Run; expect FAIL** (no column / field).
- [ ] **Step 3: Migration.**
  - `stages.officer_team text CHECK (officer_team IN ('tdep','dovr','appraisal','fixing','officer'))`, `CREATE UNIQUE INDEX stages_officer_team_uidx ON stages (bank_id, officer_team) NULLS NOT DISTINCT WHERE officer_team IS NOT NULL`.
  - Set `officer` on standard `received`; set HDFC teams by code on bank-owned stages (`bank_id IS NOT NULL`).
  - `bank_officers(id identity pk, bank_id bigint NOT NULL REFERENCES banks, team text NOT NULL same check, name text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 80), phone text NOT NULL CHECK (phone ~ '^[0-9]{10}$'), removed_at timestamptz, created_at/by, updated_at/by)`; `UNIQUE INDEX bank_officers_active_phone_uidx ON (bank_id, team, phone) WHERE removed_at IS NULL`.
  - `case_officers(id identity pk, case_id bigint NOT NULL REFERENCES cases, team text NOT NULL same check, officer_id bigint NOT NULL REFERENCES bank_officers, picked_at timestamptz NOT NULL DEFAULT now(), picked_by bigint NOT NULL REFERENCES users)`; index `(case_id, team, picked_at DESC, id DESC)`.
  - Grants follow migration 000002's pattern if it grants per table (check and copy).
  - Down drops both tables, the index and the column.
- [ ] **Step 4: Code.** `hdfcTemplate` entries gain a team; `InsertBankStage` takes `officer_team`; stage queries return `COALESCE(officer_team,'')`. `TeamName` maps codes to the names above.
- [ ] **Step 5: Run tests; expect PASS.** Then the whole backend suite.

### Task 2: Officers module

**Files:**
- Create: `backend/modules/officers/{officers.go,queries/officers.sql,officersdb/*}`, add to `backend/sqlc.yaml`
- Test: `backend/modules/officers/officers_test.go`

**Interfaces:**
- Consumes: `workflows.Service.StagesFor`, `workflows.TeamName`.
- Produces:
  - `type Officer struct{ ID, BankID int64; BankName, Team, TeamName, Name, Phone string; Removed bool }`
  - `type NewOfficer struct{ BankID int64; Team, Name, Phone string }`
  - `NewService(pool, wf *workflows.Service, rec history.Recorder) *Service`
  - `List(ctx) ([]Officer, error)` (current only, by bank name, team stage order, name)
  - `Add(ctx, actor, NewOfficer) (Officer, error)`, `Update(ctx, actor, id int64, name, phone string) (Officer, error)`, `Remove(ctx, actor, id int64) error`
  - Errors: `ErrNameRequired`, `ErrPhoneInvalid`, `ErrPhoneTaken` ("this bank already has this mobile in that team"), `ErrTeamNotForBank`, `ErrOfficerNotFound`.

- [ ] **Step 1: Failing tests.** `TestAddOfficer` (HDFC bank, team dovr, phone "98765 43210" stored as "9876543210"); `TestAddOfficerRefusesTeamNotOnBank` (officer team on HDFC bank → ErrTeamNotForBank); `TestAddOfficerPhoneTaken`; `TestReaddAfterRemoveAllowed`; `TestSamePhoneTwoTeamsAllowed`; `TestEmployeeCanManageOfficers`; `TestRemoveHidesFromList`; `TestOfficerChangesAudited` (actions `officer.added|updated|removed`).
- [ ] **Step 2: Run; expect FAIL.**
- [ ] **Step 3: Implement.** Phone: strip spaces/dashes, then 10 digits (reuse `customers.NormalizeMobile` if it gives the same rule, else local). Team must be the `OfficerTeam` of one of `StagesFor(bank)`. Map 23505 on the phone index to `ErrPhoneTaken`. Removing twice → `ErrOfficerNotFound`.
- [ ] **Step 4: Run; expect PASS.**

### Task 3: Officer picks on cases

**Files:**
- Create: `backend/modules/cases/officers.go`
- Modify: `cases/{stage.go,create.go,fromlead.go,queries/cases.sql}`, regenerate sqlc
- Test: `backend/modules/cases/officers_test.go`

**Interfaces:**
- Produces:
  - `StageMove.OfficerID int64` (0 = keep)
  - `NewCase.Officers map[string]int64`, `FromLead.Officers map[string]int64` (team → officer id)
  - `type CaseOfficer struct{ Team, TeamName string; Officer *Officer }` with `cases.Officer{ID int64; Name, Phone string; Removed bool}`
  - `type OfficerEvent struct{ At time.Time; Team, TeamName, OfficerName, ChangedBy string }`
  - `(s *Service) Officers(ctx, caseID) ([]CaseOfficer, error)` — teams of the case's stage list whose stage `SortOrder <= current stage SortOrder`, in stage order
  - `(s *Service) OfficerHistory(ctx, caseID) ([]OfficerEvent, error)` — oldest first
  - `(s *Service) SetOfficer(ctx, actor, caseID int64, team string, officerID int64) (Case, error)`
  - Errors: `ErrOfficerInvalid` ("pick a current officer of this bank and team"), `ErrTeamNotOnStages` ("this case's bank has no such team").

- [ ] **Step 1: Failing tests.**
  - `TestMoveToDOVRPicksOfficer`: move with OfficerID → `Officers` shows DOVR officer; history has one event.
  - `TestMoveWithoutOfficerKeepsCurrent`: back to DOVR with OfficerID 0 keeps it.
  - `TestMoveSameOfficerNoNewEvent`.
  - `TestPickRefusesOtherBankOfficer`, `TestPickRefusesOtherTeamOfficer`, `TestPickRefusesRemovedOfficer` → ErrOfficerInvalid.
  - `TestMoveWithOfficerToStageWithoutTeam` → ErrOfficerInvalid.
  - `TestSetOfficerTeamNotOnList` (`officer` on HDFC case) → ErrTeamNotOnStages; `TestSetOfficerDroppedCase` → ErrCaseDropped.
  - `TestCreateHDFCCaseWithTDEP`; `TestCreateStandardAtLoggedInWithOfficer` (team on Received, start at Logged in → accepted); `TestCreateRefusesTeamNotReached` (dovr at creation → ErrTeamNotOnStages); `TestCreateFromLeadWithOfficer`.
  - `TestOfficersListsReachedTeamsOnly`: HDFC case at DOVR → [TDEP, DOVR], DOVR unset shows Officer nil.
  - `TestRemovedOfficerStillShownOnCase` (Removed true).
  - `TestSetOfficerAudited` (`case.officer_changed` with from/to).
- [ ] **Step 2: Run; expect FAIL.**
- [ ] **Step 3: Implement.** One helper `pickOfficerTx(ctx, q, actor, caseID, bankID, team string, officerID int64) error`: query `ActiveBankOfficer(id)` must match bank and team; skip if the latest pick for (case, team) is the same officer; else insert `case_officers` and audit. `ChangeStage` calls it after the stage update when `m.OfficerID != 0` (stage without team → ErrOfficerInvalid). Create/CreateFromLead validate each team against the `OfficerTeam` of stages in `p.stages` (else ErrTeamNotOnStages) inside `prepare`, then call the helper in `insertTx`. `SetOfficer` locks the case like `SetBuilderContact`.
- [ ] **Step 4: Run; expect PASS**, then the cases package and the whole suite.

### Task 4: Archive carries officer picks

**Files:** Modify `backend/modules/archive/{queries/archive.sql,delete.go,restore.go}`; Test `backend/modules/archive/officers_test.go`

- [ ] **Step 1: Failing test** `TestArchiveRestoreKeepsOfficers` (pick, archive, restore → `Officers` shows it) and `TestRestoreOldSnapshotWithoutOfficers`.
- [ ] **Step 2: Run; expect FAIL** (foreign-key error on case delete).
- [ ] **Step 3:** add `'officers'` to `CaseChildrenJSON`, `DeleteCaseOfficers` before the case delete, restore inserts with `OVERRIDING SYSTEM VALUE ... jsonb_populate_recordset(NULL::case_officers, …)`, default `[]`.
- [ ] **Step 4: Run; expect PASS.**

### Task 5: HTTP

**Files:** Modify `backend/modules/http/{router.go,handlers.go,errors.go,deps}` (wire `officers.Service` where other services are built, `cmd`/main); create `backend/modules/http/officers_handlers.go`; Test `backend/modules/http/officers_routes_test.go`

**Interfaces (JSON):**
- `GET /api/officers` → `[{id, bank_id, bank_name, team, team_name, name, phone}]`
- `POST /api/officers` `{bank_id, team, name, phone}` → 201 officer; `PUT /api/officers/{id}` `{name, phone}` → officer; `DELETE /api/officers/{id}` → 204
- `GET /api/cases/{id}/officers` → `{officers:[{team, team_name, officer:{id,name,phone,removed}|null}], history:[{at, team, team_name, officer_name, changed_by}]}`
- `POST /api/cases/{id}/officers` `{team, officer_id}` → case JSON (same as `getCase`)
- Change-stage body adds `officer_id`; create-case and lead-to-case bodies add `officers: {team: id}`
- Stage JSON adds `officer_team`, `officer_team_name`
- Errors: officer errors and `ErrOfficerInvalid`/`ErrTeamNotOnStages` → 400; `ErrOfficerNotFound` → 404; `ErrPhoneTaken` → 409

- [ ] **Step 1: Failing route test** covering each route once as an employee, plus a 400 for another bank's officer on the stage route.
- [ ] **Step 2: Run; expect FAIL.** **Step 3: Implement.** **Step 4: Run; expect PASS**, whole suite.

### Task 6: Web

**Files:**
- Modify: `web/src/api/types.ts`, `web/src/roles.ts` (menu), `web/src/App.tsx` (route `/officers`), `web/src/pages/case/MoveStageDialog.tsx`, `web/src/components/AddCaseDialog.tsx`, `web/src/pages/CaseDetail.tsx`
- Create: `web/src/components/OfficerSelect.tsx`, `web/src/pages/case/BankOfficers.tsx`, `web/src/pages/Officers.tsx`, tests beside each

**Interfaces:**
- `OfficerSelect({bankId, bankName, team, teamName, value, onChange})`: label "{teamName} officer (optional)", a `<select>` of current officers of that bank and team ("Name · 9876543210"), option "Not known yet" (value ""), and "+ New officer" which shows Name and Mobile inputs with Save (disabled while busy) that POSTs and selects the new officer.
- `BankOfficers({caseId, bankId, bankName, readOnly, onChanged})`: heading "Bank officers"; per team row "TDEP · Name · [call link tel:]" or orange "Officer not set"; "Change" opens a dialog with `OfficerSelect` and Save → `POST /api/cases/{id}/officers`. Removed officers show "(removed)".
- Officers page: heading "Bank officers"; per bank, per team, list with Edit / Remove (confirm "Remove {name}? Cases that have this officer keep them."); "Add officer" form (bank, team from `/api/config/banks/{id}/stages` teams, name, mobile).

- [ ] **Step 1: Failing tests.**
  - `OfficerSelect.test.tsx`: lists only the given team's officers; "+ New officer" saves once on double click and selects it.
  - `MoveStageDialog`: target stage with team shows "DOVR officer (optional)" prefilled with current and sends `officer_id`; stage without team shows no officer field.
  - `AddCaseDialog`: HDFC bank shows "TDEP officer (optional)" and sends `officers: {tdep: id}`; nothing chosen sends `officers: {}`.
  - `BankOfficers.test.tsx`: shows name with `tel:` link, "Officer not set", "(removed)"; Change saves; read-only hides Change.
  - `CaseDetail`: history list includes "DOVR officer: Ravi" entries in time order.
  - `Officers.test.tsx`: employee sees the page, adds, edits, removes with confirm; menu shows "Officers" for every role.
- [ ] **Step 2: Run; expect FAIL.** **Step 3: Implement.** **Step 4: Run `npx vitest run` and `npm run build`; expect PASS.**

### Task 7: Docs, phone check, deploy

**Files:** Modify `docs/00-decisions.md` (§10m entry for tasks 6 and 7), `docs/database-schema.md` (officer_team, bank_officers, case_officers), backlog (tasks 6 and 7 built), `web/e2e/phone.spec.ts` (Officers page fits a phone screen without sideways scroll).

- [ ] **Step 1:** docs. **Step 2:** phone e2e in the Playwright container; expect all pass. **Step 3:** full backend suite, web suite, build. **Step 4:** `docker compose up --build -d db backend web`; migration 8 applied; app answers on :8081.
