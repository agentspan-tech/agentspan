CREATE TABLE system_prompts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    content         TEXT NOT NULL,
    content_hash    TEXT NOT NULL,
    short_uid       TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, content_hash)
);

CREATE INDEX idx_system_prompts_org_id ON system_prompts(organization_id);
CREATE INDEX idx_system_prompts_content_hash ON system_prompts(organization_id, content_hash);

CREATE TABLE span_system_prompts (
    span_id          UUID NOT NULL REFERENCES spans(id) ON DELETE CASCADE,
    system_prompt_id UUID NOT NULL REFERENCES system_prompts(id) ON DELETE CASCADE,
    PRIMARY KEY (span_id, system_prompt_id)
);

CREATE TABLE failure_clusters (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    label           TEXT NOT NULL,
    description     TEXT,
    session_count   INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_failure_clusters_org_id ON failure_clusters(organization_id);

CREATE TABLE session_failure_clusters (
    session_id         UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    failure_cluster_id UUID NOT NULL REFERENCES failure_clusters(id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, failure_cluster_id)
);
