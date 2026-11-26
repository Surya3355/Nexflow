ALTER TABLE cases ADD COLUMN IF NOT EXISTS disbursed_amount numeric(14,2);
ALTER TABLE cases ADD COLUMN IF NOT EXISTS disbursement_date date;
UPDATE cases c SET disbursed_amount = d.amount, disbursement_date = d.paid_on
FROM case_disbursements d WHERE d.case_id = c.id AND d.kind = 'full';
DROP TABLE IF EXISTS case_disbursements;
ALTER TABLE case_stage_events DROP COLUMN IF EXISTS remark;
ALTER TABLE banks DROP COLUMN IF EXISTS part_payments;
DROP INDEX IF EXISTS stages_role_uidx;
ALTER TABLE stages DROP COLUMN IF EXISTS role;
