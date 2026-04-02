-- Partial unique index for explicit session ON CONFLICT (D-05, Pitfall 1).
-- Allows INSERT ON CONFLICT (api_key_id, external_id) WHERE status = 'in_progress'.
CREATE UNIQUE INDEX idx_sessions_api_key_external_id_in_progress
    ON sessions(api_key_id, external_id)
    WHERE status = 'in_progress' AND external_id IS NOT NULL;

-- Composite index for provider_type filter subquery on sessions list (D-13, Pitfall 2, DAPI-04).
CREATE INDEX idx_spans_session_id_provider_type
    ON spans(session_id, provider_type);

-- Composite index for session list cursor pagination ORDER BY (D-12, DAPI-04).
CREATE INDEX idx_sessions_org_started_at
    ON sessions(organization_id, started_at DESC, id DESC);
