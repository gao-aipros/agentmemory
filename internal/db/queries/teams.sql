-- name: CreateTeam :one
INSERT INTO teams (id, name, owner_id, default_visibility)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetTeam :one
SELECT * FROM teams WHERE id = $1;

-- name: UpdateTeam :one
UPDATE teams SET name = $2, default_visibility = $3
WHERE id = $1
RETURNING *;

-- name: DeleteTeam :exec
DELETE FROM teams WHERE id = $1;

-- name: ListTeamsByOwner :many
SELECT * FROM teams WHERE owner_id = $1 ORDER BY created_at DESC;
