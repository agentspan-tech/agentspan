ALTER TABLE organizations
    ADD COLUMN store_span_content BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN masking_config JSONB;
