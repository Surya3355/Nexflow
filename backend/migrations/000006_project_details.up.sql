-- Project details and builder contacts (round 15 task 4, spec 2026-11-24 §4.1).
ALTER TABLE projects ADD COLUMN type text CHECK (type IN ('high_rise','apartments','plots','villas'));
ALTER TABLE projects ADD COLUMN address text NOT NULL DEFAULT '';
-- NA has no builder: no type and no address.
ALTER TABLE projects ADD CONSTRAINT projects_system_no_details CHECK (NOT is_system OR (type IS NULL AND address = ''));

-- Each project's own contact list. Rows are never deleted: removing a contact
-- sets removed_at, and leads and cases that point to it keep it.
CREATE TABLE project_contacts (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  project_id bigint NOT NULL REFERENCES projects(id),
  name       text NOT NULL CHECK (btrim(name) <> ''),
  phone      text NOT NULL CHECK (phone ~ '^[0-9]{10}$'),
  email      text,
  label      text CHECK (char_length(label) <= 40),
  removed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by bigint REFERENCES users(id),
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by bigint REFERENCES users(id),
  CONSTRAINT project_contacts_project_id_uidx UNIQUE (project_id, id)
);
-- One project cannot list the same number twice among its current contacts.
CREATE UNIQUE INDEX project_contacts_active_phone_uidx ON project_contacts (project_id, phone) WHERE removed_at IS NULL;

-- A lead or case points to a contact of its own project (NULL = none).
ALTER TABLE leads ADD COLUMN builder_contact_id bigint;
ALTER TABLE leads ADD CONSTRAINT leads_builder_contact_fk
  FOREIGN KEY (project_id, builder_contact_id) REFERENCES project_contacts (project_id, id);
ALTER TABLE cases ADD COLUMN builder_contact_id bigint;
ALTER TABLE cases ADD CONSTRAINT cases_builder_contact_fk
  FOREIGN KEY (project_id, builder_contact_id) REFERENCES project_contacts (project_id, id);
