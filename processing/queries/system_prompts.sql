-- name: CountSystemPromptsForOrg :one
SELECT COUNT(*) FROM system_prompts WHERE organization_id = $1;

-- name: FindOrCreateSystemPrompt :one
INSERT INTO system_prompts (organization_id, content, content_hash, short_uid)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, content_hash)
DO UPDATE SET created_at = system_prompts.created_at
RETURNING *;

-- name: InsertSpanSystemPrompt :exec
INSERT INTO span_system_prompts (span_id, system_prompt_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: ReplaceSpanInputPrefix :exec
UPDATE spans
SET input = sqlc.arg(replacement) || substring(input FROM length(sqlc.arg(old_prefix)::text) + 1)
WHERE id = sqlc.arg(id)::uuid AND organization_id = sqlc.arg(organization_id)::uuid AND input IS NOT NULL AND starts_with(input, sqlc.arg(old_prefix)::text);

-- name: ListSystemPrompts :many
SELECT
    sp.id,
    sp.short_uid,
    sp.content_hash,
    length(sp.content)::int AS content_length,
    left(sp.content, 200) AS content_preview,
    sp.created_at,
    COUNT(DISTINCT ssp.span_id)::int AS span_count,
    COUNT(DISTINCT s.session_id)::int AS session_count,
    MAX(s.created_at)::timestamptz AS last_seen_at
FROM system_prompts sp
LEFT JOIN span_system_prompts ssp ON ssp.system_prompt_id = sp.id
LEFT JOIN spans s ON s.id = ssp.span_id
WHERE sp.organization_id = $1
GROUP BY sp.id
ORDER BY sp.created_at DESC;

-- name: GetSystemPromptByID :one
SELECT
    sp.id,
    sp.short_uid,
    sp.content,
    sp.content_hash,
    sp.created_at,
    COUNT(DISTINCT ssp.span_id)::int AS span_count,
    COUNT(DISTINCT s.session_id)::int AS session_count,
    MAX(s.created_at)::timestamptz AS last_seen_at
FROM system_prompts sp
LEFT JOIN span_system_prompts ssp ON ssp.system_prompt_id = sp.id
LEFT JOIN spans s ON s.id = ssp.span_id
WHERE sp.id = $1 AND sp.organization_id = $2
GROUP BY sp.id;
