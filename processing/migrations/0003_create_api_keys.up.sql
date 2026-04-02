CREATE TABLE api_keys (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                   TEXT NOT NULL,
    provider_type          TEXT NOT NULL,
    provider_key_encrypted BYTEA NOT NULL,
    base_url               TEXT,
    key_digest             TEXT NOT NULL UNIQUE,
    display                TEXT NOT NULL,
    active                 BOOLEAN NOT NULL DEFAULT TRUE,
    last_used_at           TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Hot-path partial index for proxy auth validation (D-07)
CREATE UNIQUE INDEX idx_api_keys_key_digest_active
    ON api_keys(key_digest)
    WHERE active = TRUE;

CREATE INDEX idx_api_keys_org_id ON api_keys(organization_id);
