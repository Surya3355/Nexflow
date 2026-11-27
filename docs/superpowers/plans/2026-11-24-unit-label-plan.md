# Unit Label by Project Type: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The unit field is labelled by project type and is required on builder projects at Add case.

**Architecture:** `cases.prepare` refuses an empty unit when the project has a type; `SetUnitNumber` refuses clearing on a typed project. `cases.Case` gains `ProjectType` (filled by `load`), sent as `project_type`. The web gets one `unitLabel(type)` helper used by Add case, the case page row and the change box.

**Tech Stack:** Go 1.25, pgx/sqlc, PostgreSQL 16; React 18 + TypeScript, Vitest.

**Spec:** `docs/superpowers/specs/2026-11-24-unit-label-design.md`

## Global Constraints
- Labels: `high_rise`, `apartments` → "Flat no."; `villas` → "Villa no."; `plots` → "Plot no."; null → "Unit no.". Placeholder "e.g. A-1012".
- Server error: `ErrUnitRequired = errors.New("cases: enter the unit number for this project")` → HTTP 400.
- Web local message: `Enter the ${label.toLowerCase()}` + "." (e.g. "Enter the flat no.").
- Unit pattern unchanged. Commits only when the owner asks (owner asked to build; commit at the end when asked).
- Commands as in `2026-11-24-project-details-plan.md` (docker compose run --rm --build test …; npm test; npm run build).

## Review Focus
1. Lead on an old untyped project that admin later completes (gets a type) → Add case now requires the unit. Test in Task 1.
2. Case moved from NA to a typed project with no unit → move allowed, unit stays empty, case page shows the right label. Tests in Tasks 1 and 2.
3. Whitespace-only unit on a typed project counts as empty. Test in Task 1.

---

### Task 1: Server rule
**Files:** Modify `backend/modules/cases/create.go` (prepare), `unit.go`, `assign.go` (load), `queries/cases.sql` (`ProjectType :one` → `SELECT type FROM projects WHERE id = $1`), `backend/modules/http/handlers.go` (caseJSON `project_type`), `errors.go`. Test: `backend/modules/cases/unit_test.go`, `backend/modules/http/routes_test.go` or `projects_routes_test.go`.
**Produces:** `cases.ErrUnitRequired`; `Case.ProjectType *string`; JSON `project_type` (string|null).
- [ ] Tests: `TestUnitRequiredOnTypedProject` (typed + "" and "  " → ErrUnitRequired; "a-1012" → "A-1012"); `TestUnitOptionalOnNAAndUntyped`; `TestFromLeadUnitRequired` (lead on untyped project, then `UPDATE projects SET type='villas'`, CreateFromLead with "" → ErrUnitRequired); `TestSetUnitCannotClearOnTyped` (clear → ErrUnitRequired, change OK, clear on NA OK); `TestChangeProjectKeepsEmptyUnit` (NA case with no unit → typed project OK, unit nil); `TestGetCaseHasProjectType`. HTTP: POST /api/cases on typed project with empty unit → 400 "enter the unit number".
- [ ] Run cases + http → FAIL; implement; sqlc generate; run → PASS; full backend suite.

### Task 2: Web labels and checks
**Files:** Modify `web/src/api/projectTypes.ts` (`unitLabel(t)`), `web/src/api/types.ts` (Case `project_type?`), `web/src/components/AddCaseDialog.tsx`, `web/src/pages/CaseDetail.tsx` (row + UnitDialog). Test: `AddCaseDialog.test.tsx`, `CaseDetail.test.tsx`.
- [ ] Tests: Add case from a villas lead shows "Villa no." with placeholder "e.g. A-1012", empty → "Enter the villa no." and no request; high_rise shows "Flat no."; NA shows "Unit no." and sends with empty unit; case page row label "Plot no." for a plots case, "Unit no." when `project_type` null; change box titled "Change plot no.".
- [ ] Run → FAIL; implement; run web suite → PASS; build; phone e2e.
