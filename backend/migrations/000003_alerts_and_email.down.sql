ALTER TABLE users DROP COLUMN IF EXISTS email;
ALTER TABLE alert_log DROP COLUMN IF EXISTS last_error;
ALTER TABLE alert_log DROP COLUMN IF EXISTS attempts;
ALTER TABLE alert_log DROP COLUMN IF EXISTS queued_at;
UPDATE alert_log SET sent_at = now() WHERE sent_at IS NULL;
ALTER TABLE alert_log ALTER COLUMN sent_at SET DEFAULT now();
ALTER TABLE alert_log ALTER COLUMN sent_at SET NOT NULL;
