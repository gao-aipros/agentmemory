-- name: InsertObservation :one
INSERT INTO observations (id, session_id, owner_type, owner_user_id, owner_team_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetObservation :one
SELECT * FROM observations WHERE id = $1;

-- name: ListObservationsBySession :many
SELECT * FROM observations WHERE session_id = $1 ORDER BY timestamp LIMIT $2;

-- name: DeleteObservation :exec
DELETE FROM observations WHERE id = $1;

-- name: ListRecentObservations :many
SELECT * FROM observations ORDER BY created_at DESC LIMIT $1;

-- name: GetObservationsByIDs :many
SELECT * FROM observations WHERE id = ANY($1::text[]);

-- name: ListObservationsByUserID :many
SELECT o.* FROM observations o
JOIN sessions s ON o.session_id = s.id
WHERE s.user_id = $1
ORDER BY o.created_at DESC
LIMIT $2;
