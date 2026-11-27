-- Let the owner switch to nexflow_app on PostgreSQL 16+ (round 16 review).
-- When a non-superuser owner created nexflow_app (000002), PostgreSQL 16
-- gave it ADMIN on the role but not SET, so 000012's membership check passed
-- while SET ROLE nexflow_app is refused. Grant SET explicitly.
SELECT pg_advisory_xact_lock(7212016);
DO $$
BEGIN
  IF current_setting('server_version_num')::int >= 160000 THEN
    IF NOT pg_has_role(current_user, 'nexflow_app', 'SET') THEN
      EXECUTE format('GRANT nexflow_app TO %I WITH SET TRUE', current_user);
    END IF;
  ELSIF NOT pg_has_role(current_user, 'nexflow_app', 'MEMBER') THEN
    EXECUTE format('GRANT nexflow_app TO %I', current_user);
  END IF;
END
$$;
