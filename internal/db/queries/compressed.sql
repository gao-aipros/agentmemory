-- name: InsertCompressedObservation :one
INSERT INTO compressed_observations (id, observation_ids, session_id, visibility, compressed_text, concepts)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetCompressedObservation :one
SELECT * FROM compressed_observations WHERE id = $1;

-- name: ListCompressedBySession :many
SELECT * FROM compressed_observations WHERE session_id = $1 ORDER BY created_at;

-- name: DeleteCompressedObservation :exec
DELETE FROM compressed_observations WHERE id = $1;

-- name: InsertCompressedEmbedding :exec
INSERT INTO compressed_embeddings (compressed_id, embedding, model)
VALUES ($1, $2, $3);

-- name: GetCompressedEmbedding :one
SELECT * FROM compressed_embeddings WHERE compressed_id = $1;
