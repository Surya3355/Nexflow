# Round 15 backlog: leads, project details, bank processes

Status: **notes from the owner, not yet designed.** Owner will elaborate each task;
we take them one at a time (brainstorm → spec → plan → build). Items marked
*Suggestion* are Claude's proposals and are not decided.

Raised by the owner on 2026-11-22 after testing on a phone.

## Problem found in testing

The owner added a new customer, went back to Home, and could not find them to add
a case.

Cause: Home lists **cases** only. A customer saved without a bank gets no case, so
they appear nowhere on Home (only through search). Task 1 (leads) fixes this.

## The owner's flow, in their words (summarised)

1. Staff add a **lead**: customer name, mobile number, **project name**, and
   **assign it to someone**. Save.
2. Leads show on Home with just their basic details, nothing else.
3. When a lead is added, staff are asked to **contact the customer**.
4. If the customer is interested and comes in, staff add the **bank and loan
   amount** and, because the project is already chosen, the **unit number**
   (flat, villa or plot number). The lead moves to **Received** (or Logged in),
   and from there the normal stages apply.
5. Bank work (HDFC first): more stages, some **strict** stages that need proof,
   **bank queries** raised and assigned to team members and resolved with proof,
   and **bank officer details** kept per stage.

## Decisions so far (2026-11-22)

- Order agreed: 1 → 2 → 4 → 5 → 3, then 6 to 10 as the HDFC work. Keep "needs attention" on Home.
- New customer form: name, mobile, email (optional), project, assigned to (defaults to the person adding). Saving creates a **lead**, shown in the Leads section on Home.
- One project per lead. A customer can have more than one: the lead page offers **New project for this customer**.
- **Contact log** on a lead: response (Interested, Call back later, Not reachable, Not interested), remark, optional next call date. Date, time and person are saved automatically.
- Not interested: **drop** the lead with the existing drop reasons (searchable, can be brought back).
- **Add case** from a lead counts as a successful contact. The lead becomes the case (leaves Leads, continues at Received, keeps its contact log). From the case page staff can add another case for the same customer.
- Owner has more changes for the case page and its customer details section (to come).

## Tasks

### Task 1: Leads (design: `2026-11-23-leads-and-case-page-design.md`). **Built 2026-11-23.**

- New customer form: name, mobile, email (optional), project, assigned to. Save.
- The project moves into the customer/lead form ("move the project under customer
  details").
- Saved leads appear on Home in a **Leads** list with basic details only.
- After saving, staff are prompted to contact the customer.
- Changes round 9 (O2: "no separate lead record").
- *Suggestion:* a lead is a case at a new first stage, **Lead**, before Received.
  Bank and loan amount stay empty until it moves on. One record keeps one history,
  and search, assignment, follow-up alerts and drop already work for cases.
- *Suggestion:* each lead gets a follow-up date (default: today) so the existing
  follow-up alert reminds the assignee to call.
- Open: can a lead be dropped (not interested) with the existing drop reasons?

### Task 2: Lead moves to Received (same design). **Built 2026-11-23.**

- Staff add bank, loan amount and unit number, then move the lead to Received or
  Logged in. Normal stages follow.

### Task 3: Home screen for employee and manager

- Home shows all cases, the leads, and a filter.
- Advanced filters (project, stage and others) move into one **Filter** control
  instead of chips on the page.
- *Suggestion*, also worth showing on Home:
  - follow-ups due today or overdue (what to call today), which already exists as
    "needs attention";
  - leads not contacted yet;
  - later, open bank queries assigned to me (Task 9).

### Task 4: Project details

- When admin adds a project: **type** (Flats, Apartments, Villas or Plots),
  **address**, **contact person name**, **contact number**, and **email**
  (optional).
- Open: are Flats and Apartments different types, or one?
- Note: round O12 put site/plot *loans* in a later phase. A Plots project type is
  only about the builder project. Confirm that this does not bring plot loans into
  scope.

### Task 5: Unit number on the case

- Entered when the lead moves to Received (Task 2). The label follows the project
  type: Flat no., Villa no., Plot no.
- Open: is it required, and what applies to NA projects?

### Task 6: Bank officer list

- A maintained list of bank officers: name and mobile number, per bank.
- Staff pick an officer from a dropdown instead of typing.
- HDFC: different officers at different stages. SBI: one officer for now.

### Task 7: Officer details on stages

- Some stages record which officer is handling the case, so staff can follow up
  directly.

### Task 8: Strict stages with proof

- Some stages are strict: staff must upload proof to move to the next stage or
  mark the item resolved.
- Builds on the existing "document required at a stage" rule (round 11).

### Task 9: Bank queries (HDFC first)

- The bank raises queries on a case. Staff record each query and assign it to a
  team member.
- Queries are resolved with proof, so nothing is missed.
- Owner will discuss this in detail when we do HDFC.

### Task 10: HDFC stage list (design: `2026-11-25-hdfc-stages-design.md`). **Built 2026-11-25.**

- Add HDFC's own stages. Owner will provide them.

## Order

Done: 1, 2 (2026-11-23), 4, 5, 3 (2026-11-24), 10 (2026-11-25).
**Owner, 2026-11-24:** the HDFC work needs the bank's own flow first, so **Task 10 (HDFC stages) comes next**, then
6 → 7 → 8 → 9 on top of it. Owner will provide the HDFC stage list.

Earlier note: to agree with the owner. *Suggestion:* 1 → 2 → 4 → 5 → 3, then 6 → 7 → 8 → 9 → 10
together as the HDFC work.
