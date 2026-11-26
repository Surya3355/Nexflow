# Nexflow — Database Schema (draft for review)

Status: **Draft.** Not approved. Based on `docs/superpowers/specs/2026-11-11-nexflow-v1-design.md` and `docs/tech-stack.md` (PostgreSQL). The schema becomes migrations only after the owner approves it.

Conventions (my call, apply to all tables):
- `id` is `bigint generated always as identity`.
- Times are `timestamptz`. Dates for business days are `date`.
- Money is `numeric(14,2)` (INR). Interest rate is `numeric(5,2)`.
- Enumerated values use `text` with a `CHECK` constraint, so they are easy to read and change through migrations.
- Every business table has `created_at`, `created_by`, `updated_at`, `updated_by`.
- Business records are never physically deleted except by the hard-delete path, which moves a full copy into `archive` first (spec §4.5).
- Stage and assignment history tables are **append-only**. The application does not update or delete their rows.

Items marked **[open]** need an owner answer or a review decision.

---

## 1. Access

### users
| Column | Type | Rules |
|---|---|---|
| username | text | unique, lowercase, not null |
| full_name | text | not null |
| password_hash | text | not null. `argon2id` encoded hash |
| role | text | `admin`, `manager`, `employee` |
| is_active | boolean | default true. Inactive users cannot log in |
| password_changed_at | timestamptz | null until the user first changes the admin-set password **[my call]** |

Users are created only by admin (round 10). Users change their own password from the app.

### sessions
Used by the session library (`alexedwards/scs`, PostgreSQL store).
| Column | Type | Rules |
|---|---|---|
| token | text | primary key |
| data | bytea | not null |
| expiry | timestamptz | not null, indexed |

---

## 2. Configuration (maintained by admin)

### banks
| Column | Type | Rules |
|---|---|---|
| name | text | unique, not null (HDFC, SBI, and others) |
| is_active | boolean | default true |
| part_payments | boolean | default false. The bank pays some loans in parts (HDFC, later private banks; not SBI). Migration 000007 |

### stages
Standard stages are rows with `bank_id = null`. A bank with its own stages has rows with its `bank_id`. A bank with none uses the standard rows (spec §5, round 7).
| Column | Type | Rules |
|---|---|---|
| bank_id | bigint | nullable, FK `banks.id` |
| code | text | not null, for example `received`, `logged_in` |
| name | text | not null |
| sort_order | integer | not null |
| is_active | boolean | default true |
| requires_document | boolean | default false. Admin marks a stage that needs documents uploaded (round 11). Rule for "satisfied" is **[open]** |
| role | text | nullable: `start`, `tracking`, `sanction`, `disbursement`. What the stage does; rules and reports use it, not the code. At most one per role per list (`stages_role_uidx`). Migration 000007 |
| officer_team | text | nullable: `tdep`, `dovr`, `appraisal`, `fixing`, `officer`. The bank officer team staff pick at this stage. At most one per team per list (`stages_officer_team_uidx`). Migration 000008 |

Constraints: unique `(bank_id, sort_order)` among active rows, and unique `(bank_id, code)`. Because `bank_id` can be null, use `NULLS NOT DISTINCT` (PostgreSQL 15+) or a partial unique index for the standard set. **[open]** PostgreSQL version to confirm.

Standard rows (round 1): Received, Logged in, Under sanction, Sanctioned, Documentation, Disbursed (`disbursed` is the final stage). Roles: Received start, Logged in tracking, Sanctioned sanction, Disbursed disbursement.

HDFC list (round 15 task 10, admin applies it to a bank): Received, Link sent (start), Application submitted (tracking), DOVR, Appraisal, Sanctioned (sanction), Upload documents, Under fixing, Fixed, Signing, Disbursement (disbursement).

### dropdown_values
Admin-maintained lists, starting with the dropped reasons (round 8).
| Column | Type | Rules |
|---|---|---|
| category | text | `drop_reason` for now. Other categories can be added without a schema change |
| value | text | not null |
| sort_order | integer | not null |
| is_active | boolean | default true |

Unique `(category, value)`. Starter `drop_reason` values: Customer not interested; Not eligible; Rejected by bank; Customer chose another bank or DSA; Customer unreachable; Duplicate or already processed; Other.

### projects (round 14, details round 15 migration 000006)
Builder projects. NA (`is_system`) is the fixed entry for loans without a builder project. Spec `docs/superpowers/specs/2026-11-24-project-details-design.md`.
| Column | Type | Rules |
|---|---|---|
| name | text | not null, unique ignoring case and spaces |
| is_system | boolean | true only for NA |
| type | text | nullable. `high_rise`, `apartments`, `plots`, `villas`. NULL for NA and for projects made before round 15 |
| address | text | not null, default `''` |
| created_at/by, updated_at/by | | Usual |

NA has no type and no address (CHECK). The app requires type, address and at least one active contact on every save of a non-NA project.

### project_contacts (round 15, migration 000006)
Each project's own builder contacts. Lists of different projects are independent. Rows are never deleted.
| Column | Type | Rules |
|---|---|---|
| project_id | bigint | not null, FK `projects.id` |
| name | text | not null, not blank |
| phone | text | not null, 10 digits |
| email | text | nullable |
| label | text | nullable, ≤ 40 characters (e.g. "Sales office") |
| removed_at | timestamptz | nullable. Set when admin removes the contact from the project |
| created_at/by, updated_at/by | | Usual |

Unique `(project_id, id)` (target of the lead and case FKs). Unique `(project_id, phone)` among contacts not removed.

---

## 3. Customers and cases

### customers
| Column | Type | Rules |
|---|---|---|
| name | text | not null |
| mobile | text | not null, unique among customers. Exactly 10 digits, no +91 or spaces (round 12). Archived customers do not count |
| email | text | nullable |
| status | text | `active`, `dropped` |
| dropped_reason_id | bigint | nullable, FK `dropdown_values.id`. Required when status = `dropped` (check) |
| dropped_remark | text | nullable |
| dropped_at | timestamptz | nullable |
| dropped_by | bigint | nullable, FK `users.id` |

A customer that still has cases cannot be hard-deleted (round 1, §10). Enforced by `cases.customer_id` FK with `ON DELETE RESTRICT`.

### cases
| Column | Type | Rules |
|---|---|---|
| customer_id | bigint | not null, FK `customers.id`, `ON DELETE RESTRICT` |
| bank_id | bigint | not null, FK `banks.id` |
| loan_amount | numeric(14,2) | not null, > 0 |
| tracking_number | text | nullable. Set at "Logged in". Unique when not null (partial unique index) |
| current_stage_id | bigint | not null, FK `stages.id`. New case starts at Received |
| assignee_id | bigint | not null, FK `users.id` |
| status | text | `active`, `dropped`, `completed`. `completed` when stage is Disbursed |
| follow_up_date | date | nullable. Drives the due/overdue alert (round 6). One date per case **[my call]** |
| dropped_reason_id | bigint | nullable, FK `dropdown_values.id`. Required when status = `dropped` |
| dropped_remark | text | nullable |
| dropped_at | timestamptz | nullable |
| dropped_by | bigint | nullable, FK `users.id` |
| dropped_via_customer | boolean | not null, default false. True when the case was dropped because its customer was dropped (round 9) |
| sanctioned_amount | numeric(14,2) | nullable. Set at Sanctioned |
| interest_rate | numeric(5,2) | nullable |
| interest_type | text | `fixed`, `floating`, nullable |
| tenure_months | integer | nullable, > 0 |
| sanction_date | date | nullable |
| (disbursed_amount, disbursement_date) | — | **dropped in migration 000007**; payments are in `case_disbursements`. Sanction now also overwrites `loan_amount` with the sanctioned amount |
| created_at | timestamptz | not null. Used for "cases created today" (round 9) |
| project_id | bigint | not null, FK `projects.id`. NA for loans without a builder project (round 14) |
| lead_id | bigint | nullable, unique, FK `leads.id` `ON DELETE SET NULL`. The lead this case came from (round 15) |
| unit_number | text | nullable, ≤ 20 characters. Flat, villa or plot number, saved in capitals (round 15) |
| builder_contact_id | bigint | nullable. FK `(project_id, builder_contact_id)` → `project_contacts (project_id, id)`, so it is always a contact of the case's project (round 15) |

Indexes: `customer_id`, `bank_id`, `current_stage_id`, `assignee_id`, `created_at`, `follow_up_date`.

### leads (round 15, migration 000005)
An enquiry for one builder project, before the customer comes in. Spec `docs/superpowers/specs/2026-11-23-leads-and-case-page-design.md`.
| Column | Type | Rules |
|---|---|---|
| customer_id | bigint | not null, FK `customers.id`, `ON DELETE RESTRICT` |
| project_id | bigint | not null, FK `projects.id` (NA allowed) |
| assignee_id | bigint | not null, FK `users.id` |
| status | text | `open`, `converted`, `dropped` |
| next_call_date | date | nullable. IST day; drives call due/overdue emails. Set to the day the lead is added; each contact replaces it |
| dropped_reason_id, dropped_remark, dropped_at, dropped_by | | As on cases. Reason required when dropped |
| converted_at | timestamptz | nullable. Set when Add case turns the lead into a case |
| builder_contact_id | bigint | nullable. Same composite FK as on cases (round 15) |
| created_at/by, updated_at/by | | Usual |

Indexes: `customer_id`; `next_call_date` where open; unique `(customer_id, project_id)` where open (one open lead per project).

### lead_contacts (round 15)
Contact log. The app only inserts; the archive removes rows with their customer.
| Column | Type | Rules |
|---|---|---|
| lead_id | bigint | not null, FK `leads.id` |
| response | text | `interested`, `call_back`, `not_reachable`, `not_interested` |
| remark | text | not null, not blank |
| next_call_date | date | nullable |
| created_at, created_by | | When and who |

Milestone dates for reports come from `case_stage_events` (round 2), not from the columns above. The sanction and disbursement columns are the values captured at those stages.

---

## 4. History (append-only)

### case_stage_events
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| stage_id | bigint | not null, FK `stages.id` |
| reached_at | timestamptz | not null |
| changed_by | bigint | not null, FK `users.id` |
| remark | text | nullable, max 500. Optional note on the move (migration 000007) |

### case_disbursements (round 15 task 10, migration 000007)
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| amount | numeric(14,2) | not null, > 0 |
| paid_on | date | not null (IST date) |
| kind | text | `part` or `full`; at most one `full` per case (`case_disbursements_full_uidx`) |
| created_at/by, updated_at/by | | audit columns |

A Full payment completes the case while it is at its disbursement stage. Remaining = sanctioned amount (else loan amount) − total paid, never below 0. Archived and restored with the case.

Every stage change adds a row, including backward moves (round 8). The current stage is `cases.current_stage_id`. Reports count a case in a period if it has an event in that period (round 2).

### bank_officers (round 15 tasks 6 and 7, migration 000008)
| Column | Type | Rules |
|---|---|---|
| bank_id | bigint | not null, FK `banks.id` |
| team | text | not null, one of the officer team codes; must be a team of the bank's stages (checked by the service) |
| name | text | not null, not blank, max 80 |
| phone | text | nullable, 10 digits |
| email | text | nullable, max 254, one @. At least one of phone and email (`bank_officers_contact_check`) |
| removed_at | timestamptz | nullable. Rows are never deleted; removed officers stay on cases |
| created_at/by, updated_at/by | | audit columns |

Among current officers, one bank and team cannot list the same mobile (`bank_officers_active_phone_uidx`) or the same email, ignoring case (`bank_officers_active_email_uidx`), twice; the same mobile or email in another team is allowed.

### case_officers (round 15 tasks 6 and 7, migration 000008)
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| team | text | not null, officer team code |
| officer_id | bigint | not null, FK `bank_officers.id`; same bank and team as the case (checked by the service) |
| picked_at | timestamptz | not null |
| picked_by | bigint | not null, FK `users.id` |

Append-only: every pick adds a row. The latest row per case and team is the current officer; all rows are the history shown on the case page. Archived and restored with the case.

### case_queries (round 15 tasks 8 and 9, migration 000009)
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| number | integer | not null, > 0; unique per case (`Query <n>`) |
| body | text | not null, not blank, ≤ 500 characters: what the bank asked |
| raised_on | date | not null, not in the future (checked by the service) |
| due_on | date | nullable, not before `raised_on` |
| assignee_id | bigint | not null, FK `users.id` |
| status | text | `open`, `sent` (proof sent, waiting for the bank), `resolved` (bank accepted) |
| removed_at / removed_by | timestamptz / bigint | set when a query added by mistake is removed |
| created_at/by, updated_at/by | | |

A case cannot move to a later stage while it has a query that is not resolved and not removed. Archived and restored with the case.

### case_query_events (migration 000009)
| Column | Type | Rules |
|---|---|---|
| query_id | bigint | not null, FK `case_queries.id` |
| kind | text | `raised`, `edited`, `sent`, `accepted`, `rejected`, `proof_removed`, `removed` |
| note | text | nullable, ≤ 500: the proof note, or the bank's reason on `rejected` |
| document_id | bigint | nullable, FK `documents.id` **ON DELETE SET NULL**: the proof file (a case document labelled `Query <n> proof`) |
| file_name | text | nullable, ≤ 255 (migration 000010): the proof file's name, kept so the page can say "File deleted" after the document is purged or removed |
| at | timestamptz | not null |
| by_user | bigint | not null, FK `users.id` |

Append-only history of each query. Archived and restored with the case (after documents).

### case_assignment_events
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| from_user_id | bigint | nullable (null on first assignment), FK `users.id` |
| to_user_id | bigint | not null, FK `users.id` |
| changed_by | bigint | not null, FK `users.id` |
| changed_at | timestamptz | not null |

---

## 5. Documents

### documents
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | not null, FK `cases.id` |
| stage_id | bigint | not null, FK `stages.id`. The case's stage when the document was uploaded (round 11). Not a gate |
| label | text | not null. Free text naming the document **[open]** see review point 3 |
| file_name | text | not null |
| content_type | text | `application/pdf`, `image/jpeg`, `image/png` (check) |
| size_bytes | bigint | not null, `<= 10485760` (10 MB, round 5) |
| storage_key | text | not null. Private location, never a public URL |
| purge_after | date | not null. Set to the case's disbursed or dropped date plus 1 year (round 7, automatic) |

Unique `(case_id, label)`. A new upload with the same label replaces the file and removes the old one from storage (round 7, latest version only).

---

## 6. Archive and audit

### archive
Holds hard-deleted customers and cases, with their cases for a customer (round 9).
| Column | Type | Rules |
|---|---|---|
| entity_type | text | `customer` or `case` |
| entity_id | bigint | original id |
| snapshot | jsonb | full copy of the record. For a customer, includes its cases and their stage and assignment history |
| archived_at | timestamptz | not null |
| archived_by | bigint | not null, FK `users.id` |
| purge_after | timestamptz | not null. `archived_at` plus 1 year (round 3) |
| restored_at | timestamptz | nullable |
| restored_by | bigint | nullable, FK `users.id` |

Index: `purge_after`. A scheduled job deletes rows where `purge_after < now()`.

### audit_log
Permanent (round 7). Never purged.
| Column | Type | Rules |
|---|---|---|
| occurred_at | timestamptz | not null |
| actor_user_id | bigint | nullable (system jobs), FK `users.id` |
| action | text | for example `customer.created`, `case.assigned`, `case.stage_changed`, `case.dropped`, `customer.deleted`, `archive.restored`, `user.password_changed` |
| entity_type | text | not null |
| entity_id | bigint | nullable |
| details | jsonb | what changed, small and readable |

Index: `(entity_type, entity_id)`, `occurred_at`. Application database role has insert and select only on this table, so history cannot be edited.

---

## 7. Alerts

### alert_log
Prevents the same email from being sent twice.
| Column | Type | Rules |
|---|---|---|
| case_id | bigint | nullable, FK `cases.id` |
| lead_id | bigint | nullable, FK `leads.id` (round 15). Exactly one of `case_id`, `lead_id` is set |
| recipient_user_id | bigint | not null, FK `users.id` |
| kind | text | `assigned`, `follow_up_due`, `follow_up_overdue`, `query_due`, `query_overdue` (bank queries, to the query's assignee) |
| due_on | date | nullable. The follow-up date the alert is for |
| sent_at | timestamptz | not null |

Unique `(case_id, kind, due_on, recipient_user_id)` where `case_id` is set, and `(lead_id, kind, due_on, recipient_user_id)` where `lead_id` is set (round 15).

---

## 8. Draft SQL (outline, not for running yet)

```sql
CREATE TABLE users (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  username            text NOT NULL UNIQUE CHECK (username = lower(username)),
  full_name           text NOT NULL,
  password_hash       text NOT NULL,
  role                text NOT NULL CHECK (role IN ('admin','manager','employee')),
  is_active           boolean NOT NULL DEFAULT true,
  password_changed_at timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  created_by          bigint REFERENCES users(id),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  updated_by          bigint REFERENCES users(id)
);

CREATE TABLE sessions (
  token  text PRIMARY KEY,
  data   bytea NOT NULL,
  expiry timestamptz NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions (expiry);

CREATE TABLE banks (
  id        bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name      text NOT NULL UNIQUE,
  is_active boolean NOT NULL DEFAULT true
  -- audit columns omitted here for brevity; same pattern as users
);

CREATE TABLE dropdown_values (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  category   text NOT NULL,
  value      text NOT NULL,
  sort_order integer NOT NULL,
  is_active  boolean NOT NULL DEFAULT true,
  UNIQUE (category, value)
);

CREATE TABLE stages (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  bank_id    bigint REFERENCES banks(id),          -- NULL = standard stage
  code       text NOT NULL,
  name       text NOT NULL,
  sort_order integer NOT NULL,
  is_active  boolean NOT NULL DEFAULT true,
  requires_document boolean NOT NULL DEFAULT false,
  UNIQUE NULLS NOT DISTINCT (bank_id, code),       -- PostgreSQL 15+ [open]
  UNIQUE NULLS NOT DISTINCT (bank_id, sort_order)
);

CREATE TABLE customers (
  id               bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name             text NOT NULL,
  mobile           text NOT NULL CHECK (mobile ~ '^[0-9]{10}$'),
  email            text,
  status           text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dropped')),
  dropped_reason_id bigint REFERENCES dropdown_values(id),
  dropped_remark   text,
  dropped_at       timestamptz,
  dropped_by       bigint REFERENCES users(id),
  created_at       timestamptz NOT NULL DEFAULT now(),
  created_by       bigint REFERENCES users(id),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  updated_by       bigint REFERENCES users(id),
  CHECK (status <> 'dropped' OR (dropped_reason_id IS NOT NULL AND dropped_at IS NOT NULL))
);
CREATE UNIQUE INDEX customers_mobile_live_uidx ON customers (mobile);

CREATE TABLE cases (
  id                  bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id         bigint NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  bank_id             bigint NOT NULL REFERENCES banks(id),
  loan_amount         numeric(14,2) NOT NULL CHECK (loan_amount > 0),
  tracking_number     text,
  current_stage_id    bigint NOT NULL REFERENCES stages(id),
  assignee_id         bigint NOT NULL REFERENCES users(id),
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dropped','completed')),
  follow_up_date      date,
  dropped_reason_id   bigint REFERENCES dropdown_values(id),
  dropped_remark      text,
  dropped_at          timestamptz,
  dropped_by          bigint REFERENCES users(id),
  dropped_via_customer boolean NOT NULL DEFAULT false,
  sanctioned_amount   numeric(14,2),
  interest_rate       numeric(5,2),
  interest_type       text CHECK (interest_type IN ('fixed','floating')),
  tenure_months       integer CHECK (tenure_months > 0),
  sanction_date       date,
  disbursed_amount    numeric(14,2),
  disbursement_date   date,
  created_at          timestamptz NOT NULL DEFAULT now(),
  created_by          bigint REFERENCES users(id),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  updated_by          bigint REFERENCES users(id),
  CHECK (status <> 'dropped' OR (dropped_reason_id IS NOT NULL AND dropped_at IS NOT NULL))
);
CREATE UNIQUE INDEX cases_tracking_uidx ON cases (tracking_number) WHERE tracking_number IS NOT NULL;
CREATE INDEX cases_customer_idx ON cases (customer_id);
CREATE INDEX cases_bank_idx ON cases (bank_id);
CREATE INDEX cases_stage_idx ON cases (current_stage_id);
CREATE INDEX cases_assignee_idx ON cases (assignee_id);
CREATE INDEX cases_created_idx ON cases (created_at);
CREATE INDEX cases_follow_up_idx ON cases (follow_up_date);

CREATE TABLE case_stage_events (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id     bigint NOT NULL REFERENCES cases(id),
  stage_id    bigint NOT NULL REFERENCES stages(id),
  reached_at  timestamptz NOT NULL,
  changed_by  bigint NOT NULL REFERENCES users(id)
);
CREATE INDEX case_stage_events_case_idx ON case_stage_events (case_id, reached_at);
CREATE INDEX case_stage_events_reached_idx ON case_stage_events (reached_at);

CREATE TABLE case_assignment_events (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id      bigint NOT NULL REFERENCES cases(id),
  from_user_id bigint REFERENCES users(id),
  to_user_id   bigint NOT NULL REFERENCES users(id),
  changed_by   bigint NOT NULL REFERENCES users(id),
  changed_at   timestamptz NOT NULL
);

CREATE TABLE documents (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id      bigint NOT NULL REFERENCES cases(id),
  stage_id     bigint NOT NULL REFERENCES stages(id),
  label        text NOT NULL,
  file_name    text NOT NULL,
  content_type text NOT NULL CHECK (content_type IN ('application/pdf','image/jpeg','image/png')),
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 10485760),
  storage_key  text NOT NULL,
  purge_after  date,
  uploaded_at  timestamptz NOT NULL DEFAULT now(),
  uploaded_by  bigint NOT NULL REFERENCES users(id),
  UNIQUE (case_id, label)
);

CREATE TABLE archive (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  entity_type text NOT NULL CHECK (entity_type IN ('customer','case')),
  entity_id   bigint NOT NULL,
  snapshot    jsonb NOT NULL,
  archived_at timestamptz NOT NULL DEFAULT now(),
  archived_by bigint NOT NULL REFERENCES users(id),
  purge_after timestamptz NOT NULL,
  restored_at timestamptz,
  restored_by bigint REFERENCES users(id)
);
CREATE INDEX archive_purge_idx ON archive (purge_after);

CREATE TABLE audit_log (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  occurred_at   timestamptz NOT NULL DEFAULT now(),
  actor_user_id bigint REFERENCES users(id),
  action        text NOT NULL,
  entity_type   text NOT NULL,
  entity_id     bigint,
  details       jsonb
);
CREATE INDEX audit_entity_idx ON audit_log (entity_type, entity_id);
CREATE INDEX audit_time_idx ON audit_log (occurred_at);

CREATE TABLE alert_log (
  id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id            bigint NOT NULL REFERENCES cases(id),
  recipient_user_id  bigint NOT NULL REFERENCES users(id),
  kind               text NOT NULL CHECK (kind IN ('assigned','follow_up_due','follow_up_overdue')),
  due_on             date,
  sent_at            timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (case_id, kind, due_on, recipient_user_id)
);
```

---

## 9. Points to review

1. **Mobile number format.** Store digits only, or keep as typed? Digits only, exactly 10. The form shows a fixed +91 prefix, and pasted +91 and spaces are stripped (round 12). **[resolved]**
2. **Archived mobile numbers.** After a customer is archived, can the same mobile be used for a new customer? Recommended: yes, while the archived copy remains in the archive. Restoring it then needs a clash check. **[resolved, round 12: yes]**
3. **Document label.** Free text, or a fixed list per bank? Free text. Checklists dropped (round 11). **[resolved, round 12]**
4. **Archive as JSON snapshot.** The archive stores the full record as `jsonb`. This keeps the archive simple and avoids copies of every table. Restore re-inserts rows from the snapshot. Alternative: relational archive tables. **[my call]**
5. **Stage events for backward moves.** Every move adds a row, so the history shows the true sequence (round 8). **[my call]** confirmed by design.
6. **Tracking number uniqueness.** Enforced among live cases. An archived case with the same number would not block a new case. Same question as point 2. **[resolved, round 12: live cases only]**
7. **PostgreSQL version.** `NULLS NOT DISTINCT` needs PostgreSQL 15 or newer. Confirm the version when hosting is chosen. **[open]**
8. **Follow-up date.** One date per case. If a case needs several follow-ups, the schema would need a separate table. **[my call]**
9. **Denormalized current stage and status.** `cases.current_stage_id` and `cases.status` are kept alongside the history for fast lists. The application updates them in the same transaction as the history row. **[my call]**

10. **Stage document requirement.** `stages.requires_document` lets admin mark a stage as needing uploads (round 11). What counts as satisfied, and whether a missing upload blocks the stage move, is resolved: a stage needs at least one file on the case, and a missing file only warns (round 12). **[resolved]**

## 10. Next

After the owner reviews this draft and answers points 1 to 3 and 6, I will update the schema, then write the migration files and the implementation plan.
