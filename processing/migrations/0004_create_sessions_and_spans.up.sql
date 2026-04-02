CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    api_key_id      UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    external_id     TEXT,
    agent_name      TEXT,
    status          TEXT NOT NULL DEFAULT 'in_progress'
                        CHECK (status IN ('in_progress', 'completed', 'completed_with_errors', 'failed', 'abandoned')),
    narrative       TEXT,
    total_cost_usd  NUMERIC(12, 8) NOT NULL DEFAULT 0,
    span_count      INT NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_span_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite index for session grouping + cron closure queries (D-07)
CREATE INDEX idx_sessions_api_key_status_last_span
    ON sessions(api_key_id, status, last_span_at);

CREATE INDEX idx_sessions_org_id ON sessions(organization_id);
CREATE INDEX idx_sessions_external_id ON sessions(external_id) WHERE external_id IS NOT NULL;

CREATE TABLE spans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_type   TEXT NOT NULL,
    model           TEXT NOT NULL,
    input           TEXT,
    output          TEXT,
    input_tokens    INT,
    output_tokens   INT,
    cost_usd        NUMERIC(12, 8),
    duration_ms     INT NOT NULL,
    http_status     INT NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Composite index for session span queries (D-07)
CREATE INDEX idx_spans_session_id_created_at ON spans(session_id, created_at);
CREATE INDEX idx_spans_org_id ON spans(organization_id);
