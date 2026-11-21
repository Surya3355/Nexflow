# Projects, extra cases for existing customers, tracking number rule

Date: 2026-11-21. Status: draft for owner review.

## 1. Why

After the first local demo the owner asked for three changes:

1. **Projects.** Most home loans are for a flat in a builder's project. Staff want to see all cases of one project together, and a builder login will come later that needs exactly this view. The admin keeps the list of projects, the same way as banks.
2. **A second case for an existing customer.** When staff type a mobile that is already registered in the New customer panel, the app says "already belongs to X, no new customer was created" and drops the bank and amount they typed. There is no way forward from that point. A customer can have many cases, so staff must be able to add the case to the existing customer.
3. **Tracking number rule.** Like the mobile check, the tracking number must accept only letters and digits, and store letters as capitals.

Success: staff open **Projects**, pick a project, see its cases and download them as CSV. Typing a known mobile in New customer leads straight to opening the case for that customer. A tracking number like `hdfc-123 ` is refused with a clear message, and `hdfc123` is saved as `HDFC123`.

## 2. Goals, non-goals, anti-goals

Goals
- A projects list managed by the admin; every case belongs to exactly one project; "NA" for loans with no builder project.
- A Projects page in the top bar for all roles, with per-project case list and CSV download.
- Project shown on the case and in the customer's case list, editable from case edit, filterable in case lists and Reports, in the CSV export, and found by search.
- Existing-mobile flow in New customer continues into opening the case for the existing customer.
- Tracking number: letters and digits only, saved upper case, checked in the browser and on the server.

Non-goals (this round)
- Builder role or builder login, and any per-project access limits. Everyone signed in sees all projects (owner: "for now make it all, we will update the access later").
- Project details beyond the name (no builder name, location, units).
- Deleting or deactivating projects.
- Changing how banks, stages or mobile checks work.

Anti-goals
- A project choice that slows staff down: the picker must find a project from a few typed letters.
- Losing what the user already typed when the mobile turns out to be known.
- Breaking existing cases: they must keep working without manual clean-up.

## 3. Approaches considered

1. **Own projects table, like banks (chosen by owner).** New `projects` table and admin section; `cases.project_id` required. Filters, reports, search and CSV follow the bank pattern. Leaves a clean place for builder access later.
2. **Reuse admin dropdown lists** (`dropdown_values`, category "project"). Less new code, but filters, report joins and "NA on top" become special cases, and builder access later would need a rework.

Owner picked 1.

## 4. Design

### 4.1 Data

- New table `projects`: `id`, `name` (text, not empty, unique ignoring case and surrounding spaces), `is_system` (boolean, true only for NA), plus the usual `created_at/by`, `updated_at/by`. Same shape and audit rules as `banks`.
- Seed one row: name `NA`, `is_system = true`.
- `cases.project_id bigint NOT NULL REFERENCES projects(id)`, with an index. Migration: add the column, set every existing case to NA, then make it NOT NULL.
- Project changes on a case are recorded in the audit log the same way as other case edits.
- New migration `000004_projects` (up and down). Down drops the column and table.

### 4.2 Admin: managing projects

- On the admin **Banks & lists** tab, a **Projects** list next to Banks: shows every project with its number of open cases; **Add project** (name) and **Rename**.
- Admin only, enforced on the server, same permission as adding a bank.
- NA is shown first and cannot be renamed (button hidden; server refuses).
- Duplicate names (ignoring case and spaces) are refused with "A project with this name already exists."

### 4.3 Choosing a project on a case

- **Required** on every new case: in the New customer panel's "First case" part, and in the New case dialog on the customer page. The server refuses a case without a valid project.
- **Searchable picker.** A text box; as the user types, the list shows projects whose name contains the typed letters (ignoring case). **NA always appears at the top** of the list, whatever is typed. The user must pick an item from the list; free text is not saved as a project.
- New customer panel: the project picker sits with bank, amount and assignee and is only needed when a bank is chosen (same as the other case fields today).
- **The picker starts empty** in every new-case form; staff must pick a project or NA each time (owner, 2026-11-21). Leaving it empty shows "Choose a project, or NA."
- **Change project** on the case page: a small dialog with the same picker. (The web app has no general case-edit dialog today, so this is its own action. *Builder's call.*)

### 4.4 Where the project appears

- **Case page:** project name next to bank in the header.
- **Customer page case list:** "Bank · Project · Case #id".
- **Home case table and Reports:** a **Project** filter next to the Bank filter (same searchable picker, plus "All projects"). The case tables show a Project column on wide screens; on phones it joins the bank line.
- **CSV export** (Reports and Projects page): new "Project" column after "Bank".
- **Search:** typing a project name finds that project's cases, grouped like a bank match today. Matching rule follows the existing bank match (partial, ignoring case), **except NA is never matched** by search, since most cases are NA and "na" appears in many names (Nandini, Kannan). *(Builder's call.)*

### 4.5 Projects page

- New top bar item **Projects**, between Home and Reports, for every role (admin, manager, employee).
- `/projects`: list of projects, NA first then by name, each with open-case count and total cases. A search box filters the list as you type.
- `/projects/:id`: project name, the same case table used on Home (with stage, bank, amount, assignee filters), and **Download CSV**, which uses the existing export with the project filter applied. The CSV opens in Excel like the Reports export.
- Shows all cases in the project for every role (no "My cases" default on this page).

### 4.6 Adding a case to an existing customer

- In the New customer panel, when the mobile already belongs to a customer, the warning stays and gains a button: **"Add this case to <name>"**.
- If the user had chosen a bank, the button opens the case for the existing customer with the bank, project, amount and assignee already entered, after the same checks as a normal new case. Errors show in the panel, nothing typed is lost.
- If no bank was chosen, the button becomes **"Open <name>"** and goes to the customer page, where **+ New case** is.
- **Dropped customer (owner, 2026-11-21):** if the existing customer is dropped, the button reads **"Bring back <name> and add this case"**. It makes the customer active again (clearing the drop reason, remark, date and who dropped it from the customer row, and recording the change in the audit log with the previous values), then opens the case, in one transaction. Their old dropped cases stay dropped. With no bank chosen the button is **"Bring back <name>"** and goes to the customer page. Bringing a customer back uses the same permission as adding a record (all roles).
- The server refuses a new case for a dropped customer unless it comes through this bring-back action ("This customer was dropped. Bring them back first."), so the rule holds even without the browser. On the customer page a dropped customer shows a **Bring back** button in place of **+ New case**.
- The name and email typed for the "new" customer are not saved; the existing customer's details are not changed by this flow.

### 4.7 Tracking number rule

- Allowed: letters A to Z and digits 0 to 9 only. No spaces, dashes, slashes or other symbols. Surrounding spaces are trimmed first.
- Letters are turned to capitals: in the input as the user types, and again on the server before saving and before the duplicate check.
- Anything else is refused with: "Tracking number can contain only letters and numbers."
- Applies wherever a tracking number is entered: the move to "Logged in" dialog, and any other place that sets it.
- Search by tracking number also turns the typed text to capitals, so `hdfc123` finds `HDFC123`.
- Existing tracking numbers are left as they are. (The local test data has two that would fail the new rule; they are test entries.)

## 5. Security, permissions, data integrity

- Only admin can add or rename projects; checked on the server, not just hidden in the UI.
- All signed-in roles can read projects and project case lists (same as today's "view all" for cases).
- `project_id` must point to an existing project; the database enforces this with a foreign key and NOT NULL.
- Tracking numbers are normalised on the server, so a request that skips the browser still cannot store lower case or symbols, and the uniqueness check works on the normalised value.
- Archive and restore keep the case's project. Cases archived before this change have no project in their saved copy; restoring one sets it to NA. *(Builder's call.)*
- The project filter on Reports also applies to the summary figures, the same as the bank filter. *(Builder's call.)*

## 6. Testing

- Backend: project create/rename/duplicate/NA-locked, admin-only checks; case create without project refused; project change audited; list/report/CSV/search with project filter; tracking number normalise, refuse symbols, duplicate check on the normalised value; migration sets existing cases to NA.
- Web: picker filtering with NA on top; New customer existing-mobile flow (with and without bank); Projects page list and case table; tracking input turns letters to capitals and shows the error.
- Phone-width check of Projects page and the picker.

## 7. Left to the builder

- Exact layout of the Projects list and picker, within the existing UI style.
- Whether the picker is a shared component (expected: yes, used in four places).
- Query and code structure, following the bank code as the pattern.
