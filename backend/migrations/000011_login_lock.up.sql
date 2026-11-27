-- Login lock (round 16, spec 2026-11-27-hardening §4.1): 5 wrong passwords in a
-- row lock the username for 15 minutes. Kept in the database so a restart does
-- not clear it.
ALTER TABLE users
  ADD COLUMN failed_logins integer NOT NULL DEFAULT 0,
  ADD COLUMN locked_until  timestamptz;
