-- name: InsertObservation :one
INSERT INTO observations (id, session_id, owner_type, owner_user_id, owner_team_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetObservation :one
SELECT * FROM observations WHERE id = $1;

-- name: ListObservationsBySession :many
SELECT * FROM observations WHERE session_id = $1 ORDER BY timestamp;

-- name: DeleteObservation :exec
DELETE FROM observations WHERE id = $1;
