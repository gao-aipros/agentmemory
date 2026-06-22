-- name: CreateSession :one
INSERT INTO sessions (id, user_id, team_id, status)
VALUES ($1, $2, $3, 'active')
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: EndSession :one
UPDATE sessions SET ended_at = now(), status = 'ended' WHERE id = $1 RETURNING *;

-- name: ListSessionsByUser :many
SELECT * FROM sessions WHERE user_id = $1 ORDER BY started_at DESC LIMIT $2;

-- name: GetActiveSession :one
SELECT * FROM sessions WHERE user_id = $1 AND status = 'active' LIMIT 1;
