-- Leads (round 15, spec 2026-11-23 §4.1). A lead is an enquiry for one
-- project, before the customer comes in and a case is opened.
CREATE TABLE leads (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id       bigint NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  project_id        bigint NOT NULL REFERENCES projects(id),
  assignee_id       bigint NOT NULL REFERENCES users(id),
  status            text NOT NULL DEFAULT 'open' CHECK (status IN ('open','converted','dropped')),
  next_call_date    date,
  dropped_reason_id bigint REFERENCES dropdown_values(id),
  dropped_remark    text,
  dropped_at        timestamptz,
  dropped_by        bigint REFERENCES users(id),
  converted_at      timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  created_by        bigint REFERENCES users(id),
  updated_at        timestamptz NOT NULL DEFAULT now(),
  updated_by        bigint REFERENCES users(id),
  CHECK (status <> 'dropped' OR (dropped_reason_id IS NOT NULL AND dropped_at IS NOT NULL))
);
CREATE INDEX leads_customer_idx ON leads (customer_id);
CREATE INDEX leads_open_call_idx ON leads (next_call_date) WHERE status = 'open';
CREATE UNIQUE INDEX leads_one_open_per_project_uidx ON leads (customer_id, project_id) WHERE status = 'open';

-- Contact log. The app only inserts rows; the archive removes them with the customer.
CREATE TABLE lead_contacts (
  id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  lead_id        bigint NOT NULL REFERENCES leads(id),
  response       text NOT NULL CHECK (response IN ('interested','call_back','not_reachable','not_interested')),
  remark         text NOT NULL CHECK (btrim(remark) <> ''),
  next_call_date date,
  created_at     timestamptz NOT NULL DEFAULT now(),
  created_by     bigint REFERENCES users(id)
);
CREATE INDEX lead_contacts_lead_idx ON lead_contacts (lead_id, created_at);

ALTER TABLE cases ADD COLUMN lead_id bigint UNIQUE REFERENCES leads(id) ON DELETE SET NULL;
ALTER TABLE cases ADD COLUMN unit_number text CHECK (char_length(unit_number) <= 20);

-- Alerts point at a case or a lead, never both.
ALTER TABLE alert_log ALTER COLUMN case_id DROP NOT NULL;
ALTER TABLE alert_log ADD COLUMN lead_id bigint REFERENCES leads(id);
ALTER TABLE alert_log ADD CONSTRAINT alert_log_one_target CHECK (num_nonnulls(case_id, lead_id) = 1);
-- Each alert is queued once per target. The old case-only rule would treat
-- alerts of two different leads (case_id NULL) as the same alert.
ALTER TABLE alert_log DROP CONSTRAINT alert_log_case_id_kind_due_on_recipient_user_id_key;
CREATE UNIQUE INDEX alert_log_case_once_uidx ON alert_log (case_id, kind, due_on, recipient_user_id) NULLS NOT DISTINCT WHERE case_id IS NOT NULL;
CREATE UNIQUE INDEX alert_log_lead_once_uidx ON alert_log (lead_id, kind, due_on, recipient_user_id) NULLS NOT DISTINCT WHERE lead_id IS NOT NULL;

-- Active customers with no case become open leads on NA, so none are lost.
INSERT INTO leads (customer_id, project_id, assignee_id, next_call_date, created_by, updated_by)
SELECT cu.id, na_project_id(), who.id, (now() AT TIME ZONE 'Asia/Kolkata')::date, who.id, who.id
FROM customers cu
CROSS JOIN LATERAL (
  SELECT COALESCE(cu.created_by,
                  (SELECT u.id FROM users u WHERE u.role = 'admin' AND u.is_active ORDER BY u.id LIMIT 1)) AS id
) who
WHERE cu.status = 'active'
  AND who.id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM cases c WHERE c.customer_id = cu.id);
