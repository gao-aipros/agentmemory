-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, name, totp_secret, totp_enabled)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: UpdateUser :one
UPDATE users SET
    email = $2,
    password_hash = $3,
    name = $4,
    totp_secret = $5,
    totp_enabled = $6
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: ListUsers :many
SELECT * FROM users ORDER BY created_at DESC;
