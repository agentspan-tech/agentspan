DROP INDEX IF EXISTS idx_organizations_pending_deletion;
ALTER TABLE api_keys DROP COLUMN deactivated_by_deletion;
