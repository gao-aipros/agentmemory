-- name: Bm25SearchMemories :many
-- BM25 full-text search over the memories table using ParadeDB's @@@ operator.
-- Explicit casts ensure sqlc generates proper Go types.
-- sqlc.narg(owner_user_id) enforces cross-tenant isolation.
SELECT m.id::text AS id, paradedb.score(m.id)::float8 AS bm25_score
FROM memories m
WHERE m.source = 'manual_save'
  AND (sqlc.narg('owner_user_id')::text IS NULL OR m.owner_user_id = sqlc.narg('owner_user_id'))
  AND m.content @@@ paradedb.parse(sqlc.arg('query_text'))
ORDER BY paradedb.score(m.id) DESC
LIMIT sqlc.arg('result_limit');

-- name: VectorSearchMemories :many
-- Cosine similarity search over memory embeddings using pgvector.
-- Returns memory IDs with similarity scores (1 - cosine_distance).
-- sqlc.narg(owner_user_id) enforces cross-tenant isolation.
SELECT
    me.memory_id AS id,
    (1.0 - (me.embedding <=> sqlc.arg('embedding')))::float8 AS vector_score
FROM memory_embeddings me
JOIN memories m ON me.memory_id = m.id
WHERE me.embedding IS NOT NULL
  AND m.source = 'manual_save'
  AND (sqlc.narg('owner_user_id')::text IS NULL OR m.owner_user_id = sqlc.narg('owner_user_id'))
ORDER BY me.embedding <=> sqlc.arg('embedding')
LIMIT sqlc.arg('limit');

-- name: HybridSearchMemories :many
-- Full hybrid BM25 + vector search for memories using CTE-based approach.
-- Combines BM25 and vector results with FULL OUTER JOIN.
-- Weights: BM25 * 0.4 + vector * 0.6.
WITH
bm25_hits AS (
    SELECT m.id, paradedb.score(m.id)::float8 AS bm25_score
    FROM memories m
    WHERE m.source = 'manual_save'
      AND (sqlc.narg('owner_user_id')::text IS NULL OR m.owner_user_id = sqlc.narg('owner_user_id'))
      AND m.content @@@ paradedb.parse(sqlc.arg('query_text'))
    ORDER BY paradedb.score(m.id) DESC
    LIMIT sqlc.arg('result_limit')
),
vector_hits AS (
    SELECT me.memory_id AS id, (1.0 - (me.embedding <=> sqlc.arg('query_embedding')))::float8 AS vector_score
    FROM memory_embeddings me
    JOIN memories m ON me.memory_id = m.id
    WHERE me.embedding IS NOT NULL
      AND m.source = 'manual_save'
      AND (sqlc.narg('owner_user_id')::text IS NULL OR m.owner_user_id = sqlc.narg('owner_user_id'))
    ORDER BY me.embedding <=> sqlc.arg('query_embedding')
    LIMIT sqlc.arg('result_limit')
)
SELECT
    COALESCE(b.id, v.id) AS id,
    (COALESCE(b.bm25_score, 0.0) * 0.4 + COALESCE(v.vector_score, 0.0) * 0.6)::float8 AS combined_score,
    COALESCE(b.bm25_score, 0.0)::float8 AS bm25_score,
    COALESCE(v.vector_score, 0.0)::float8 AS vector_score
FROM bm25_hits b
FULL OUTER JOIN vector_hits v ON b.id = v.id
ORDER BY combined_score DESC
LIMIT sqlc.arg('result_limit');

-- name: InsertMemoryEmbedding :exec
INSERT INTO memory_embeddings (id, memory_id, embedding, model)
VALUES ($1, $2, $3, $4);

-- name: DeleteMemoryEmbedding :exec
DELETE FROM memory_embeddings WHERE memory_id = $1;
