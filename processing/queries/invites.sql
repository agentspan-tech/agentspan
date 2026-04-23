-- name: CreateInvite :one
INSERT INTO invites (organization_id, invited_by, email, token_hash, role, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetInviteByTokenHash :one
SELECT * FROM invites
WHERE token_hash = $1 AND accepted_at IS NULL AND expires_at > NOW()
LIMIT 1;

-- name: ListPendingInvitesByOrg :many
SELECT * FROM invites
WHERE organization_id = $1 AND accepted_at IS NULL AND expires_at > NOW()
ORDER BY created_at DESC;

-- name: CountPendingInvitesByOrg :one
SELECT COUNT(*) FROM invites
WHERE organization_id = $1 AND accepted_at IS NULL AND expires_at > NOW();

-- name: AcceptInvite :exec
UPDATE invites
SET accepted_at = NOW()
WHERE id = $1;

-- name: DeleteInvite :exec
DELETE FROM invites
WHERE id = $1 AND organization_id = $2;

-- name: DeleteExpiredInvites :exec
DELETE FROM invites
WHERE expires_at < NOW() AND accepted_at IS NULL;

-- name: GetInviteByEmailAndOrg :one
SELECT * FROM invites
WHERE email = $1 AND organization_id = $2 AND accepted_at IS NULL AND expires_at > NOW()
LIMIT 1;
