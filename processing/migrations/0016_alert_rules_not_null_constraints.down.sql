ALTER TABLE alert_rules ALTER COLUMN threshold DROP NOT NULL;
ALTER TABLE alert_rules ALTER COLUMN threshold DROP DEFAULT;
ALTER TABLE alert_rules ALTER COLUMN window_minutes DROP NOT NULL;
ALTER TABLE alert_rules ALTER COLUMN window_minutes DROP DEFAULT;
