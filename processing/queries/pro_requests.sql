-- name: CreateProRequest :one
INSERT INTO pro_requests (email, company, message, source)
VALUES ($1, $2, $3, $4)
RETURNING *;
