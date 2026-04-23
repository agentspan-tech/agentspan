DROP TABLE IF EXISTS span_masking_maps;
ALTER TABLE spans DROP COLUMN IF EXISTS masking_applied;
