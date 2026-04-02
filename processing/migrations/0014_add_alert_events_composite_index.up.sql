CREATE INDEX idx_alert_events_org_triggered ON alert_events(organization_id, triggered_at DESC);
