DROP INDEX IF EXISTS alert_log_pending_idx;
ALTER TABLE alert_log DROP COLUMN skip_reason, DROP COLUMN skipped_at;
