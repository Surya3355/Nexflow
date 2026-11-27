-- Application role for the audit log (spec §4.6, database-schema §6).
-- The app connects as a login role that is granted nexflow_app. That role
-- may only insert and read audit rows, so history cannot be edited.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexflow_app') THEN
    CREATE ROLE nexflow_app NOLOGIN;
  END IF;
END
$$;

GRANT USAGE ON SCHEMA public TO nexflow_app;
REVOKE ALL ON audit_log FROM nexflow_app;
GRANT SELECT, INSERT ON audit_log TO nexflow_app;
GRANT USAGE ON SEQUENCE audit_log_id_seq TO nexflow_app;
