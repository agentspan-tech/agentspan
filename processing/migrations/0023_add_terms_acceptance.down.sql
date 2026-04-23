ALTER TABLE users
    DROP COLUMN IF EXISTS accepted_terms_at,
    DROP COLUMN IF EXISTS accepted_privacy_at,
    DROP COLUMN IF EXISTS policy_version;
