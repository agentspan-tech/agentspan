ALTER TABLE api_keys ADD COLUMN deactivated_by_deletion BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index for hard-delete cron query (GetOrganizationsDueForDeletion)
-- Covers WHERE status = 'pending_deletion' efficiently
CREATE INDEX idx_organizations_pending_deletion ON organizations(deletion_scheduled_at) WHERE status = 'pending_deletion';
