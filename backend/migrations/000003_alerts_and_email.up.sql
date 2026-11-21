-- Alerts are queued first and sent later, so sent_at is optional until sent.
ALTER TABLE alert_log ALTER COLUMN sent_at DROP NOT NULL;
ALTER TABLE alert_log ALTER COLUMN sent_at DROP DEFAULT;
ALTER TABLE alert_log ADD COLUMN queued_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE alert_log ADD COLUMN attempts integer NOT NULL DEFAULT 0;
ALTER TABLE alert_log ADD COLUMN last_error text;

-- Staff email address for alerts (spec §10). Set by admin.
ALTER TABLE users ADD COLUMN email text;
