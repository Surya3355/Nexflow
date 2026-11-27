DROP TABLE IF EXISTS case_officers;
DROP TABLE IF EXISTS bank_officers;
DROP INDEX IF EXISTS stages_officer_team_uidx;
ALTER TABLE stages DROP COLUMN IF EXISTS officer_team;
