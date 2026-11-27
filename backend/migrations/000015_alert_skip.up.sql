-- Alerts that will not be sent (round 16, spec 2026-11-27-hardening §6):
-- tried 5 times, older than 3 days, or the person has no email address.
-- Skipped alerts are kept for the record but no longer picked up.
ALTER TABLE alert_log ADD COLUMN skipped_at timestamptz, ADD COLUMN skip_reason text;
CREATE INDEX alert_log_pending_idx ON alert_log (attempts, queued_at) WHERE sent_at IS NULL AND skipped_at IS NULL;
