-- name: GetMembershipByOrgAndUser :one
SELECT * FROM memberships
WHERE organization_id = $1 AND user_id = $2
LIMIT 1;

-- name: ListMembershipsByOrg :many
SELECT m.id, m.organization_id, m.user_id, m.role, m.created_at, u.email, u.name as user_name FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1
ORDER BY m.created_at ASC
LIMIT 200;

-- name: GetMembershipByID :one
SELECT m.id, m.organization_id, m.user_id, m.role, m.created_at, u.email, u.name as user_name FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.id = $1 AND m.organization_id = $2
LIMIT 1;

-- name: UpdateMembershipRole :exec
UPDATE memberships
SET role = $1
WHERE id = $2 AND organization_id = $3;

-- name: DeleteMembership :exec
DELETE FROM memberships
WHERE id = $1 AND organization_id = $2;

-- name: CountMembershipsByOrg :one
SELECT COUNT(*) FROM memberships
WHERE organization_id = $1;

-- name: GetOwnerMembership :one
SELECT * FROM memberships
WHERE organization_id = $1 AND role = 'owner'
LIMIT 1;

-- name: ListMembershipsByUser :many
SELECT organization_id FROM memberships
WHERE user_id = $1;
