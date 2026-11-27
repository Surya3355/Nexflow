-- Bank officers and officer per stage (round 15 tasks 6 and 7, spec 2026-11-26 §5.2).

-- The officer team a stage asks for. Each team at most once per stage list.
ALTER TABLE stages ADD COLUMN officer_team text
  CHECK (officer_team IN ('tdep','dovr','appraisal','fixing','officer'));
CREATE UNIQUE INDEX stages_officer_team_uidx ON stages (bank_id, officer_team) NULLS NOT DISTINCT
  WHERE officer_team IS NOT NULL;
UPDATE stages SET officer_team = 'officer' WHERE bank_id IS NULL AND code = 'received';
-- Banks already on HDFC stages.
UPDATE stages SET officer_team = 'tdep'      WHERE bank_id IS NOT NULL AND code = 'link_sent';
UPDATE stages SET officer_team = 'dovr'      WHERE bank_id IS NOT NULL AND code = 'dovr';
UPDATE stages SET officer_team = 'appraisal' WHERE bank_id IS NOT NULL AND code = 'appraisal';
UPDATE stages SET officer_team = 'fixing'    WHERE bank_id IS NOT NULL AND code = 'under_fixing';

-- Each bank's officers, per team. Rows are never deleted: removing sets
-- removed_at, and cases that point to the officer keep them.
CREATE TABLE bank_officers (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  bank_id    bigint NOT NULL REFERENCES banks(id),
  team       text NOT NULL CHECK (team IN ('tdep','dovr','appraisal','fixing','officer')),
  name       text NOT NULL CHECK (btrim(name) <> '' AND char_length(name) <= 80),
  phone      text CHECK (phone ~ '^[0-9]{10}$'),
  email      text CHECK (char_length(email) <= 254 AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$'),
  removed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id),
  -- Some officers give only a mobile, some only an email (owner, 2026-11-26).
  CONSTRAINT bank_officers_contact_check CHECK (phone IS NOT NULL OR email IS NOT NULL)
);
-- One bank and team cannot list the same mobile or email twice among current officers.
CREATE UNIQUE INDEX bank_officers_active_phone_uidx ON bank_officers (bank_id, team, phone) WHERE removed_at IS NULL AND phone IS NOT NULL;
CREATE UNIQUE INDEX bank_officers_active_email_uidx ON bank_officers (bank_id, team, lower(email)) WHERE removed_at IS NULL AND email IS NOT NULL;

-- Every officer pick on a case. The latest row per case and team is the
-- current officer; all rows are the history.
CREATE TABLE case_officers (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  case_id    bigint NOT NULL REFERENCES cases(id),
  team       text NOT NULL CHECK (team IN ('tdep','dovr','appraisal','fixing','officer')),
  officer_id bigint NOT NULL REFERENCES bank_officers(id),
  picked_at  timestamptz NOT NULL DEFAULT now(),
  picked_by  bigint NOT NULL REFERENCES users(id)
);
CREATE INDEX case_officers_case_idx ON case_officers (case_id, team, picked_at DESC, id DESC);
