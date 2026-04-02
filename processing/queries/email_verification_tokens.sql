-- name: CreateEmailVerificationToken :one
INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetEmailVerificationTokenByHash :one
SELECT * FROM email_verification_tokens
WHERE token_hash = $1 AND expires_at > NOW()
LIMIT 1;

-- name: DeleteEmailVerificationTokensByUser :exec
DELETE FROM email_verification_tokens
WHERE user_id = $1;
