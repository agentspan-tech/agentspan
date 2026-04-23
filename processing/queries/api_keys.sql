-- name: CreateApiKey :one
INSERT INTO api_keys (organization_id, name, provider_type, provider_key_encrypted, base_url, key_digest, display)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetApiKeyByDigest :one
SELECT id, organization_id, name, provider_type, provider_key_encrypted, base_url, key_digest, display, active, last_used_at, created_at, deactivated_by_deletion FROM api_keys
WHERE key_digest = $1 AND active = TRUE
LIMIT 1;

-- name: ListApiKeysByOrg :many
SELECT id, organization_id, name, provider_type, base_url, display, active, last_used_at, created_at FROM api_keys
WHERE organization_id = $1
ORDER BY created_at DESC
LIMIT 200;

-- name: DeactivateApiKey :exec
UPDATE api_keys
SET active = FALSE
WHERE id = $1 AND organization_id = $2;

-- name: UpdateApiKeyLastUsed :exec
UPDATE api_keys
SET last_used_at = NOW()
WHERE id = $1;

-- name: DeactivateApiKeysByOrg :exec
UPDATE api_keys
SET active = FALSE, deactivated_by_deletion = TRUE
WHERE organization_id = $1 AND active = TRUE;

-- name: ReactivateApiKeysByOrg :exec
UPDATE api_keys
SET active = TRUE, deactivated_by_deletion = FALSE
WHERE organization_id = $1 AND deactivated_by_deletion = TRUE;

-- name: GetApiKeyByID :one
SELECT id, organization_id, name, provider_type, provider_key_encrypted, base_url, key_digest, display, active, last_used_at, created_at, deactivated_by_deletion FROM api_keys
WHERE id = $1 AND organization_id = $2
LIMIT 1;
