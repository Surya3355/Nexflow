# Leads, lead to case, and case page layout

Date: 2026-11-23. Status: draft for owner review.
Covers backlog tasks 1 and 2 (`2026-11-22-round-15-backlog.md`) and the case page
changes the owner asked for. Mockups: `mockups/round15/` (owner: "mockups look good").

## 1. Why

While testing on a phone the owner added a new customer, went back to Home, and
could not find them to add a case. Home lists cases only, and a customer saved
without a bank has no case.

The business also works this way: most customers start as an enquiry for a
builder's project. Staff call them, sometimes several times, before the customer
comes in and a loan is started. Today none of that is tracked.

The owner also found the case page hard to read: the case number, bank and project
are big while the customer is in small text below, the details text is small, and
the project is shown twice.

Success:
- Staff add a customer with name, mobile, project and assignee, and it appears at
  once in **Leads** on Home.
- Each call is logged with the response, a remark and the next call date, so anyone
  can see where the lead stands.
- When the customer comes in, staff press **Add case**, enter bank, amount and unit
  number, and the lead continues as a normal case.
- The case page leads with the customer's name in large text.

## 2. Goals, non-goals, anti-goals

Goals
- A lead record: customer, one project, assignee, status, next call date.
- New customer form creates a lead (no bank or amount).
- Leads list on Home (basic details only), lead page, contact log.
- Add case from a lead; New project (another lead) for the same customer.
- Drop a lead with the existing drop reasons, and bring it back.
- Call reminders for leads through the existing follow-up email alerts.
- Case page: customer first, larger details, project once, contact log, and
  "+ New case" for the same customer.
- A unit number on the case (flat, villa or plot number).

Non-goals (this round)
- The Home **Filter** button and other Home changes (task 3).
- Project type, address and contact details (task 4). Until then the unit field is
  labelled **Unit no.**; task 4 changes the label by project type.
- Bank officer list, strict stages, bank queries, HDFC stages (tasks 6 to 10).
- Lead counts or lead conversion in Reports. Reports stay as they are.
- Editing or deleting a logged contact.

Anti-goals
- Losing a customer again: every active customer must show on Home, either as a
  lead or through a case.
- Weakening the case rules: a case still always has a bank, an amount and a stage.
- Slowing down staff who already have a walk-in customer: Add case must be one
  step from the lead.

## 3. Approaches considered

1. **A separate lead record (chosen, Claude's call).** New `leads` and
   `lead_contacts` tables. Add case creates a normal case linked to the lead.
   Cases keep their rules (bank, amount, bank's stages) unchanged, and reports
   keep counting what they count today.
2. **A lead as a case at a new first stage, "Lead"** (Claude's earlier
   suggestion). One record, and search, assignment and reminders would work
   without new code. But a case needs a bank and an amount, and stages belong to
   each bank. A lead has neither, so the bank and amount rules, the progress bar,
   stage reports and the "stuck in stage" check would all need exceptions.
   Dropped for that reason.

The owner left the how to Claude. This replaces round 9's "no separate lead
record" (O2).

## 4. Design

### 4.1 Data

New table `leads`:
- `customer_id` (required), `project_id` (required, NA allowed), `assignee_id`
  (required).
- `status`: `open`, `converted` or `dropped`.
- `next_call_date` (date, optional). Set to the IST day the lead is created.
  Each logged contact replaces it with the contact's next call date, or clears it
  when that is empty.
- Drop fields as on cases: reason, remark, when, by whom.
- `converted_at`, plus the usual created and updated by and at.

New table `lead_contacts` (append-only):
- `lead_id`, `response`, `remark`, `next_call_date` (optional), `created_at`,
  `created_by`.
- `response` is one of: `interested`, `call_back`, `not_reachable`,
  `not_interested`.
- Remark is required (the owner's "their response in the remark block").

Cases:
- `lead_id` (optional, unique): the lead this case came from.
- `unit_number` (optional text, trimmed, saved in capitals like tracking numbers,
  max 20 characters).

Alerts: `alert_log` gets an optional `lead_id`. Each alert row has exactly one of
`case_id` or `lead_id`.

**Migration of existing data.** Every active customer with no cases gets one open
lead: project NA, assigned to the person who created the customer (or the first
admin if unknown), next call today. This brings back the customer the owner lost
in testing.

### 4.2 New customer

Home → **+ New customer** opens the panel from mockup 2: name, mobile, email
(optional), project (searchable picker), assigned to (starts as the signed-in
user). **Save lead** creates the customer and the lead in one transaction, then
opens the lead page.

Bank and amount leave this form.

Known mobile: the panel keeps today's behaviour (round 14, §4.6), but offers
**Add lead for this customer** instead of opening a case. A dropped customer is
brought back first, in the same step. Their old dropped cases and leads stay
dropped.

### 4.3 Leads on Home

A **Leads** chip sits first, before Needs attention, My cases and All cases. It
lists open leads for all staff (mockup 1). Each row shows:
- name and mobile;
- project and assignee;
- last response (or **Not contacted**);
- next call date (or the date added).

Sort order: leads with a next call date that is today or past come first, oldest
first. Then the rest, newest first.

Needs attention stays and also lists open leads whose next call date is today or
past ("Call due today", "Call overdue by N days").

A customer who has both an open lead and a case shows in both lists. That's
expected.

### 4.4 Lead page

Route `/leads/:id` (mockup 3). At the top, in large text: customer name, mobile
(tap to call) and email. Then a Details section in readable text: project,
assigned to (change), next call, added on and by.

Buttons, for open leads:
- **Log contact:** response chips, remark, next call date (optional). Saving adds
  the contact and updates the lead's next call date. Choosing **Not interested**
  asks "Drop this lead?" with the drop reason picker, and the user may decline.
- **Customer is in: Add case** (section 4.5).
- **+ New project:** creates another open lead for the same customer (project and
  assignee only).
- **Drop…:** the existing drop dialog and reasons.

The contact log is listed newest first: response, remark, date and time, who, and
next call.

For dropped leads: a **Bring back** button reopens the lead (bringing the customer
back too if they were dropped). For converted leads: a link to the case.

The customer page lists the customer's leads (open and closed) above their cases.

### 4.5 Add case from a lead

Mockup 4. The form shows the lead's project (fixed) and asks for:
- unit no.;
- bank;
- loan amount;
- assigned to (starts as the lead's assignee);
- start at: **Received** or **Logged in**. Logged in also asks for the tracking
  number, using the existing rule.

Saving, in one transaction:
- creates the case with `lead_id`, the lead's project and the unit number;
- writes the stage history rows (Received, plus Logged in if chosen);
- marks the lead `converted`, and clears its next call date.

The lead leaves the Leads list. Add case counts as a successful contact; no
contact row is needed.

Adding a case directly, without a lead, stays possible from the customer page and
from **+ New case** on the case page. It uses the same form with a project picker.

### 4.6 Case page

Mockup 5.
- **Top section:** the customer name, large and linked to the customer page. Under
  it, mobile (tap to call), bank and case number. The amount and stage badge sit on
  the right, with the progress bar below.
- **Details section**, in body text size, one item per line: project (change), unit
  no. (change), tracking no., assigned to (change), opened. The project appears only
  here.
- **Contact log** section, when the case came from a lead (read-only).
- **+ New case for <first name>**, next to Drop.

### 4.7 Search, drop, delete

- **Search:** customer results show the customer's open leads next to their cases.
- **Dropping a customer** also drops their open leads, with the same reason, like
  their cases today.
- **Deleting a customer** (admin or manager) includes their leads and contact log in
  the archive snapshot. Restore brings the leads back as dropped. A customer with an
  open lead cannot be deleted. They must be dropped first, the same rule as live
  cases.
- **Deleting a case** that came from a lead leaves the lead as converted. Its link
  to the case is cleared.

### 4.8 Reminders

- The daily follow-up run also queues **call due** and **call overdue** emails for
  open leads to their assignee, keyed by lead and date, so each is sent once.
- Assigning or reassigning a lead sends the existing assignment email.

## 5. Security, permissions, data integrity

- Same roles as cases: every signed-in staff member views, adds, logs contacts,
  assigns, drops and brings back leads. Admin and manager delete, through the
  customer.
- Lead creation, contact logging and conversion each run in one transaction, with
  audit entries (`lead.created`, `lead.contacted`, `lead.assigned`,
  `lead.converted`, `lead.dropped`, `lead.brought_back`).
- Conversion locks the lead row, so two people pressing Add case at once cannot
  create two cases. The unique `cases.lead_id` is the last guard.
- A converted or dropped lead accepts no new contacts and cannot be converted.
- The contact log cannot be edited or deleted through the app.
- Server checks repeat every browser check: response value, remark required,
  project exists, assignee is an active user, unit number length and characters.

## 6. Testing

- Go service tests (red first) for: lead create (new and known mobile, and a
  dropped customer brought back); contact log and next call update; drop and bring
  back; convert at Received and at Logged in (tracking required); double convert
  refused; customer drop cascading to leads; customer delete with an open lead
  refused; the archive snapshot including leads; lead follow-up alerts queued once;
  the migration creating leads for case-less customers.
- Web tests for the new customer panel, the Leads list and its order, the lead page
  actions, the Add case form, and the case page layout (customer name as the
  heading, project once).
- Phone Playwright run of the main flow: new customer → log contact → add case.

## 7. Left to the builder

- Exact API paths and JSON names, following the existing `/api/cases` style.
- Component split in the web app; reuse `DropDialog`, `ProjectPicker` and
  `SidePanel`.
- How the archive snapshot nests leads.
- Wording of empty states and confirmations, kept short and plain.

## 8. Claude's calls for the owner to check

1. A separate lead record instead of a "Lead" stage (section 3).
2. A new lead's next call is the day it is added, so the assignee gets a reminder.
3. Remark is required on every contact.
4. Unit no. is optional, and saved in capitals.
5. Existing customers without a case become leads on project NA (section 4.1).
6. The Leads list shows everyone's open leads, not only mine. A "mine" filter
   comes with task 3.
7. Needs attention also shows lead calls that are due or overdue.
8. Add case offers Received or Logged in, as the owner said.
