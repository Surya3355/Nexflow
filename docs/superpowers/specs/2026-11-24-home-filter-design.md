# Home: list tabs and one Filter (round 15, task 3)

Status: **approved by owner 2026-11-24.** Backlog: `2026-11-22-round-15-backlog.md`, Task 3. Small change, web only.

## Why
Home has eight controls in a row (four list chips, then Bank, Project, Stage, Assigned, Date); on a phone they wrap
onto three or four lines above the list. Owner: the first line chooses the list, the second is a real filter, so it
should be one Filter control.

## Decisions (owner)
- Four tabs with counts: **Leads · My cases · All · Needs attention**, then a **Filter** button with the number of
  filters on ("Filter · 2"). On a narrow phone the tabs scroll sideways; Filter stays visible. Defaults unchanged
  (employee → My cases, manager/admin → All).
- Filter opens a panel: Bank, Project, Stage (after a bank), Assigned to, Date from/to; **Clear all** and **Apply**.
  Nothing changes until Apply. Filters stay when switching tabs.
- My cases and All use every filter. **Leads** uses Project and Assigned to only (the panel shows just those).
  **Needs attention** is not filtered; Filter is disabled there.
- Leads with no contact logged keep the existing "Not contacted" badge and sort first among leads whose call is not due.

## Approaches
- **Leads filtered in the page (chosen):** Home already loads every open lead with its project, assignee and last
  contact. No server change.
- Server-side lead filters: only worth it with thousands of leads; rejected for now.

## Claude's calls
- On **My cases** the panel hides Assigned to (it is always you).
- Tab counts follow the filters, except Needs attention.
