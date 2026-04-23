ALTER TABLE organizations
    DROP COLUMN IF EXISTS masking_config,
    DROP COLUMN IF EXISTS store_span_content;
