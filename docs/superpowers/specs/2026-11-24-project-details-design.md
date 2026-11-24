# Project details and builder contacts (round 15, task 4)

Status: **draft for owner review** (2026-11-24). Backlog: `2026-11-22-round-15-backlog.md`, Task 4.

## 1. Why

Staff sometimes need to call the builder's office about a case. A builder office has
different people and numbers (sales, site, accounts), so each case must say which
contact to call. Today a project is only a name.

The project type also drives the unit label in Task 5 (Flat no., Villa no., Plot no.).

## 2. Goals, non-goals, anti-goals

Goals
- Admin records a project's **type**, **address** and a **list of contacts**.
- Every project except NA has **at least one** active contact.
- Each lead and each case points to **one contact** from its project's list. Staff see
  it with a tap-to-call number and can change it.

Non-goals
- Plot *loans*. A **Plots** project type describes the builder project only; plot
  loans stay in a later phase (O12).
- A builder record or builder login (later).
- Admin-editable project types. The four types are fixed; new ones are added in code
  when the owner asks.
- Contacts in reports or the CSV export.

Anti-goals
- Contacts shared between projects. Owner: each project's list is **independent**.
  The same person on two projects is two separate entries that never affect each
  other.
- Losing which contact an old case used when a contact is removed.

## 3. Approaches considered

- **A. Per-project contact table; leads and cases point to a contact (chosen).**
  Fixing a number fixes it on every case of that project. "Remove from project" can
  hide a contact while old cases keep it. Most new parts.
- B. Contacts stored as one text/JSON block on the project. Cheap, but a case cannot
  point to a contact reliably, and hide-but-keep does not work. Rejected.
- C. Copy name and number onto each case when picked. Simple, but a changed number
  stays wrong on every old case. Rejected.
- A shared contact list that projects pick from was proposed by Claude and
  **rejected by the owner** (lists must be independent per project).

## 4. Design

### 4.1 Data (migration 000006)

`projects` gains:
- `type text NULL CHECK (type IN ('high_rise','apartments','plots','villas'))`.
  Labels: **High-rise apartments**, **Apartments**, **Plots**, **Villas** (owner's
  order). NULL only for NA and for projects created before this change.
- `address text NOT NULL DEFAULT ''`.

New `project_contacts`:
- `id`, `project_id` (FK projects, NOT NULL), `name` (required, trimmed non-empty),
  `phone` (required, same mobile rules as customers), `email` (optional, same rules as
  customers), `label` (optional, e.g. "Sales office"), `removed_at timestamptz NULL`,
  `created_at/by`, `updated_at/by`.
- Unique `(project_id, id)` so leads and cases can use a composite foreign key.
- Partial unique index on `(project_id, phone) WHERE removed_at IS NULL`: the same
  number cannot be added twice to one project. The same number on another project is
  allowed.
- Contacts are never hard-deleted (old cases and archived copies point to them).
- NA cannot have contacts (checked in the service).

`leads` and `cases` gain `builder_contact_id bigint NULL`, with composite FK
`(project_id, builder_contact_id) → project_contacts(project_id, id)`. The database
therefore rejects a contact from another project. NULL means none (NA, or an old
project with no contacts yet).

### 4.2 Rules

- **Create project:** name, type, address and at least one contact, saved together in
  one transaction. Type and address are required for non-NA projects.
- **Edit project:** name, type and address can be changed; contacts can be added,
  edited (name, phone, email, label) or removed. NA stays locked except that it has no
  type, address or contacts.
- **Every save of a non-NA project must be complete** (owner, 2026-11-24): type,
  address and at least one active contact. This applies to old projects too: an admin
  who only wants to rename one must also fill in the missing details. The same
  validation runs on create and edit.
- **One save for the whole form** (*Claude's call*): the project form sends the
  project and its full contact list together, applied in one transaction. Cancel
  discards every change, including removals. The "at least one active contact" check
  runs on the final list, so "remove both old contacts and add a new one" in one save
  is allowed. Contacts are matched by id; a contact missing from the list is marked
  removed; new rows (no id) are inserted.
- **Remove contact:** sets `removed_at` (on Save, see above). Refused if the save
  would leave no active contact. Leads and cases that point to it keep it; their pages show it marked
  "removed". It no longer appears in pickers for that project.
- **Old projects** (no type or contacts yet): still usable for leads and cases, so
  nothing breaks. Admin sees "Add type and contact" on them, and the first edit must
  complete them. Leads and cases on them have no contact until
  one exists. Once a contact exists, existing leads and cases are not filled in
  automatically; staff pick one when needed. *Claude's call.*
- **Who:** only admin creates or edits projects and contacts. All staff choose which
  contact a lead or case uses.

### 4.3 Choosing the contact on leads and cases

- **New lead:** the contact is set to the project's first active contact (oldest
  first). No extra field in the New customer form. *Claude's call*, to keep that form
  short.
- **Lead page:** shows the contact (label, name, tap-to-call number, email if any)
  with **change**, which offers the project's active contacts.
- **Add case:** a "Builder contact" select, prefilled with the lead's contact (or the
  project's first active contact when there is no lead, e.g. "+ New case" from a case
  page). Required when the project has active contacts; hidden for NA.
- **Case page:** in Details, under Project: the contact with tap-to-call and
  **change**. A removed contact shows "(removed)" and staff can pick another.
- **Dropped cases and leads:** the contact cannot be changed, matching how
  `ChangeProject` refuses dropped cases today. *Claude's call.*
- If the lead's contact was removed meanwhile, Add case prefills the project's first
  active contact instead. *Claude's call.*
- **Change a case's project:** the contact switches to the new project's first active
  contact (or none). The history entry for the project change records it.
- Changing the contact on a lead or case writes a history entry (who, when, from, to).

### 4.4 Admin screens

Admin › Banks & lists › Projects: the list shows name, type and the number of
contacts; old projects show "Add type and contact". **+ Add project** and **Edit**
open one form:

```
Name      [ Green Meadows        ]
Type      ( High-rise apartments | Apartments | Plots | Villas )
Address   [ Survey 12, Kollur     ]
Contacts  Sales office · Ravi K · 98xxxxxxxx    [edit] [remove]
          Site office · Suresh · 90xxxxxxxx      [edit] [remove]
          [+ Add contact]
```

The project page (`/projects/:id`) shows type, address and the active contacts
(tap-to-call) above its cases list.

### 4.5 Search, drop, delete, archive

- Dropping or bringing back a lead or case keeps its contact.
- The archive copy of a deleted lead or case includes `builder_contact_id`; restore puts
  it back (contacts are never deleted). If that contact was removed meanwhile, it
  shows as removed.
- Search is unchanged.

## 5. Security, permissions, data integrity

- Project and contact writes are admin-only on the server, not only hidden in the UI.
- Composite FK: a lead or case can never point to another project's contact.
- "At least one active contact" and "no contacts on NA" are enforced in the service
  inside the transaction, with the project row locked so two admins cannot remove the
  last two contacts at the same time.
- Phone and email go through the same validation as customer fields.

## 6. Testing

- Service (red/green): create and edit need type, address and one contact (an old
  incomplete project cannot be saved incomplete); one save replaces the contact list
  in one transaction; NA refuses
  contacts; duplicate phone in one project refused, allowed across projects; removing
  the last active contact refused; concurrent removals keep one; contact from another
  project rejected by the database; project change resets the contact; lead to case
  carries the contact; archive and restore keep it.
- HTTP: admin-only routes refuse manager and employee.
- Web: project form (add, edit, remove contact), contact picker on lead, Add case and
  case page, "(removed)" display, NA hides the picker.

## 7. Left to the builder

Endpoint shapes, component split, and query names, following existing patterns
(`workflows` module owns projects).

## 8. Claude's calls for the owner to check

1. The four types are fixed in code (owner: "we can add later").
2. A new lead gets the project's first contact automatically; the New customer form
   does not ask.
3. Old projects keep working without a type or contact until admin adds them.
4. The same number cannot be on one project's list twice.
5. Contacts are not in reports or the CSV.
6. The project form saves everything at once; Cancel undoes all changes.
7. A dropped lead or case's contact cannot be changed.
8. If a lead's contact was removed, Add case picks the project's first active contact.
