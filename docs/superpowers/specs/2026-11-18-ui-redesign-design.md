# Nexflow UI Redesign and Role-Based Pages — Design Spec

Status: **Approved by the owner (2026-11-18).** 7-day "needs attention" threshold confirmed; other [my call] items accepted.
Builds on: `docs/superpowers/specs/2026-11-11-nexflow-v1-design.md` and `docs/00-decisions.md`.
Visual reference: the throwaway mockups in `mockups/` (`home-wide.html`, `customer.html`, `case.html`, `reports.html`, `admin.html`, `login.html`).
Items marked **[my call]** are design choices made here, not owner decisions.

---

## 1. Intent and why

The v1 screens work but feel like filling in forms: every page is a stack of labelled inputs and black buttons, the case page hides where a case stands behind a dropdown, and admin can add users and banks but never see the ones that exist. The owner wants the app to feel simple and quiet ("no noise"), to use the width of a desktop screen, and to show each person only the pages that matter to their role.

Success: staff open Home and see their cases with stage, bank and amount at a glance; moving a case to the next stage is one button; admin sees the team, banks and stages as lists; nobody sees menu items they do not use.

## 2. Goals, non-goals, anti-goals

**Goals**
- Reduce 9 screens to 6: Login, Home, Customer, Case, Reports, Admin.
- Show pages by role (section 3).
- Use desktop width (up to ~1320px) and fold cleanly to a phone layout.
- Add the server data the new screens need (section 6).

**Non-goals**
- No board view or split view (versions B and C were considered and not chosen).
- No change to business rules, permissions or the database model beyond what section 6 lists.
- No new UI component library; keep Tailwind.

**Anti-goals** (failure even if it works)
- Clutter: extra panels, banners or controls that are not in the approved mockups.
- Hiding a page in the UI being treated as security. The server keeps enforcing every permission as today.
- Breaking the phone layout (`web/e2e/phone.spec.ts` must keep passing).

## 3. Pages by role (owner decision, 2026-11-18)

| Role | Menu | Lands on after sign-in | Home opens on |
|---|---|---|---|
| Admin | Home, Reports, Admin | **Admin** | All cases |
| Manager | Home, Reports | Home | **All cases** |
| Employee | Home, Reports | Home | **My cases** |

- A non-admin who opens `/admin` is sent to Home (as today).
- Customer and Case pages are reachable by every role (decisions: everyone views and edits all records).
- **[my call]** Admin's Home opens on "All cases", since admin normally has no assigned cases.

## 4. Approaches considered

| Version | Shape | Outcome |
|---|---|---|
| A. List + pages | Home is a case list; Customer and Case are their own pages | **Chosen by owner**, with the desktop layout from `home-wide.html` |
| B. Board | Columns per stage, case opens in a side panel | Not chosen: busier, sideways scrolling on phones |
| C. Split view | Inbox-style list on the left, customer and cases on the right | Not chosen: cramped on phones |

## 5. Screens

### 5.1 Top bar (all signed-in pages)
Logo, menu links filtered by role (section 3), and an avatar initial that opens a small menu: **Change password**, **Sign out**. Change password uses the existing `POST /api/me/password`.

### 5.2 Home (merges old Search, Cases, New customer)
- Greeting, **+ New customer** button.
- One search box. Typing (debounced **[my call]** 300 ms) searches name, mobile, tracking number and bank using `GET /api/search`; when the box is empty the list shows filtered cases instead.
- Filter chips: **My cases**, **All cases**, **Needs attention** (with counts), then **Bank**, **Stage**, **Assigned to**, **Date** as small drop-down chips. Stage is enabled only after a bank is chosen (as today).
- Desktop: rows are columns: Customer (name, mobile, case #) · Bank · Stage (6-step progress bar, stage name, warning badge) · Assigned · Updated · Amount. Phone: the compact row from `index.html` (name, progress bar, amount, badge).
- Right side panel on wide screens: **Today** card (new customers, new cases, stage moves today, sanctioned this month, link to Reports) and **Needs attention** card (up to 5 cases with a one-line reason). Under 1100px the panel moves above the list; on phones the Today card and the attention card are hidden and the chip does that job.
- **[my call]** Show the first 50 rows with a "Show more" link; the list endpoint is not paginated yet.
- **New customer panel** (slides in from the right): name, mobile, optional email, then an optional **First case** block (bank, amount, assigned to, default me). Save creates the customer, then the case if bank and amount are filled. If the mobile already exists, show the existing-customer link and create nothing (current rule). If the case step fails, the customer stays saved and the panel shows the error with a link to the customer.

### 5.3 Customer
- Header card: name, mobile, email, customer since; buttons **Call** (`tel:` link), **Edit**, **+ New case**.
- Edit opens a small dialog for name, mobile, email using the existing `PUT /api/customers/{id}`.
- Case list in the Home row style. **Drop customer…** is a small red button that opens a dialog with reason and remark (existing endpoint).
- New case opens a dialog (bank, amount, assigned to).

### 5.4 Case
- Header card: case #, bank, customer link, mobile, tracking, amount, stage badge, progress bar.
- **Progress** card: the stage list as steps (done / current / to do) with date and who moved it; one primary button **Move to <next stage> →** and an **Other stage…** link that lets the user pick any stage of the bank (forward or back, as today).
- The move dialog asks only what the target stage needs: tracking number for Logged in; sanction details for Sanctioned; disbursement details for Disbursed (fields as accepted by `ChangeStage` today).
- Side column: **Documents** card (missing-document warning, list with open links, **+ Upload** dialog), **Details** card (assigned to with **change**, using the existing assign endpoint; opened date), **Drop this case…** dialog.

### 5.5 Reports (merges old Dashboard and Reports)
- Period chips: Today, This week, This month (default), Custom (from/to). Bank chip.
- Tiles: New customers, New cases, Sanctioned (count, amount, dropped since), Disbursed (count, amount).
- **Open cases by stage** as simple bars.
- Table of cases in the period with case links; **Export CSV** uses the same filters.
- One set of filters drives tiles, bars, table and CSV.

### 5.6 Admin (tabs; admin only)
- **Team**: table of users (name, username, role, open cases count, active), **+ Add person** dialog, **Reset password** per user.
- **Banks & lists**: banks table (name, "Own stages" or "Standard stages", **Edit** to rename), **+ Add bank**; drop reasons list with **+ Add**.
- **Stages**: standard stages table with the "Needs documents" checkbox (existing).
- **Archive**: archived records with archived date, delete-on date, **Restore…** (existing).

### 5.7 Login
Same fields, cleaner card. After sign-in, admin goes to `/admin`, others to `/`.

## 6. Server changes

All new endpoints sit behind `RequireUser`; admin-only ones check permissions in the service, as today. Each change is audited where existing similar actions are audited.

| Need | Change |
|---|---|
| Home columns | `GET /api/cases` rows also return `customer_name`, `customer_mobile`, `bank_name`, `stage_name`, `stage_order`, `stage_count` (for the progress bar), `assignee_name`, `stage_since` (time the current stage was reached) and `needs_document` (current stage requires a document and the case has none). Existing fields stay. |
| Needs attention | `GET /api/cases/attention?assignee_id=` returns active cases with a reason. **[my call]** Rules: (a) current stage requires a document and none is uploaded; (b) in the same stage for 7 or more days and not Disbursed. Dropped and completed cases are excluded. |
| Today card | `GET /api/reports/dashboard` adds `stage_moves_today` (stage events reached today, IST). |
| Reports period | Dashboard accepts `from` / `to` so tiles follow the period chips (the service already takes a `Period`). |
| Team tab | `GET /api/admin/users` (admin only) returns id, username, full name, role, email, active, open case count. |
| Reset password | `POST /api/admin/users/{id}/password` (admin only) sets a new password (8+ characters, same rule as create). **[my call]** Existing sessions of that user are ended. |
| Edit bank | `PUT /api/admin/banks/{id}` (admin only) renames a bank; names stay unique. |
| Banks tab label | `GET /api/config/banks` adds `has_own_stages`. |

Existing endpoints already cover: edit customer, assign case, drop customer, change password, stage requirements, archive restore.

## 7. Constraints

- Stack stays: React + Vite + Tailwind + react-router on the web side; Go + chi + sqlc + Postgres on the server.
- Money stays a decimal string end to end; dates show in India time.
- Every new server rule gets a failing test first (TDD). Web pages keep Vitest + Testing Library tests; existing tests are updated, not deleted, where the behaviour still exists.
- Accessibility: keep labelled controls and `role="alert"` / `role="status"` messages as today.

## 8. Left to the builder

- Exact Tailwind classes and spacing, following the mockups.
- Splitting pages into small shared components (top bar, chip, progress bar, dialog, side panel).
- Query shapes for the new list fields and attention rules.
