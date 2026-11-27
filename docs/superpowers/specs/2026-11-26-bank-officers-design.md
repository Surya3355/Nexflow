# Bank officers and officer per stage (round 15, tasks 6 and 7)

Status: **approved by owner** (2026-11-26). Backlog: `2026-11-22-round-15-backlog.md`, Tasks 6 and 7.
Builds on Task 10 (`2026-11-25-hdfc-stages-design.md`): stage lists and stage roles.
Tasks 8 (strict stages with proof) and 9 (bank queries) are **not** part of it.

## 1. Why

Staff chase the bank to move a case. At HDFC a case passes through several teams, and
the officer handling it changes from case to case; today Nexflow records no officer, so
staff ask around or keep numbers on their phones. The owner wants a maintained list of
officers (name, mobile) per bank, picked from a dropdown, and the case to show who is
handling it at each team so staff can call directly.

## 2. Owner's answers (2026-11-26)

- Officers change **per case** (not one fixed officer per stage).
- Design tasks 6 and 7 together.
- Picking the officer is **optional**; a missing officer is shown, not blocked.
- HDFC teams: TDEP, DOVR, Appraisal, Fixing. **No officer** for Signing or Disbursement.
- **Everyone** (admin, manager, employee) can add, edit and remove officers.
- One person can be in **two teams** (added once per team).
- Approach **A** (each stage names its team).
- Officers are **not** added to the CSV files.
- An officer has a **mobile or an email** (or both): some officers do not give a mobile (owner, later on 2026-11-26).

## 3. Goals, non-goals, anti-goals

Goals
- A list of officers per bank: name, mobile and/or email, team.
- When a case reaches a stage that has a team, staff pick that team's officer from a
  dropdown (or add a new one there).
- The case page shows each team's officer with a call button, and "Officer not set" for
  a team the case has reached without an officer.
- SBI (standard stages): one "Officer" team, picked when the case is added.

Non-goals
- Officer columns in CSV exports or reports (owner).
- Strict stages with proof (Task 8), bank queries (Task 9).
- A screen to create teams or link teams to stages. Teams per stage list are set by us in
  code, as stages are (Task 10 §3).
- Officer branch or designation.
- Officers on leads (a lead has no bank stage yet).

Anti-goals
- Blocking a stage move because the officer is not known yet (owner: optional).
- An officer picked for the wrong team or another bank's officer on a case.
- Losing who handled an old case when an officer is removed from the list.

## 4. Approaches considered

- **A. Each stage names its team (chosen).** A stage carries an optional *officer team*.
  Officers belong to a bank and a team. The move dialog shows the officer dropdown for the
  target stage's team. Same pattern as Task 10's stage roles; a later private bank gets
  its teams the same way. Cost: new teams need a code change.
- B. Each officer is tagged with the stages they handle. Rejected: staff tick stages per
  officer, and every officer needs editing if stages change.
- C. Teams created by admin and linked to stages on a new screen. Rejected: a setup
  screen for something that rarely changes.

## 5. Design

### 5.1 Teams per stage list

| Stage list | Stage | Team |
|---|---|---|
| HDFC | Link sent (start) | TDEP |
| HDFC | DOVR | DOVR |
| HDFC | Appraisal | Appraisal |
| HDFC | Under fixing | Fixing |
| Standard | Received (start) | Officer |

All other stages have no team. Team codes: `tdep`, `dovr`, `appraisal`, `fixing`,
`officer`; display names as in the table. A stage list has each team at most once.

### 5.2 Data

- `stages.officer_team text NULL` with a check on the five codes; unique per stage list
  (`(bank_id, officer_team)`, nulls not distinct, where not null). Set on the standard
  stages and the HDFC template; the migration also sets it on banks already switched to
  HDFC stages, by stage code.
- New `bank_officers`: `id`, `bank_id`, `team`, `name` (not blank), `phone` (10 digits, optional) and `email` (optional; one of the two required,
  phone same rule as builder contacts), `removed_at`, `created_at/by`, `updated_at/by`.
  Rows are never deleted; removing sets `removed_at`. Among current officers, one bank
  and team cannot list the same mobile or the same email (any case) twice. The same mobile
  or email in two teams is allowed.
- New `case_officers`: `case_id`, `team`, `officer_id`, `picked_at/by`; append-only, the latest row per
  case and team is current and all rows are the history. The officer must belong to the case's bank and that team (checked in
  the service; the officer's bank and team are fixed once created).
- Archive and restore carry `case_officers` with the case. Officers themselves are not
  archived (they are never deleted).

### 5.3 Picking the officer

- **Add case** (from a lead or a customer): shows an officer dropdown for every team on
  stages up to and including the stage the case starts at. HDFC: TDEP. Standard:
  Officer (whether the case starts at Received or Logged in). Optional.
- **Move stage**: when the target stage has a team, the move dialog shows that team's
  dropdown, prefilled with the case's current officer for that team if any. Optional;
  leaving it empty keeps the current officer (it never clears one).
- The dropdown lists the bank's current officers in that team (name and mobile, or email when there is no mobile), plus
  **"+ New officer"**, which opens a small form (name, mobile, email; one of the two required) and selects the new
  officer. The team and bank are taken from the stage.
- **Case page**: a **Bank officers** box lists each team of the case's stage list that
  the case has reached (the team's stage is at or before the current stage in order):
  officer name, mobile with a call link and/or email with a mail link, and a **Change** button with the same dropdown.
  A reached team without an officer shows **"Officer not set"** in orange. Teams not yet
  reached are not shown. A removed officer still shows on the case, marked "(removed)".
- "Officer not set" does **not** put a case in Needs attention or send alerts (Claude's
  call: the owner made the officer optional).
- Editing an officer's name, mobile or email changes it on every case that points to them.
- Dropped cases: the box is read-only, as other case details are.
- Every pick or change is audited and shows in the case history as
  "DOVR officer: <name>" (change from → to).
- Moving a case backward keeps its officers.
- A bank switched from Standard to HDFC (Task 10): an "Officer" picked on its cases is
  kept in history but not shown, as HDFC has no Officer team. (Claude's call; only test
  data exists.)

### 5.4 Officers page

- New **Bank officers** page in the menu, for all staff: officers grouped by bank, then
  by team; **Add officer** (bank, team from that bank's stage list, name, mobile, email), Edit
  (name, mobile, email), Remove (with a confirm). Removed officers are hidden; cases keep them.
- Admin > Banks gets no new controls.

### 5.5 Permissions and integrity

- Any signed-in staff can add, edit or remove officers (owner). All changes audited.
- Picking a case's officer follows the case edit rule (whoever can edit the case).
- Picking locks the case row, the same pattern as builder contacts.
- The server refuses: an officer of another bank, of another team, or removed; a team
  that is not on the case's stage list.
- Mobile numbers are shown only to signed-in staff, like customer numbers.

## 6. Left to the builder

Exact query and handler shapes; whether the case's officers come in the case response or
a separate call; dropdown component reuse with the builder contact picker; menu label
placement; how the Add case dialog orders the new field.

## 7. Open for the owner

None.
