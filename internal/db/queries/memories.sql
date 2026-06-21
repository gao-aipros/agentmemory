-- name: InsertMemory :one
INSERT INTO memories (id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetMemory :one
SELECT * FROM memories WHERE id = $1;

-- name: ListMemoriesByOwner :many
SELECT * FROM memories WHERE owner_user_id = $1 ORDER BY created_at DESC;

-- name: UpdateVisibility :exec
UPDATE memories SET visibility = $2 WHERE id = $1;

-- name: DeleteMemory :exec
DELETE FROM memories WHERE id = $1;
