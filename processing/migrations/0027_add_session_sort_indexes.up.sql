-- Indexes for sortable session columns (cost, span_count, last_span_at).
-- started_at already has idx_sessions_org_started_at from migration 0010.

CREATE INDEX idx_sessions_org_cost
    ON sessions(organization_id, total_cost_usd DESC, id DESC);

CREATE INDEX idx_sessions_org_span_count
    ON sessions(organization_id, span_count DESC, id DESC);

CREATE INDEX idx_sessions_org_last_span_at
    ON sessions(organization_id, last_span_at DESC, id DESC);
