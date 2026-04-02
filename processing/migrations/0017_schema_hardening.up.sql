-- #10: Add trigger to enforce cross-org FK integrity (api_key.org = session.org)
CREATE OR REPLACE FUNCTION check_session_api_key_org() RETURNS TRIGGER AS $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM api_keys WHERE id = NEW.api_key_id AND organization_id = NEW.organization_id
  ) THEN
    RAISE EXCEPTION 'api_key % does not belong to organization %', NEW.api_key_id, NEW.organization_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_session_api_key_org
  BEFORE INSERT ON sessions
  FOR EACH ROW EXECUTE FUNCTION check_session_api_key_org();

-- #15: Composite index for ListApiKeys hot path
CREATE INDEX IF NOT EXISTS idx_api_keys_org_active ON api_keys(organization_id, active);

-- #16: Index for password_reset_tokens cleanup cron
CREATE INDEX IF NOT EXISTS idx_prt_expires ON password_reset_tokens(expires_at) WHERE used_at IS NULL;

-- #17: CHECK constraint on session_timeout_seconds
ALTER TABLE organizations ADD CONSTRAINT chk_session_timeout_positive CHECK (session_timeout_seconds > 0);

-- #21: Make finish_reason NOT NULL with default
UPDATE spans SET finish_reason = 'unknown' WHERE finish_reason IS NULL;
ALTER TABLE spans ALTER COLUMN finish_reason SET NOT NULL;
ALTER TABLE spans ALTER COLUMN finish_reason SET DEFAULT 'unknown';

-- #33: Drop redundant single-column index on alert_events(organization_id)
-- (covered by composite idx_alert_events_org_triggered from migration 0014)
DROP INDEX IF EXISTS idx_alert_events_org_id;
