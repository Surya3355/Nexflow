-- Nexflow v1 initial schema. Source: docs/database-schema.md §8, with the
-- round 11 and 12 changes (stages.requires_document, documents.stage_id,
-- mobile format check). Times are timestamptz. Money is numeric(14,2).

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
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       text NOT NULL UNIQUE,
  is_active  boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id)
);

CREATE TABLE dropdown_values (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  category   text NOT NULL,
  value      text NOT NULL,
  sort_order integer NOT NULL,
  is_active  boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id),
  UNIQUE (category, value)
);

CREATE TABLE stages (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  bank_id           bigint REFERENCES banks(id),          -- NULL = standard stage
  code              text NOT NULL,
  name              text NOT NULL,
  sort_order        integer NOT NULL,
  is_active         boolean NOT NULL DEFAULT true,
  requires_document boolean NOT NULL DEFAULT false,
  created_at        timestamptz NOT NULL DEFAULT now(),
  created_by        bigint REFERENCES users(id),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  updated_by        bigint REFERENCES users(id),
  UNIQUE NULLS NOT DISTINCT (bank_id, code),
  UNIQUE NULLS NOT DISTINCT (bank_id, sort_order)
);

CREATE TABLE customers (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name              text NOT NULL,
  mobile            text NOT NULL CHECK (mobile ~ '^[0-9]{10}$'),
  email             text,
  status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dropped')),
  dropped_reason_id bigint REFERENCES dropdown_values(id),
  dropped_remark    text,
  dropped_at        timestamptz,
  dropped_by        bigint REFERENCES users(id),
  created_at        timestamptz NOT NULL DEFAULT now(),
  created_by        bigint REFERENCES users(id),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  updated_by        bigint REFERENCES users(id),
  CHECK (status <> 'dropped' OR (dropped_reason_id IS NOT NULL AND dropped_at IS NOT NULL))
);
CREATE UNIQUE INDEX customers_mobile_uidx ON customers (mobile);

CREATE TABLE cases (
  id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id          bigint NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  bank_id              bigint NOT NULL REFERENCES banks(id),
  loan_amount          numeric(14,2) NOT NULL CHECK (loan_amount > 0),
  tracking_number      text,
  current_stage_id     bigint NOT NULL REFERENCES stages(id),
  assignee_id          bigint NOT NULL REFERENCES users(id),
  status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active','dropped','completed')),
  follow_up_date       date,
  dropped_reason_id    bigint REFERENCES dropdown_values(id),
  dropped_remark       text,
  dropped_at           timestamptz,
  dropped_by           bigint REFERENCES users(id),
  dropped_via_customer boolean NOT NULL DEFAULT false,
  sanctioned_amount    numeric(14,2),
  interest_rate        numeric(5,2),
  interest_type        text CHECK (interest_type IN ('fixed','floating')),
  tenure_months        integer CHECK (tenure_months > 0),
  sanction_date        date,
  disbursed_amount     numeric(14,2),
  disbursement_date    date,
  created_at           timestamptz NOT NULL DEFAULT now(),
  created_by           bigint REFERENCES users(id),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  updated_by           bigint REFERENCES users(id),
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
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id    bigint NOT NULL REFERENCES cases(id),
  stage_id   bigint NOT NULL REFERENCES stages(id),
  reached_at timestamptz NOT NULL,
  changed_by bigint NOT NULL REFERENCES users(id)
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
CREATE INDEX case_assignment_events_case_idx ON case_assignment_events (case_id, changed_at);

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
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id           bigint NOT NULL REFERENCES cases(id),
  recipient_user_id bigint NOT NULL REFERENCES users(id),
  kind              text NOT NULL CHECK (kind IN ('assigned','follow_up_due','follow_up_overdue')),
  due_on            date,
  sent_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (case_id, kind, due_on, recipient_user_id)
);

-- Seed: standard stages (bank_id NULL), in order (spec §5).
INSERT INTO stages (bank_id, code, name, sort_order, requires_document) VALUES
  (NULL, 'received',        'Received',        1, false),
  (NULL, 'logged_in',       'Logged in',       2, false),
  (NULL, 'under_sanction',  'Under sanction',  3, false),
  (NULL, 'sanctioned',      'Sanctioned',      4, false),
  (NULL, 'documentation',   'Documentation',   5, false),
  (NULL, 'disbursed',       'Disbursed',       6, false);

-- Seed: dropped reasons (spec §4.2, round 8).
INSERT INTO dropdown_values (category, value, sort_order) VALUES
  ('drop_reason', 'Customer not interested',               1),
  ('drop_reason', 'Not eligible',                          2),
  ('drop_reason', 'Rejected by bank',                      3),
  ('drop_reason', 'Customer chose another bank or DSA',    4),
  ('drop_reason', 'Customer unreachable',                  5),
  ('drop_reason', 'Duplicate or already processed',        6),
  ('drop_reason', 'Other',                                 7);
