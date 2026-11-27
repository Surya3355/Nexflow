# Nexflow — Confirmed Decisions and Open Questions

Status: discovery in progress. Record of decisions confirmed with the business owner on 2026-11-10.
Items marked **Open** are not decided and must not be assumed.

## 1. Purpose

- Internal tracking portal for a Direct Selling Agent (DSA) business that helps customers get home loans from banks.
- Staff enter and track information. The system does not collect data from customers and has no customer login.
- Web app first (phone-friendly). A native mobile app comes later. Any later client uses the same backend and rules.

## 2. Workflow stages (generic for now)

Stages are shown in order:

1. Received
2. Logged in
3. Under sanction
4. Sanctioned
5. Documentation
6. Disbursed

- Stages are generic at first. Bank-specific stages will be added later, after the generic flow works.
- The stage list is a lifecycle that can change later. Treat it as configuration, not hard-coded logic.

## 3. Lead data (minimum)

| Field | Required | Input |
|---|---|---|
| Name | Yes | Text |
| Mobile number | Yes | Text |
| Email | No | Text |
| Bank | Yes | Dropdown |
| Loan amount | Yes | Number |

## 4. Tracking data (after "Logged in")

- Tracking number = the bank's loan application number. Entered by staff when the case reaches "Logged in". The system does not generate it. No separate internal case number is needed.

## 5. Data entry style

- Dropdowns for most fields.
- A remarks/description text field for any extra details.
- No modelling of co-applicants, joint borrowers or guarantors for now.

## 6. Access (initial)

- Everyone has access to the system.
- Employees can add, edit and delete cases (applications).
- Only admin or manager can delete a lead.
- Access rules are expected to be tightened later. The server must enforce them on every request, so changes stay in one place.

## 7. Assignment

- The current assignee can reassign a case once their own task is complete.
- No approval is needed for reassignment.
- Assignment history must be kept.

## 8. Administration

- Admin maintains everything: banks, stages, dropdown values and users.
- Support for other staff to maintain the system will be added later.

## 9. Suggested fields (proposed, not yet confirmed)

After sanction:
- Sanctioned loan amount
- Interest rate (fixed or floating)
- Tenure (months)
- Sanction date (recorded when the stage is reached)

After disbursement:
- Disbursed amount
- Disbursement date

Each stage keeps its own date so that reports can use milestone dates.

## 10. Assumptions to keep unless the owner says otherwise

- Admin and manager are separate roles. Both can delete leads. Admin has full rights. Manager does not change bank setup.
- A lead that already has cases cannot be deleted, so no case is lost by accident.

## 10a. Decisions from round 2 (2026-11-10)

- **Duplicates:** when a new lead has a mobile number that already exists, warn the user and link the lead to the existing customer. Do not create a duplicate customer record.
- **Dropped instead of delete:** abandoned leads and cases are moved to a **Dropped** status with a reason. Staff never delete them. Admin and manager can hard-delete leads and cases (confirmed).
- **Dropped reason:** chosen from an admin-maintained dropdown, with an optional remark.
- **Dashboard (first version):** leads received today; cases by stage; sanctions and disbursements for a period; sanctioned and disbursed amounts for a period.
- **Notifications:** follow-up reminders and alerts are deferred. Channel not decided: WhatsApp or email over SMTP.
- **Alert cost check (researched, third-party figures, verify before budgeting):** SMTP via Amazon SES is about $0.10 per 1,000 emails. Brevo paid plans start at about $9/month. WhatsApp costs about ₹0.1 to ₹0.9 per message in India, depending on type and provider markup. Meta's rate card is authoritative. Channel still open.
- **Reporting dates:** "leads received today" uses the date the lead was created. Staff must register each new contact immediately, so created date and contact date are the same in practice.
- **Milestone counting:** a sanction or disbursement report counts every case that reached that stage in the period, even if the case has moved on since. Disbursed is the final stage and marks the case as completed.
- **Dropped after sanction:** still counted as sanctioned for the period, and the row is marked Dropped with its reason.
- **Search:** staff can search by name, mobile number, tracking number and bank. Results show leads and their cases together, grouped.
- **List filters:** stage and bank. Assigned employee, date range and dropped were not selected and need confirmation.
- **Staff email mailboxes:** not needed. Staff do not get `@business` mailboxes. System alerts only need a sender address on the business domain, which needs a domain and an SMTP sending service.
- **Documents (first version):** basic per-case checklist with upload is included in the first version. Requirements per document are still to be defined (O6).
- **Hosting budget:** ₹1,000 to ₹3,000 per month. **To be discussed further.**
- **Operations:** the owner will look after the system after go-live, with outside help as needed. Admin support for other staff comes later.
- **Platform (first version):** web app only, designed to work well on a phone browser. A native iOS and Android app is a later phase. This replaces the earlier "web and mobile" direction in section 1.

## 10b. Decisions from round 3 (2026-11-11)

- **Report export:** reports are viewable on screen and can be exported as CSV (opens in Excel). PDF export is not in the first version.
- **Report access:** all staff can see all reports. Tighten later with the permissions work (O5).
- **List filters:** the case and lead lists include filters for **date range** and **assigned employee**, in addition to stage and bank. The **dropped** filter was not selected; the Dropped status still shows in lists.
- **Deletion history:** when admin or manager hard-deletes a lead or case, the system keeps a full copy of the deleted data in an archive. The copy is hidden from normal lists and can be restored by admin. Exact restore rules and archive retention are not yet defined (see O10).

## 10c. Decisions from round 4 (2026-11-11)

- **Archive retention:** archived copies of deleted leads and cases are kept for **1 year**, then purged automatically.
- **Alert channel:** system alerts use **email via SMTP** in the first version. WhatsApp is not used yet. Requires a business-domain sender address.
- **Permission changes:** only **admin** can change roles and permissions. Manager does not manage roles.
- **Document uploads (first version):** **PDF, JPG and PNG** only, up to **10 MB per file**.

## 10d. Decisions from round 5 (2026-11-11)

- **Employee view:** employees can see **all** leads and cases. Consistent with open access.
- **Document checklists (parked for a later phase):** checklists differ by bank, and some banks need no checklist. Staff are **not** required to complete a checklist before a case can proceed. Checklist handling is recorded as a future feature and is not in the first version's build scope.
- **Alert triggers (email):** the assigned employee is alerted when (a) a lead or case is assigned to them, and (b) a follow-up is due or overdue. The due-date field for follow-ups is not yet defined (see O7).
- **Backups:** daily automatic backup, with the last **30 days** kept.

## 10e. Decisions from round 6 (2026-11-11)

- **Edit rights:** all staff (employees, managers, admin) can edit any lead or case. Delete stays with admin and manager only.
- **Follow-up due date:** optional. Staff can set a follow-up due date on a lead or case. The due-or-overdue email alert is sent on that date, and an overdue alert is sent after it passes. No due date means no alert.
- **Restore of archived items:** a restored lead or case comes back with **Dropped** status and a reason. Staff review it before continuing. The restore action is admin only.
- **Other products:** site/plot and LAP are **not** in the first version. First version is home loans only.

## 10f. Decisions from round 7 (2026-11-11)

- **Document retention:** case documents can be deleted **1 year** after the case is disbursed or dropped. Assumed to be deletable after 1 year. Whether this is automatic or admin-triggered is not stated; see O6.
- **Document versions:** a new upload replaces the old file. Only the latest version is kept.
- **Audit log:** who deleted what, and when, is kept **permanently**, even after the 1-year archive is purged. Matches the audit rule in `agent.md`.
- **Hosting:** data is hosted in a **cloud region in India**. Provider and cost are chosen after technology is discussed (O11).
- **Lead stages:** Received, Contacted, Converted, Dropped. Converted means a case has been created from the lead.
- **Bank stages:** admin sets bank-specific stages in configuration. Until a bank has its own stages, it uses the **standard stages**.
- **Customer fields (v1):** minimum only: name, mobile, email, bank, loan amount. Extra fields come later (O4).

## 10g. Decisions from round 9 (2026-11-11)

- **No separate lead record.** The model is **Customer** and **Case** only. A customer can have many cases.
- **Drops:** customers and cases can be dropped, each with a reason. Leads are not a record type.
- This supersedes earlier references to "lead" in sections 3, 4, 10a, 10b and the O2 row. Lead statuses (Contacted, Converted) are removed. A new case starts at Received.
- **Customer drop cascades:** dropping a customer drops all its cases, each linked to the customer drop. The archive keeps the customer stored with its cases and customer data.
- **Dashboard:** new customers and new cases created today, confirmed.

## 10h. Decisions from round 10 (2026-11-11)

- **Stack:** Go backend with chi, PostgreSQL, React web app. Details in `docs/tech-stack.md`.
- **Login:** username and password. Admin creates users. Users can change their own password. No self sign-up.
- **Still open:** hosting provider and cost, SMTP provider and sender domain, file storage location.

## 10i. Decisions from round 11 (2026-11-12)

- **Document checklist: dropped.** There is no checklist of required documents, now or in v1. Banks differ and some need none. This supersedes the round 2 checklist line (§10a) and the round 5 parking note (§10d).
- **Documents are uploaded at a stage.** Each uploaded document records the stage the case was at when it was uploaded (for example Sanctioned or Disbursed).
- **Uploads are optional by default at every stage.** Admin can mark a stage as **requiring documents** where a bank or process needs them. Which stages require documents is set by admin in configuration.
- **Still open:** what counts as "satisfied" (at least one file, or a named document) and whether a missing upload blocks the move to the next stage or only shows a warning.

## 10j. Decisions from round 12 (2026-11-12)

- **Mobile number entry:** the form shows a fixed +91 prefix, and staff type the 10 digits. If someone pastes a full number, the system strips +91, spaces and other non-digits. Stored value: exactly 10 digits.
- **Archived records do not block reuse:** a mobile number or tracking number that belongs only to an archived customer or case can be used again. Uniqueness applies to live records only.
- **Document rule:** a stage marked as requiring documents is satisfied when the case has at least one uploaded file. A missing file shows a warning and does not block the stage move.
- **Document labels:** free text. Uploads are optional.
- **Technical choices approved:** `sqlc` for database access and `golang-migrate` for migrations. Frontend styling and test tools are chosen in the implementation plan.
- **Hosting, email provider, file storage and PostgreSQL version:** still open. Hosting is deferred.

## 10k. Decisions from round 13 (2026-11-12)

- **Customer drop:** already-dropped cases keep their original reason. Disbursed (completed) cases are dropped too, with the customer's reason.
- **Restore clash:** restoring a customer whose mobile is now used by a live customer is refused.
- **Follow-up alerts:** a due alert goes on the due date, and an overdue alert goes the day after.
- **Customer and cases:** the customer is the top-level record. Each case is linked to its customer ID.
- **Archive:** an archived case keeps a copy of its customer's details with it. An archived customer keeps its own details and the list of its cases.
- **Customer delete:** a customer is deleted only after its cases are deleted. Each case goes to the archive, and the customer's archive entry lists them.
- **Link expiry:** 10 minutes.
- **Frontend and other tool choices:** left to the implementation plan (owner has no preference).

## 10l. Decisions from round 14 (2026-11-21, after first demo)

- **Projects.** Admin keeps a list of projects (name only, rename allowed), same as banks. Every case belongs to one project; **NA** is a fixed entry at the top for loans without a builder project. Existing cases become NA. The project picker is searchable and starts empty.
- **Projects page** in the top bar for all roles, each project's cases with CSV download. A builder role with limited access comes later.
- Project shows on the case and customer pages, is editable from case edit, filters case lists and Reports, is a CSV column, and is found by search (NA excluded from search).
- **Existing mobile in New customer:** offer to add the case to the existing customer. If that customer was dropped, offer to bring them back and add the case; their old dropped cases stay dropped.
- **Tracking number:** letters and digits only, saved in capitals.
- Spec: `docs/superpowers/specs/2026-11-21-projects-and-fixes-design.md`.

## 10m. Round 15 requests (2026-11-22, not yet designed)

- Owner asked for leads (name, mobile, project, assignee; shown on Home), lead to Received with bank, amount and unit number, project details (type, address, contact), a Home filter, and HDFC bank work (officer list, strict stages with proof, bank queries). This reopens O2 (no separate lead record).
- Notes and task list: `docs/superpowers/specs/2026-11-22-round-15-backlog.md`. Nothing here is decided until each task is designed with the owner.
- **Leads (tasks 1 and 2), approved 2026-11-23** (spec `2026-11-23-leads-and-case-page-design.md`): New customer = name, mobile, email (optional), project, assigned to → a **lead** on Home (Leads chip). Lead page: Log contact (response, remark required, next call date), Add case (bank, amount, unit no., start at Received or Logged in; the lead becomes the case), + New project (another lead), Drop with the case drop reasons, Bring back. One open lead per customer and project. A new lead's call is due the day it is added; due and overdue calls show in Needs attention and send reminder emails. Leads are a separate record (supersedes O2). Existing customers with no case became leads on NA.
- **Project details (task 4), approved 2026-11-24** (spec `2026-11-24-project-details-design.md`): project types High-rise apartments, Apartments, Plots, Villas (fixed; more added on request). Address required. Each project has its **own** contact list (name, mobile, optional email and label), independent of other projects; at least one contact except NA. Each lead and case points to one contact of its project (prefilled with the first; lead page, Add case and case page can change it; leads show it too). Removing a contact takes it off that project only; old cases keep it, marked removed. Every save of a project must be complete, including old projects. Admin only. Plots type does not bring plot loans into scope (O12).
- **Restored lead (2026-11-24):** when a deleted lead and its case are both restored by admin, the lead stays **Dropped** (same as cases, O10). Staff then need to move it to the right stage. Stages can already move backwards (corrected 2026-11-25); a dropped item cannot be reopened to a chosen stage. **Open, to design later:** a way to reopen or move a restored lead or case back to the right stage.
- **Case page:** customer name large on top, case number and bank below, details in normal text size, project shown once (in Details), unit no. editable, contact log shown when the case came from a lead, "+ New case" for the same customer.
- **HDFC stages and part disbursement (task 10), approved 2026-11-25** (spec `2026-11-25-hdfc-stages-design.md`): HDFC stages Received, Link sent (start), Application submitted (HDFC application number required), DOVR (TDEP + DOVR), Appraisal, Sanctioned, Upload documents, Under fixing, Fixed, Signing, Disbursement. SBI keeps the standard six. Stages carry a role (start, tracking, sanction, disbursement) used by rules and reports. Sanctioned amount is required and **overwrites the loan amount**. Part payments (Part/Full, each with date and amount) per bank setting: on for HDFC and later private banks, off for SBI; remaining = sanctioned − paid; Full completes the case. Disbursement report: **one row per case** (owner rejected one row per payment). Remark on a stage move is optional. Nexflow does not calculate payout. Moving a dropped case back to a stage: later.
- **Bank officers (tasks 6 and 7), approved 2026-11-26** (spec `2026-11-26-bank-officers-design.md`): each bank keeps a list of officers (name, and a **mobile or an email or both**; some officers give no mobile) per **team**. HDFC teams TDEP (at Link sent), DOVR, Appraisal, Fixing (at Under fixing); no officer for Signing or Disbursement. Standard banks (SBI) have one team "Officer" at Received. The officer changes **per case**: staff pick it on Add case and when moving into a stage with a team, and can change it on the case page. Picking is **optional** ("Officer not set" shown, no alert). **Everyone** can add, edit and remove officers (Officers page; "+ New officer" in every dropdown). One person can be in two teams. Officers are **not** in CSV files. Removed officers stay on old cases.
- **Bank queries (tasks 8 and 9), approved 2026-11-26** (spec `2026-11-26-bank-queries-design.md`): Task 8 "strict stages" meant query proof; the admin "needs a document" tick stays a warning and no stage is strict. Queries on **every bank**: what the bank asked, date raised, assignee, optional due date (not who raised it). Steps Open → Sent to bank (needs a **note or a file/photo**) → Resolved only when the **bank accepted**; Bank rejected (with its reason) reopens it. **Remove** takes away a query added by mistake; **Remove proof** takes away wrong proof (back to Open). Proof hidden until "View proof". An unresolved query **blocks moving forward** (back and drop allowed), puts the case in Needs attention, and sends due/overdue alerts to its assignee while Open. Desktop: queries box inside Progress beside the stages; phone: above the stages. Review fixes (2026-11-26, owner: "fix all"): Edit and Remove also while Sent to bank; a gone proof file shows "File deleted"; a stale action says what happened (already resolved / already sent); a query needs an assignee chosen; the overdue alert goes once for any open query past its due date (so also after a late reopen); the blocked-move message names the count ("the 2 bank queries above" on a phone).

## 10n. Round 16: hardening before production (2026-11-27)

After the code review and QA test of 2026-11-27 (both: not production ready), the owner asked to fix everything found, including medium and smaller items, except email and hosting. Spec: `docs/superpowers/specs/2026-11-27-hardening-design.md`; plan: `docs/superpowers/plans/2026-11-27-hardening-plan.md`.

- **Login lock:** 5 wrong passwords in a row lock that username for 15 minutes; an admin password reset unlocks it at once. Self-service "Forgot password" comes with email.
- **Switching off a leaving employee:** the admin picks who takes over; all their open leads, cases and bank queries move to that person in one step, recorded in history; leads and cases send the "assigned" alert (bank queries have no such alert). The last active admin can't be switched off or demoted; an admin can't switch themselves off. Admin can change roles.
- **Payment dates:** today or earlier (India time); future dates are refused.
- **Phone top bar:** a ☰ menu button lists every page and Sign out.
- **Later, with the owner's domain and hosting:** sending emails, "Forgot password" by email, HTTPS, hosting setup, daily backup schedule with copies off the server.

## 11. Open questions

| # | Topic | Question |
|---|---|---|
| O1 | Tracking number | **Resolved.** "Logged in" = submitted to the bank; the bank issues the loan application number, which is the tracking number. |
| O2 | Lead lifecycle | **Superseded (round 15).** Leads are a separate record again: open, converted (Add case) or dropped, with a contact log. Round 9's "no separate lead record" no longer applies. |
| O3 | Bank stages | **Resolved for v1.** Admin sets bank stages in configuration. Banks without their own stages use the standard stages. Actual HDFC and SBI stage lists are still to be entered by admin. |
| O4 | Customer Data | **Resolved for v1.** Minimum fields only. Extra fields come later. |
| O5 | Permissions | **Resolved for v1.** Everyone views and edits all leads and cases. Admin and manager delete. Only admin changes roles and permissions. Role changes after v1 are a later decision. |
| O6 | Documents | **Resolved.** Upload: PDF, JPG, PNG, max 10 MB per file. Retention: deleted automatically 1 year after disbursed or dropped (round 8). Versions: keep latest only. Checklist dropped (round 11). Uploads optional by default; admin can require documents at a stage (round 11). Requirement rules still open. |
| O7 | Tasks and reminders | **Resolved for v1.** Channel: email via SMTP. Alerts: assignment, and follow-up due or overdue. Follow-up due date is optional. |
| O8 | Reports | **Resolved.** Lead count uses creation date. Sanction and disbursement counts use the date each stage was reached. Dropped-after-sanction is counted and marked. Export: on-screen plus CSV. Access: all staff. |
| O9 | Search and duplicates | **Resolved.** Search by name, mobile, tracking number, bank; grouped results; duplicates warn and link. Filters: stage, bank, date range, assigned employee. Dropped filter not selected. |
| O10 | Audit and deletion | **Resolved.** Hard deletes keep a full archived copy for 1 year. Admin restores as Dropped. Audit log of deletions is kept permanently. |
| O11 | Operations | **Partly resolved.** Backups: daily, 30 days kept. Budget is ₹1,000 to ₹3,000/month, to be discussed further. Owner administers the system after go-live. Hosting: cloud region in India. Still open: provider and cost, after technology is discussed. |
| O12 | Other products | **Resolved for v1.** Home loans only. Site/plot and LAP are later phases with their own terminology and stages. |
