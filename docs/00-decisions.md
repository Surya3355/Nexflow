# Nexflow — Confirmed Decisions and Open Questions

Status: discovery in progress. Record of decisions confirmed with the business owner on 2026-11-10.
Items marked **Open** are not decided and must not be assumed.

## 1. Purpose

- Internal tracking portal for a Direct Selling Agent (DSA) business that helps customers get home loans from banks.
- Staff enter and track information. The system does not collect data from customers and has no customer login.
- Web and mobile. Both use the same backend and rules.

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

- Tracking number. Generated when the case reaches "Logged in" (see Open question O1).

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

## 11. Open questions

| # | Topic | Question |
|---|---|---|
| O1 | Tracking number | Is "Logged in" the point where the file is submitted to the bank and the tracking number is issued? |
| O2 | Lead lifecycle | Exact lead stages beyond "Received" (qualified, lost, inactive, and so on). |
| O3 | Bank stages | Bank-specific stages and allowed transitions for HDFC and SBI, to follow the generic flow. |
| O4 | Customer Data | Fields beyond the minimum lead data. |
| O5 | Permissions | Final role permissions after the open-access phase. |
| O6 | Documents | Checklist contents, file size limits, retention, versioning. |
| O7 | Tasks and reminders | Due dates, overdue alerts, notification channels. |
| O8 | Reports | Metric definitions, filters, export needs, who can see each report. |
| O9 | Search and duplicates | Search fields and duplicate handling for customers and leads. |
| O10 | Audit and deletion | Which actions need permanent history; deletion policy for cases. |
| O11 | Operations | Hosting, budget, backups, production administrator. Deferred until technology is discussed. |
| O12 | Other products | Site/plot loan terminology and the LAP workflow. |
