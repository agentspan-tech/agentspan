ALTER TABLE spans
    ADD COLUMN masking_applied BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE span_masking_maps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    span_id     UUID NOT NULL REFERENCES spans(id) ON DELETE CASCADE,
    mask_type   TEXT NOT NULL,
    original_value TEXT NOT NULL,
    masked_value   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_span_masking_maps_span_id ON span_masking_maps(span_id);
