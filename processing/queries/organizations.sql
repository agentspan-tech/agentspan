-- name: CreateOrganization :one
INSERT INTO organizations (name, slug, plan)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations
WHERE id = $1
LIMIT 1;

-- name: GetOrganizationByIDForUpdate :one
SELECT * FROM organizations
WHERE id = $1
LIMIT 1
FOR UPDATE;

-- name: GetOrganizationsByUserID :many
SELECT o.* FROM organizations o
JOIN memberships m ON m.organization_id = o.id
WHERE m.user_id = $1
ORDER BY o.created_at ASC;

-- name: CreateMembership :one
INSERT INTO memberships (organization_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetMembership :one
SELECT * FROM memberships
WHERE organization_id = $1 AND user_id = $2
LIMIT 1;

-- name: UpdateOrganizationSettings :exec
UPDATE organizations
SET locale = $2, session_timeout_seconds = $3, updated_at = NOW()
WHERE id = $1;

-- name: UpdateOrganizationName :exec
UPDATE organizations
SET name = $1, slug = $2, updated_at = NOW()
WHERE id = $3;

-- name: SetOrganizationPendingDeletion :exec
UPDATE organizations
SET status = 'pending_deletion', deletion_scheduled_at = $2, updated_at = NOW()
WHERE id = $1;

-- name: RestoreOrganization :exec
UPDATE organizations
SET status = 'active', deletion_scheduled_at = NULL, updated_at = NOW()
WHERE id = $1;

-- name: GetOrganizationsDueForDeletion :many
SELECT * FROM organizations
WHERE status = 'pending_deletion' AND deletion_scheduled_at <= NOW();

-- name: DeleteOrganization :exec
DELETE FROM organizations
WHERE id = $1;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations
WHERE slug = $1
LIMIT 1;

-- name: UpdateOrganizationPrivacySettings :exec
UPDATE organizations
SET store_span_content = $2, masking_config = $3, updated_at = NOW()
WHERE id = $1;

-- name: GetOrganizationPrivacySettings :one
SELECT store_span_content, masking_config FROM organizations
WHERE id = $1;
