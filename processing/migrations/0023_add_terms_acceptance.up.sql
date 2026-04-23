ALTER TABLE users
    ADD COLUMN accepted_terms_at   TIMESTAMPTZ,
    ADD COLUMN accepted_privacy_at TIMESTAMPTZ,
    ADD COLUMN policy_version      INTEGER NOT NULL DEFAULT 1;
