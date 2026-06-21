-- name: InsertEmbedding :exec
INSERT INTO observation_embeddings (observation_id, embedding, model)
VALUES ($1, $2, $3);

-- name: GetEmbedding :one
SELECT * FROM observation_embeddings WHERE observation_id = $1;

-- name: DeleteEmbedding :exec
DELETE FROM observation_embeddings WHERE observation_id = $1;
