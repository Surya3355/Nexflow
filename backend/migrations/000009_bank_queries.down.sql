DELETE FROM alert_log WHERE kind IN ('query_due', 'query_overdue');
ALTER TABLE alert_log DROP CONSTRAINT IF EXISTS alert_log_kind_check;
ALTER TABLE alert_log ADD CONSTRAINT alert_log_kind_check CHECK (kind IN ('assigned', 'follow_up_due', 'follow_up_overdue'));
DROP TABLE IF EXISTS case_query_events;
DROP TABLE IF EXISTS case_queries;
