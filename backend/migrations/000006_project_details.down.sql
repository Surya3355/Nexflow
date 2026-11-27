ALTER TABLE cases DROP CONSTRAINT IF EXISTS cases_builder_contact_fk;
ALTER TABLE cases DROP COLUMN IF EXISTS builder_contact_id;
ALTER TABLE leads DROP CONSTRAINT IF EXISTS leads_builder_contact_fk;
ALTER TABLE leads DROP COLUMN IF EXISTS builder_contact_id;
DROP TABLE IF EXISTS project_contacts;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_system_no_details;
ALTER TABLE projects DROP COLUMN IF EXISTS address;
ALTER TABLE projects DROP COLUMN IF EXISTS type;
