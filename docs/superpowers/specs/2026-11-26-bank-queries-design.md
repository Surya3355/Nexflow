# Bank queries, resolved with proof (round 15, tasks 8 and 9)

Status: **approved by owner** (2026-11-26). Backlog: `2026-11-22-round-15-backlog.md`, Tasks 8 and 9.
Mockups: `mockups/round15/queries.html` (desktop and phone, with the dialogs); phone screen also at
https://claude.ai/artifact/VrcQtNSFRjaR2Sq4aKruyR.

## 1. Why

The bank raises queries on a case ("send the latest salary slips", "NOC on letterhead").
Today they live in calls, emails and memory, so one can be forgotten or closed before
the bank is satisfied. The owner wants every query recorded, assigned to someone, and
closed only when proof was given **and** the bank accepted it.

## 2. Owner's answers (2026-11-26)

- Task 8 was meant as this: queries need proof to be resolved. **Strict stages stay as
  they are** (the admin "needs a document" tick, warning only); no stage is ticked today.
  Tasks 8 and 9 are built as one.
- Queries on **every bank**, not only HDFC.
- A query records what the bank asked, when, who it is assigned to, and an optional due
  date. **Not** which bank officer raised it.
- Proof is **a note or a file/photo** (either is enough).
- **An open query blocks the case from moving forward.**
- The assignee gets a **due and an overdue alert**, as with follow-ups.
- Staff can **reopen** a query when the bank rejects the proof, and must **not mark it
  resolved until the bank accepts**.
- The proof is **hidden until clicked** ("View proof"), and there is a **Remove** button.
- Desktop: the queries box sits **inside Progress, in the empty space to the right of the
  stages**, between the stages and the right-hand column (Disbursements, officers, …).
  Phone: Claude's call (§5.5).
- Claude's picks accepted: due date optional; anyone who can edit the case can resolve;
  a proof file also shows in Documents; a proof file counts for the stage's document tick.

## 3. Goals, non-goals, anti-goals

Goals
- Record queries on a case, assign each, see at a glance how many are open.
- Three steps per query: Open → Sent to bank (needs proof) → Resolved (bank accepted);
  Sent to bank → Open again when the bank rejects.
- Block forward stage moves while any query is not resolved.
- Due and overdue alerts to the assignee; the case shows in Needs attention.
- Full history of each query on the case.

Non-goals
- Changing strict stages or the "needs a document" tick.
- Queries on leads (a lead has no bank yet).
- Which bank officer raised the query (owner).
- A list of all queries across cases (a report or Home tab can come later).
- Queries in CSV exports.

Anti-goals
- A query marked resolved without proof, or before the bank accepted.
- Losing the old proof or the rejection reason when a query is reopened.
- A query silently disappearing (removal is audited and kept in history).

## 4. Approaches considered

- **A. Queries as their own record on the case (chosen).** A `case_queries` table with a
  status and an append-only event list (raised, proof sent, rejected, accepted, proof
  removed, removed). Own box on the case page, same pattern as Bank officers.
- B. Queries as a kind of follow-up. Rejected: follow-ups are call reminders with no
  proof or bank acceptance; queries would get lost among them.
- C. Queries as case notes with a tag. Rejected: no status, assignee or blocking.

## 5. Design

### 5.1 Data

- `case_queries`: `id`, `case_id`, `body` (what the bank asked; not blank, ≤ 500),
  `raised_on` (date, not in the future), `assignee_id` (staff user), `due_on` (date,
  optional, not before `raised_on`), `status` (`open`, `sent`, `resolved`),
  `removed_at/by` (null unless removed), `created_at/by`, `updated_at/by`.
- `case_query_events` (append-only): `query_id`, `kind` (`raised`, `sent`, `rejected`,
  `accepted`, `proof_removed`, `reassigned`, `edited`, `removed`), `note` (proof note or
  rejection reason), `document_id` (proof file, optional), `at`, `by`.
- A proof file is stored as a case document (same upload rules: PDF/JPG/PNG, ≤ 10 MB),
  labelled "Query <n> proof" (n = the query's number on the case; a second file for the
  same query gets "Query <n> proof 2"). It shows in the Documents box and follows the
  same one-year purge; the query keeps its note and history after a purge, and shows
  the file's name with "File deleted".
- Archive and restore carry both tables with the case; delete removes them.

### 5.2 Steps and rules

| From | Action | To | Needs |
|---|---|---|---|
| — | Add query | Open | body, raised on, assignee |
| Open | Send proof | Sent to bank | a note or a file (or both) |
| Sent to bank | Bank accepted | Resolved | — |
| Sent to bank | Bank rejected | Open | the bank's reason |
| Sent to bank | Remove proof | Open | — (the proof file is deleted from Documents) |
| Open or Sent | Remove query | removed | a confirm; kept in history |

- Resolved is final (a new query is added if the bank asks again).
- A resolved query cannot be removed, and its proof cannot be removed.
- Edit (body, due date) and reassign are allowed while Open or Sent; both audited.
- Anyone who can edit the case can do every action. Dropped cases: read-only.
- Every action is audited and shows in the case's full history, e.g.
  "Query sent to bank: Builder NOC copy".

### 5.3 Blocking the stage move

- While any query is Open or Sent, the case **cannot move to a later stage**. The server
  refuses (409) with "Resolve the open bank queries first"; the page disables
  "Move to …" and shows the reason.
- Moving to an earlier stage and dropping the case are still allowed.
- Adding a case's first stage is unaffected (a new case has no queries).

### 5.4 Alerts and Needs attention

- A case with any Open or Sent query appears in **Needs attention**.
- On the due date, the assignee gets a due alert; from the day after, one overdue alert,
  for a query still Open (also when it reopens after its due date). A query already Sent to bank gets no alert (waiting on the bank).
  Same delivery and once-only rule as follow-up alerts. No due date: no alert.

### 5.5 Case page

- **Bank queries** box with a count ("2 open", red border when any is open) and
  **+ Add query**. Each query: what was asked, a status chip (Open amber, Sent to bank
  blue), raised date, assignee, due date ("overdue" in red).
- Buttons per step: Open → **Send proof**, **Remove**. Sent → **View proof**,
  **Bank accepted**, **Bank rejected**.
- **The proof is hidden** until **View proof** is clicked; it then shows the note, the
  file link and **Remove proof**.
- Resolved queries collapse under "Resolved (n)".
- Desktop: the Progress box is split in two: stages on the left, **Bank queries on the
  right** (the space that is empty today). The right-hand column (Disbursements, Bank
  officers, Documents, Details) is unchanged.
- Phone (Claude's call): the box sits **above the stages**, right under the case header,
  because an open query is what stops the case moving; the blocked Move button below
  says "Resolve the 2 bank queries above first" (desktop: "Resolve the 2 open bank
  queries first").

## 6. Left to the builder

Exact queries and handlers; whether queries come in the case response or a separate
call; component split; how the Needs attention query joins in; numbering of queries per
case (any stable order, e.g. by id within the case).

## 7. Open for the owner

None. Owner confirmed (2026-11-26): **Remove** takes away a query added by mistake (kept
in history) and **Remove proof** takes away wrong proof (query back to Open). Placement
confirmed from the mockups.
