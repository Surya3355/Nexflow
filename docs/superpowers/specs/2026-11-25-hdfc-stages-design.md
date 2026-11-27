# HDFC stages and part disbursement (round 15, task 10)

Status: **approved by owner** (2026-11-25). Backlog: `2026-11-22-round-15-backlog.md`, Task 10.
Tasks 6 to 9 (officer list, officer per stage, strict stages, bank queries) build on this and are **not** part of it.

## 1. Why

HDFC moves a case through many teams (TDEP, DOVR, APPR, fixing). The six standard stages
hide where an HDFC case really is, so staff cannot tell who to chase. HDFC also pays some
loans in **parts** (PD) before the **full** disbursement (FD); today a case holds one
disbursed amount and closes on the first one. The payout HDFC sends the business depends
on these disbursements, but Nexflow does **not** calculate the payout (owner).

The bank often sanctions a different amount from what was applied for. Today the
sanctioned amount is captured but Home, the case page, the customer page and the case
report still show the applied amount.

## 2. Owner's process (summary of 2026-11-25)

1. Lead comes in; staff explain the process, take name, mobile, email; HDFC sends the
   customer an application link. Loan amount entered. (Selecting the TDEP officer: Task 6/7.)
2. Customer completes the application online and uploads documents.
3. TDEP officer verifies and passes it to DOVR.
4. DOVR checks everything and raises queries (missing documents, clarifications); then APPR.
5. APPR (appraiser) may raise eligibility queries and sanctions the loan.
6. Staff collect documents from the customer, fill a form, scan and upload.
7. Fixing team may raise queries (latest EC, legal, property); then the case is fixed.
8. Customer visits the bank and signs; then disbursement.
9. Disbursement is part (PD) or full (FD). The case is complete only after full.

## 3. Goals, non-goals, anti-goals

Goals
- HDFC cases use HDFC's own stage list (§5.1). Other banks (SBI) keep the standard six.
- A new HDFC case starts at **Link sent**.
- The sanctioned amount **overwrites** the loan amount when a case is sanctioned.
- Banks with **part payments** switched on (HDFC now; private banks later; not SBI) record
  several payments, each Part or Full, and show the remaining amount. The case completes
  when a Full payment is recorded.
- Reports keep counting sanctions and disbursements across banks; a disbursement report
  shows **one row per case**.
- Any stage move can carry an **optional remark**.

Non-goals
- Payout calculation or payout tracking (HDFC sends it).
- Sending the application link from Nexflow (HDFC's system sends it; Nexflow records the stage).
- Bank officers, officer per stage, strict stages with proof, bank queries (Tasks 6 to 9).
- Reopening a dropped or restored case to a chosen stage (later, owner 2026-11-25).
- An admin screen to add, rename or reorder stages. New bank stage lists are added by us
  in a migration when the owner asks (see §5.2).

Anti-goals
- One case appearing as two rows in the disbursement report (owner: staff would think it
  is two cases).
- Forcing a remark on a backward move (owner: moves are sometimes mistakes).
- Breaking the sanction/disbursement counts in reports when a bank has its own stages.

## 4. Approaches considered

- **A. Mark each stage with what it does (chosen).** Each stage has a *role*: start,
  tracking, sanction, disbursement, or none. Rules, forms and reports use the role, not
  the stage code or name. A later private bank is set up the same way.
- B. Reuse the standard hidden codes (`sanctioned`, `disbursed`) on HDFC's stages. Less
  work now, but "start at Link sent" still needs a separate setting and each new bank
  needs code changes. Rejected.
- C. Write the HDFC flow into the program. Rejected: decision §2 says stages are
  configuration.

Part payments: one sensible way. A **payments list** per case (date, amount, Part/Full)
used by **all** banks; banks without part payments are only offered Full. Every bank then
reports disbursements the same way.

Report layout: Claude proposed one row per payment; **owner rejected it** for one row per case.

Remaining amount: owner chose **sanctioned amount − total paid** (not loan amount).

## 5. Design

### 5.1 HDFC stage list (owner-approved names)

| # | Stage | Role | Notes |
|---|---|---|---|
| 1 | Received | none | Kept so staff can move a case back to it |
| 2 | Link sent | **start** | Add case lands here |
| 3 | Application submitted | **tracking** | Customer finished the online form; HDFC application number required |
| 4 | DOVR | none | TDEP and DOVR together; most queries are raised here |
| 5 | Appraisal | none | APPR officer, eligibility queries |
| 6 | Sanctioned | **sanction** | Sanction form, overwrites loan amount |
| 7 | Upload documents | none | Staff collect, fill the form, scan, upload |
| 8 | Under fixing | none | Fixing team queries |
| 9 | Fixed | none | |
| 10 | Signing | none | Customer signs at the bank |
| 11 | Disbursement | **disbursement** | Part payments, then Full completes |

The HDFC application number (tracking number) is required to reach **Application
submitted** (owner, 2026-11-25).

Standard stages get roles: Received = start, Logged in = tracking, Sanctioned = sanction,
Disbursed = disbursement.

### 5.2 Data

- `stages.role text NULL CHECK (role IN ('start','tracking','sanction','disbursement'))`;
  at most one stage per role per stage list (partial unique index on
  `(bank_id, role) WHERE role IS NOT NULL`, nulls not distinct).
- HDFC stages are a built-in **stage template**. Banks are created by admin (there is no
  seeded HDFC bank), so admin chooses a bank's **stage list: Standard or HDFC** on the
  Banks screen. Choosing HDFC creates that bank's stage rows from the template.
  Builder chooses whether templates live in code or a table.
- Switching a bank that already has cases from Standard to HDFC moves its cases with this
  fixed map (Claude's call; the app is not live, only test data exists): Received → Link
  sent, Logged in → Application submitted, Under sanction → DOVR, Sanctioned → Sanctioned,
  Documentation → Upload documents, Disbursed → Disbursement. Old stage history rows keep
  their old stage. Switching back from HDFC to Standard is not offered.
- `banks.part_payments boolean NOT NULL DEFAULT false`. Admin switches it on the Banks
  screen. Choosing the HDFC stage list switches it on by default.
- New `case_disbursements`: `id`, `case_id`, `amount numeric(14,2) > 0`, `paid_on date`,
  `kind ('part','full')`, `created_at/by`, `updated_at/by`. At most one `full` per case.
  Existing `cases.disbursed_amount/disbursement_date` are migrated into it as one `full`
  row each; builder decides whether the old columns are dropped or kept in sync.
- `case_stage_events.remark text NULL` (optional remark on any move).
- Archive and restore carry `case_disbursements` with the case.

### 5.3 Stage moves

- Any stage, forward or backward, as today. Remark box is always shown and optional; it
  is saved on the history row and shown in the case history.
- Reaching the **tracking** stage needs a tracking number (today's Logged in rule).
- Reaching the **sanction** stage opens today's sanction form (amount prefilled with the
  loan amount, rate, type, tenure, date). Saving sets `sanctioned_amount` **and
  overwrites `loan_amount`** with it. The old loan amount is kept only in the audit/case
  history. Re-sanctioning after a backward move overwrites again.
- Reaching the **disbursement** stage asks for the first payment: amount, date, and
  Part/Full (Part only offered when the bank has part payments on). Full completes the case.
- Dropped cases still cannot move (unchanged).
- The **sanctioned amount is required** on the sanction form (owner: "we need to take
  that input"); rate, type, tenure stay as today. The **first payment is required** when
  moving into the disbursement stage, unless the case already has payments (Claude's call:
  the stage means money was paid).
- Moving a completed case back out of the disbursement stage is allowed (today's rule):
  it becomes active and its purge date is cleared; payments are kept. Moving it into the
  disbursement stage again with a Full payment already recorded completes it again with no
  payment prompt; with only Part payments, the prompt is optional (Claude's call).

### 5.4 Payments on the case page

- Whenever a case has payments, the case page shows a **Disbursements** block: each
  payment (date, amount, Part/Full), **total paid**, and **remaining = sanctioned amount −
  total paid** (falls back to loan amount when there is no sanctioned amount, e.g. old
  migrated cases). **Add payment** shows only while the case is at the disbursement
  stage and has no Full payment. On completed cases the list is read-only except for
  admin/manager edit/delete (Claude's call).
- Recording a **Full** payment completes the case (status `completed`, document purge
  date set, same as today's Disbursed).
- If total paid would exceed the sanctioned amount, show a warning; do not block (Claude's
  call: banks sometimes adjust amounts). Remaining never shows below 0.
- Editing or deleting a wrong payment: admin and manager only, audited (Claude's call).
  Deleting the Full payment reopens the case to active at the disbursement stage.
- Moving a case back out of the disbursement stage keeps its payments.

### 5.5 New case

- Add case (from a lead or a customer) starts the case at its bank's **start** stage.
  Standard banks keep today's choice of Received or Logged in. HDFC-list banks start at
  Link sent with no choice.

### 5.6 Reports and alerts

- **Sanctions** (dashboard tile, Home "Sanctioned this month"): cases that reached the
  stage with role **sanction** in the period, any bank. Amount = sanctioned amount.
- **Disbursements** (dashboard tile): cases with **any payment** in the period. Amount =
  sum of payments in the period. Dropped cases still count and are marked, the same as
  the existing dropped-after-sanction rule (O8; Claude's call). Archived cases are out of
  reports until restored, as today.
- **New disbursement report** on the Reports page with CSV, one row per case that had a
  payment in the period: customer, mobile, bank, project, tracking number, assignee,
  paid in period, total paid, last payment date, Part/Full (Full once a full payment
  exists), sanctioned amount, remaining. Same bank/project filters as the dashboard.
- **Cases by stage** with all banks: grouped per bank, each bank's own stages in order
  (Claude's call; HDFC and standard stages do not share codes).
- Case list report and Home show `loan_amount`, which is now the sanctioned amount once
  sanctioned. No new column.
- **Stuck** alert: skip cases at the disbursement stage (a part-paid case can wait months
  for construction; Claude's call). "Stage moves" count skips each case's first stage
  event instead of the `received` code.
- Every rule that today checks the codes `received`, `logged_in`, `sanctioned`,
  `disbursed` switches to roles.

### 5.7 Admin

- Banks screen: per bank, **Stage list** (Standard / HDFC) and **Part payments** on/off.
- Stages screen: shows each stage list (Standard, and each bank with its own list) with
  today's "Requires documents" checkbox per stage. No add/rename/reorder.

## 6. Security and data integrity

- Payments: staff who can edit a case can add; only admin/manager can edit or delete.
  All changes audited with before/after.
- Payment insert and the case completion happen in one transaction with the case row
  locked (same pattern as stage moves), so a double tap cannot record two Full payments
  (enforced also by the unique index).
- Bank stage-list switch runs in one transaction; refused while another switch runs.

## 7. Open for the owner

None.

## 8. Left to the builder

Template storage; migration shape for the old disbursed columns; exact UI layout following
the existing case page and MoveStageDialog patterns; CSV column order.
