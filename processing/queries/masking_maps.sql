-- name: InsertSpanMaskingMap :exec
INSERT INTO span_masking_maps (span_id, mask_type, original_value, masked_value)
VALUES ($1, $2, $3, $4);

-- name: GetSpanMaskingMaps :many
SELECT id, span_id, mask_type, original_value, masked_value, created_at
FROM span_masking_maps
WHERE span_id = $1
ORDER BY created_at;
