-- name: BatchInsertCompressedObservations :copyfrom
INSERT INTO compressed_observations (id, observation_ids, session_id, compressed_text, concepts, owner_type, owner_user_id, visibility)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: MarkObservationsCompressed :exec
UPDATE observations SET compressed_at = NOW() WHERE id = ANY($1::text[]);
