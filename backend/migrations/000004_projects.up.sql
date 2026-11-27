-- Projects (round 14, spec 2026-11-21 §4.1). Admin keeps the list, like banks.
-- Every case belongs to one project; NA is the fixed entry for loans without
-- a builder project. Existing cases become NA.
CREATE TABLE projects (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       text NOT NULL CHECK (btrim(name) <> ''),
  is_system  boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id)
);
CREATE UNIQUE INDEX projects_name_uidx ON projects (lower(btrim(name)));
-- Only one system project (NA).
CREATE UNIQUE INDEX projects_one_system_uidx ON projects (is_system) WHERE is_system;

INSERT INTO projects (name, is_system) VALUES ('NA', true);

-- The NA project's id, used as the column default so that rows inserted
-- without a project (old fixtures, restored snapshots) land on NA. The
-- service still requires staff to choose a project.
CREATE FUNCTION na_project_id() RETURNS bigint
  LANGUAGE sql STABLE AS $$ SELECT id FROM projects WHERE is_system $$;

ALTER TABLE cases ADD COLUMN project_id bigint REFERENCES projects(id);
UPDATE cases SET project_id = na_project_id();
ALTER TABLE cases ALTER COLUMN project_id SET DEFAULT na_project_id();
ALTER TABLE cases ALTER COLUMN project_id SET NOT NULL;
CREATE INDEX cases_project_idx ON cases (project_id);
