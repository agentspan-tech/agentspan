DROP TRIGGER IF EXISTS trg_session_api_key_org ON sessions;
DROP FUNCTION IF EXISTS check_session_api_key_org();
DROP INDEX IF EXISTS idx_api_keys_org_active;
DROP INDEX IF EXISTS idx_prt_expires;
ALTER TABLE organizations DROP CONSTRAINT IF EXISTS chk_session_timeout_positive;
ALTER TABLE spans ALTER COLUMN finish_reason DROP NOT NULL;
ALTER TABLE spans ALTER COLUMN finish_reason DROP DEFAULT;
CREATE INDEX IF NOT EXISTS idx_alert_events_org_id ON alert_events(organization_id);
