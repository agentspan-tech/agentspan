-- Set defaults for existing rows with NULL values before adding NOT NULL constraints.
UPDATE alert_rules SET threshold = 0 WHERE threshold IS NULL;
UPDATE alert_rules SET window_minutes = 0 WHERE window_minutes IS NULL;

-- Add NOT NULL constraints with defaults.
-- threshold defaults to 0 (reactive alerts like new_failure_cluster don't use it).
-- window_minutes defaults to 0 (reactive alerts don't use it; cron alerts validate > 0 at creation).
ALTER TABLE alert_rules ALTER COLUMN threshold SET NOT NULL;
ALTER TABLE alert_rules ALTER COLUMN threshold SET DEFAULT 0;
ALTER TABLE alert_rules ALTER COLUMN window_minutes SET NOT NULL;
ALTER TABLE alert_rules ALTER COLUMN window_minutes SET DEFAULT 0;
