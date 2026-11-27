-- The last day the daily jobs ran (round 16, spec 2026-11-27-hardening §6),
-- so a server that was down at 02:00 IST runs them when it starts.
CREATE TABLE job_runs (
  name     text PRIMARY KEY,
  last_day date NOT NULL
);
