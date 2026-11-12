# Nexflow v1 — Design Spec

Status: **Approved by the owner (2026-11-12).** Implementation follows `docs/superpowers/plans/2026-11-12-nexflow-v1-plan.md`. Rounds 11 and 12 changes are included.
Date: 2026-11-11
Source of truth for business rules: `docs/00-decisions.md` (rounds 1–9). Every rule below cites its round. Items marked **[my call]** are design choices made here, not owner decisions. Items marked **[open]** need an answer.

**Round 9 change (owner):** there is no separate lead record. The model is **Customer** and **Case** only. A customer can have many cases. Customers and cases can be dropped.

---

## 1. Intent and why

A small Direct Selling Agent (DSA) business helps customers get home loans from banks (HDFC, SBI). Staff currently track customers and their loan cases in a way that is hard to see across the team. Nexflow is an internal tool that gives staff one place to record every customer and loan case from first contact to disbursement, see where each case stands, and get basic numbers for the business.

Success for v1: staff can find any customer or case in seconds, see its stage and history, get reminders for follow-ups, and the owner can see the daily and period numbers without asking staff.

## 2. Users and roles

Internal staff only. No customer login, no customer portal.

| Role | Who | Summary |
|---|---|---|
| Admin | Owner or delegate | Full rights. Maintains banks, stages, dropdown values, users and roles. Restores archived items. |
| Manager | Team lead | Can hard-delete customers and cases. Does not change bank setup or roles. |
| Employee | Staff who handle customers | Adds, views and edits all customers and cases. Cannot hard-delete. |

Source: rounds 1, 2, 5, 6, 8 (round 2 confirms admin and manager hard-delete; round 8 confirms employees cannot delete).

**Note on an older line:** §6 of the decisions file says employees can "delete" cases. Round 2 later states "Staff never delete them" and that only admin and manager can hard-delete. This spec follows round 2 and round 8.

## 3. Scope of v1

**In scope**
1. Customer capture with duplicate warning, and customer drop with reason.
2. Case creation for a customer (new or existing), bank, stage flow, tracking number, case drop with reason.
3. Assignment and reassignment of cases, with history.
4. Stage history with dates for each milestone.
5. Per-case document upload. Optional at every stage by default. Each file records the stage the case was at when uploaded. Admin can mark a stage as requiring documents (§5). No checklist.
6. Search and list filters.
7. Dashboard and reports, viewable on screen and exportable as CSV.
8. Email alerts for case assignment and follow-up due or overdue.
9. Admin screens for banks, stages, dropdown values, users and roles.
10. Archive of deleted items, audit log, daily backups.
11. Web app that works well in a phone browser.

**Out of scope for v1 (do not build)**
- Customer login or portal. Customer data collection.
- Commission, accounting, call logging, employee monitoring.
- Native iOS or Android app (later phase; same backend and API).
- WhatsApp alerts. PDF report export.
- Document checklists of any kind. Dropped (round 11): banks differ and some need none.
- Bank-specific stage lists for HDFC and SBI. Admin enters them later. Until then, the standard stages apply.
- Site/plot and LAP products.
- Co-applicants, joint borrowers, guarantors.
- Customer fields beyond the minimum (no address, no PAN in v1).
- Staff `@business` mailboxes.
- A separate lead record or lead stages (round 9).

## 4. Data model

Two records: **customer** and **case**, per `agent.md` §3 (a customer can have many cases).

### 4.1 Customer
- Name (required), mobile (required, unique among customers), email (optional) (round 1, §3; round 9).
- **Mobile format (round 12):** exactly 10 digits. The form shows a fixed +91 prefix. A pasted full number has +91, spaces and other non-digits stripped. Archived customers do not block reuse of their mobile number.
- Status: **Active** or **Dropped** (round 9). Dropped requires a reason from the admin-maintained list plus optional remark (round 2).
- Created date recorded.
- One customer has **many cases** (round 9).
- **[my call]** A customer is created the first time a mobile number is entered. If the mobile already exists, show a warning and link the new case to the existing customer. Do not create a second customer (round 2).
- **[my call]** A customer has no assignee of its own. Assignment is on cases.
- Dropping a customer drops **all its cases** too. Each case is set to Dropped and linked to the customer drop, with the same reason (round 9).
- A customer that has cases cannot be hard-deleted, so no case is lost by accident (round 1, §10). Only the customer's cases can be deleted first.

### 4.2 Case
- Linked customer (required).
- Bank (dropdown, required), loan amount (required, number) (round 1, §3).
- Stage (see §5). A new case starts at **Received** (round 1, §2).
- Tracking number: the bank's loan application number. Entered by staff at "Logged in". The system does not generate it (round 1, O1). **[my call]** tracking numbers are unique among live cases, so a duplicate entry is refused. Archived cases do not block reuse (round 12).
- Current assignee.
- Status: **active**, **Dropped** (with reason and optional remark), or **completed** (Disbursed, round 2).
- Dropped reason: from an admin-maintained dropdown plus optional remark (round 2).
  - **Starter list (confirmed in round 8):** Customer not interested; Not eligible; Rejected by bank; Customer chose another bank or DSA; Customer unreachable; Duplicate or already processed; Other (remark required).
- Created date recorded (used for "cases received today", §9).
- Follow-up date: optional (round 6). Drives the due or overdue alert.
- **Proposed, confirmed in round 8 (§9 of decisions file):** sanctioned loan amount, interest rate (fixed or floating), tenure (months), sanction date; disbursed amount, disbursement date. Each stage keeps its own date.

### 4.3 Stage history and assignment history
- Every stage change adds a new event with the date, the user and the stage. Earlier milestones are never overwritten (`agent.md` §3).
- Every assignment or reassignment adds a history event (round 1, §7).

### 4.4 Documents
- Attached to a case. Types: **PDF, JPG, PNG**. Max **10 MB** per file (round 5).
- Each document records the stage the case was at when it was uploaded, for example Sanctioned or Disbursed (round 11).
- Uploads are optional by default. Admin can mark a stage as requiring documents (§5). Rule: at least one file, warning only (round 12).
- Only the **latest version** is kept. A new upload replaces the old file (round 7).
- Files are private. Access uses short-lived, authorized links (`agent.md` §3).
- Deleted **automatically** 1 year after the case is disbursed or dropped (round 7; automatic confirmed in round 8).

### 4.5 Archive
- A hard-deleted customer or case is kept as a full copy in the archive, hidden from normal lists (round 6). An archived customer is stored **together with its cases and customer data**, so the cases stay linked to it (round 9).
- Admin can restore an archived item. It comes back with **Dropped** status and a reason, so staff review it before continuing (round 6).
- Archived copies are purged **1 year** after deletion (round 3).
- Archived documents are purged on the same 1-year rule as §4.4 **[my call]**.
- **Round 13:** an archived case keeps a copy of its customer's details. An archived customer keeps its details and the list of its cases. Deleting a customer means deleting its cases first, as in §4.1.

### 4.6 Audit log
- Permanent. Records who did what and when: customer creation, case creation, assignment, reassignment, stage change, drop, delete, restore (`agent.md` §3).
- The audit log is **not** purged when the archive is purged (round 7).

## 5. Stage flow

Generic stages, in order (round 1, §2):

1. Received
2. Logged in
3. Under sanction
4. Sanctioned
5. Documentation
6. Disbursed

- Stages are **configuration**. Admin maintains them. No bank logic is hard-coded (`agent.md` §3).
- A bank with no bank-specific stages uses the standard stages (round 7).
- Disbursed is the final stage and marks the case completed (round 2).
- Stages can move **backward as well as forward** in v1 (round 8). Each change adds a dated history event. Restrictions on stage moves are a future enhancement.
- **[my call]** Tracking number is required when a case moves to "Logged in".
- Dropped can be set from any active stage, with a reason. A dropped case keeps its stage history.
- **Document requirement per stage (round 11):** admin can mark any stage as requiring documents. Off by default. A stage is satisfied when the case has at least one uploaded file. A missing file shows a warning and does not block the move (round 12).

## 6. Permissions

Permissions are checked on the server for every request. The client is never trusted (`agent.md` §3).

| Action | Admin | Manager | Employee |
|---|---|---|---|
| View all customers, cases, documents | Yes | Yes | Yes |
| Add customer / case | Yes | Yes | Yes |
| Edit any customer / case | Yes | Yes | Yes |
| Upload document | Yes | Yes | Yes **[my call]** |
| Drop customer / case (with reason) | Yes | Yes | Yes **[my call]** |
| Assign or reassign case | Yes | Yes | Yes, at any stage (round 8) |
| Hard-delete customer / case (to archive) | Yes | Yes | No |
| Restore archived item | Yes | No | No |
| Manage banks, stages, dropdowns | Yes | No | No |
| Manage users and roles | Yes | No | No |
| View and export reports (CSV) | Yes | Yes | Yes |

**Resolved (round 8):** anyone can assign or reassign at any stage. Stages run up to Disbursed, which marks the case complete.

Each row in this table becomes at least one test that proves the server refuses an unauthorized action (`agent.md` §5).

## 7. Assignment

- A case has one current assignee.
- Any staff member can assign or reassign a case at any stage (round 8). No approval is needed (round 1).
- Each reassignment keeps history (who, from, to, when).
- Alerts go to the new assignee on assignment (§10).

## 8. Duplicates, search and filters

**Duplicates (round 2):** when a new customer's mobile already exists, show a warning and link the new case to the existing customer. Do not create a second customer.

**Search (round 2):** by name, mobile number, tracking number and bank. Results show customers and their cases together, grouped by customer.

**List filters (round 6, round 3):** stage, bank, date range and assigned employee. The dropped filter was **not** selected. Dropped items still appear in lists with their status.

## 9. Dashboard and reports

**Dashboard, first version (round 2):**
- New customers and new cases created today **[my call]**. Round 2 said "leads received today"; with no lead record, this is the closest equivalent. Confirmed by owner in round 9.
- Cases by stage.
- Sanctions and disbursements for a selected period, with counts and amounts.

**Counting rules (round 2):**
- New-customer and new-case counts use the created date.
- Sanction and disbursement counts use the date each stage was reached.
- A case that reached a stage in the period counts for that period, even if it has moved on since.
- Dropped after sanction still counts as sanctioned for the period, and the row is marked Dropped with its reason.

**Reports (round 3):** viewable on screen; export as **CSV** (opens in Excel). No PDF in v1. Visible to all staff.

## 10. Alerts (email)

- Channel: **email over SMTP** (round 5). Requires a sender address on the business domain. Domain and SMTP provider to be discussed with the tech stack (round 8).
- Triggers (round 5):
  1. A case is assigned to the employee.
  2. A case follow-up is due or overdue (requires the optional follow-up date, §4.2).
- Follow-up with no date sends no alert (round 6).
- Reminders for other events, WhatsApp, and other channels are out of scope for v1.

## 11. Admin configuration

Admin can maintain, without code changes:
- Banks.
- Stages per bank (generic stages are the default).
- Dropdown values, including the **Dropped reason** list for customers and cases.
- Users and roles.

## 12. Non-functional requirements

- **Web app**, works well in a phone browser with no horizontal scroll (round 2, §11).
- **Server-side authorization** on every request (`agent.md` §3).
- **Private files** with short-lived links (`agent.md` §3).
- **Data hosting in an India cloud region** (round 7). Provider and cost to be discussed together with the tech stack (round 8).
- **Backups:** daily, last 30 days kept (round 5).
- **Budget:** ₹1,000 to ₹3,000 per month for hosting, to be discussed further (round 2).
- **Operations:** the owner runs the system after go-live, with outside help as needed (round 2).
- **Audit log** permanent (§4.6).
- **Technology stack:** Go with chi, PostgreSQL, React, username and password login with admin-created users (round 10). See `docs/tech-stack.md`.

## 13. Acceptance checks for v1

Each check becomes a test or a manual test script in the plan.

1. A new customer with a new mobile is created with status Active.
2. A new customer with an existing mobile shows a warning and links the new case to the existing customer. No second customer is created.
3. A customer can have several cases, and each case shows under the same customer.
4. A case starts at Received and moves through the stages in order, and each move adds a dated history event.
5. Tracking number is required at "Logged in" and cannot be duplicated.
6. A customer or case can be dropped with a reason, and it stays visible with Dropped status. Dropping a customer drops all its cases, each linked to the customer.
7. An employee cannot hard-delete. A manager or admin can; the item moves to the archive and the audit log records it.
8. A customer that still has cases cannot be hard-deleted.
9. Admin can restore an archived item; it returns as Dropped.
10. An archived item and its documents are purged after 1 year. The audit entry remains.
11. Search by name, mobile, tracking number and bank returns results grouped by customer.
12. Filters by stage, bank, date range and assigned employee work together.
13. Dashboard counts match the counting rules in §9.
14. CSV export opens in Excel and matches the on-screen report.
15. Document upload accepts only PDF, JPG and PNG up to 10 MB, and keeps only the latest version.
16. Documents are not reachable without a valid short-lived link.
17. Assignment email goes to the new assignee. A follow-up due or overdue email goes on the due date.
18. Only admin can change roles, banks, stages and dropdowns. The server refuses each other role.
19. Daily backup runs and retains 30 days.
20. Any staff member can assign or reassign a case at any stage, and each change is recorded in assignment history.
21. Each uploaded document shows the stage the case was at when it was uploaded. Uploads are optional at stages not marked as requiring documents. No checklist is shown.

## 14. Open items for the owner

| # | Item | Status |
|---|---|---|
| 1 | Hosting provider and cost, with the tech stack discussion (§12) | **Deferred** to tech stack discussion. |
| 2 | Sender domain and SMTP provider, with the tech stack discussion (§10) | **Deferred** to tech stack discussion. |
| 3 | Document requirement per stage: what counts as satisfied (at least one file, or a named document), and does a missing upload block the stage move or only warn? (§5, §4.4) | **Resolved** (round 12): at least one file on the case; missing file shows a warning only. |

Resolved in round 12: mobile format and archive reuse (§4.1), tracking uniqueness among live cases (§4.2), document rule (§5), free-text document labels. Resolved in round 11: the document checklist is dropped; each document records the stage at upload (§3, §4.4).

Resolved in round 9: dropping a customer drops all its cases, and the archive keeps the customer with its cases (§4.1, §4.5); dashboard counts of new customers and new cases today confirmed (§9).

Resolved in round 8: document deletion is automatic after 1 year (§4.4); proposed case fields are confirmed (§4.2); employees cannot delete (§2); stages can move backward (§5); anyone can assign or reassign at any stage (§6, §7); dropped reason list confirmed (§4.2).

Changed in round 9: no separate lead record. Customer and case are the only records. Lead statuses (Contacted, Converted) are removed.

## 15. Assumptions and my calls (for owner to check)

- A customer is created the first time a mobile is entered; a repeat mobile links to the existing customer (§4.1).
- Customers have no assignee; cases do (§4.1).
- Tracking numbers are unique among live cases (§4.2, round 12).
- Stage changes can go backward as well as forward in v1. Restrictions are a future enhancement (§5).
- Tracking number is required at "Logged in" (§5).
- Employees can upload and drop, but cannot hard-delete (§6).
- Archived documents follow the same 1-year rule as archived records (§4.5).
- A new case starts at Received. The lead-only stages Contacted and Converted are removed (§4.2).

## 16. Review path

This draft is written for review. Next steps:
1. Owner answers the two open items in §14 (or confirms the drafted assumptions).
2. Update this document with the answers.
3. Approve the document.
4. Write the implementation plan (`docs/superpowers/plans/`) with small, testable steps, per `agent.md` §5.
5. Commit only when the owner asks.
