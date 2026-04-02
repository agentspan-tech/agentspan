-- name: GetUserByEmail :one
SELECT id, email, name, password_hash, email_verified_at, created_at, updated_at FROM users
WHERE email = $1
LIMIT 1;

-- name: GetUserByID :one
SELECT id, email, name, password_hash, email_verified_at, created_at, updated_at FROM users
WHERE id = $1
LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (email, name, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: SetUserEmailVerified :exec
UPDATE users
SET email_verified_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $1, password_changed_at = NOW(), updated_at = NOW()
WHERE id = $2;

-- name: GetUserPasswordChangedAt :one
SELECT password_changed_at FROM users WHERE id = $1;

-- name: UpdateUserName :exec
UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2;

-- name: UpdateUserEmail :exec
UPDATE users SET email = $1, email_verified_at = NULL, updated_at = NOW() WHERE id = $2;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;
