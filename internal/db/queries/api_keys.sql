-- name: CreateAPIKey :one
INSERT INTO api_keys (id, user_id, label, key_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetAPIKeyByID :one
SELECT * FROM api_keys WHERE id = $1;

-- name: GetAPIKeyByHash :one
SELECT * FROM api_keys WHERE key_hash = $1;

-- name: ListAPIKeysByUser :many
SELECT * FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;

-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE id = $1;
