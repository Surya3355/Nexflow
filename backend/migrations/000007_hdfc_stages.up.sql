-- HDFC stages and part disbursement (round 15 task 10, spec 2026-11-25 §5.2).

-- A stage's role tells rules and reports what the stage does, whatever its name.
ALTER TABLE stages ADD COLUMN role text CHECK (role IN ('start','tracking','sanction','disbursement'));
CREATE UNIQUE INDEX stages_role_uidx ON stages (bank_id, role) NULLS NOT DISTINCT WHERE role IS NOT NULL;
UPDATE stages SET role = 'start'        WHERE bank_id IS NULL AND code = 'received';
UPDATE stages SET role = 'tracking'     WHERE bank_id IS NULL AND code = 'logged_in';
UPDATE stages SET role = 'sanction'     WHERE bank_id IS NULL AND code = 'sanctioned';
UPDATE stages SET role = 'disbursement' WHERE bank_id IS NULL AND code = 'disbursed';

-- Banks that pay loans in parts (HDFC, later private banks; not SBI).
ALTER TABLE banks ADD COLUMN part_payments boolean NOT NULL DEFAULT false;

-- Optional remark on a stage move.
ALTER TABLE case_stage_events ADD COLUMN remark text CHECK (char_length(remark) <= 500);

-- Every disbursement payment of a case. A case has at most one Full payment.
CREATE TABLE case_disbursements (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id    bigint NOT NULL REFERENCES cases(id),
  amount     numeric(14,2) NOT NULL CHECK (amount > 0),
  paid_on    date NOT NULL,
  kind       text NOT NULL CHECK (kind IN ('part','full')),
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id)
);
CREATE INDEX case_disbursements_case_idx ON case_disbursements (case_id, paid_on);
CREATE INDEX case_disbursements_paid_on_idx ON case_disbursements (paid_on);
CREATE UNIQUE INDEX case_disbursements_full_uidx ON case_disbursements (case_id) WHERE kind = 'full';

-- The single disbursed amount of old cases becomes their Full payment.
INSERT INTO case_disbursements (case_id, amount, paid_on, kind, created_by, updated_by)
SELECT c.id, c.disbursed_amount,
       COALESCE(c.disbursement_date,
                (SELECT ((max(e.reached_at)) AT TIME ZONE 'Asia/Kolkata')::date
                   FROM case_stage_events e JOIN stages s ON s.id = e.stage_id
                  WHERE e.case_id = c.id AND s.code = 'disbursed'),
                (c.updated_at AT TIME ZONE 'Asia/Kolkata')::date),
       'full', c.updated_by, c.updated_by
FROM cases c
WHERE c.disbursed_amount IS NOT NULL AND c.disbursed_amount > 0;

ALTER TABLE cases DROP COLUMN disbursed_amount;
ALTER TABLE cases DROP COLUMN disbursement_date;
