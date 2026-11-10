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

## 11. Open questions

| # | Topic | Question |
|---|---|---|
| O1 | Tracking number | **Resolved.** "Logged in" = submitted to the bank; the bank issues the loan application number, which is the tracking number. |
| O2 | Lead lifecycle | Exact lead stages beyond "Received" (qualified, lost, inactive, and so on). |
| O3 | Bank stages | Bank-specific stages and allowed transitions for HDFC and SBI, to follow the generic flow. |
| O4 | Customer Data | Fields beyond the minimum lead data. |
| O5 | Permissions | Final role permissions after the open-access phase. |
| O6 | Documents | Basic checklist and upload are in v1. Still open: checklist contents per case type, file size limits, retention, versioning. |
| O7 | Tasks and reminders | Due dates, overdue alerts, notification channels. |
| O8 | Reports | **Partly resolved.** Lead count uses creation date. Sanction and disbursement counts use the date each stage was reached. Dropped-after-sanction is counted and marked. Still open: export needs, and who can see each report. |
| O9 | Search and duplicates | **Mostly resolved.** Search by name, mobile, tracking number, bank; grouped results; duplicates warn and link. Still open: whether filters for assigned employee, date range and dropped should also be included. |
| O10 | Audit and deletion | Which actions need permanent history; deletion policy for cases. |
| O11 | Operations | Budget is ₹1,000 to ₹3,000/month, to be discussed further. Owner administers the system after go-live. Still open: hosting choice (after technology is discussed) and backup expectations. |
| O12 | Other products | Site/plot loan terminology and the LAP workflow. |
