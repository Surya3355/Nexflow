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

## 11. Open questions

| # | Topic | Question |
|---|---|---|
| O1 | Tracking number | **Resolved.** "Logged in" = submitted to the bank; the bank issues the loan application number, which is the tracking number. |
| O2 | Lead lifecycle | **Superseded (round 9).** No separate lead record. Customer and case only. |
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
