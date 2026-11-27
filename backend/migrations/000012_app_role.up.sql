-- The app works as nexflow_app (round 16, spec 2026-11-27-hardening §4.6).
-- Migration 000002 made the role with insert and read on the audit log only;
-- here it gets full use of every other table, so the running app can never
-- change or delete history. Migrations still run as the owner.
-- Serialise with other databases on the same server: roles are server-wide.
SELECT pg_advisory_xact_lock(7212016);

GRANT USAGE ON SCHEMA public TO nexflow_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO nexflow_app;
REVOKE UPDATE, DELETE, TRUNCATE ON audit_log FROM nexflow_app;
REVOKE ALL ON schema_migrations FROM nexflow_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO nexflow_app;

-- Tables added by later migrations get the same rights.
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexflow_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO nexflow_app;

-- The owner may act as the app role (SET ROLE nexflow_app on every app connection).
DO $$
BEGIN
  IF NOT pg_has_role(current_user, 'nexflow_app', 'MEMBER') THEN
    EXECUTE format('GRANT nexflow_app TO %I', current_user);
  END IF;
END
$$;
