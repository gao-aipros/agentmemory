-- name: Bm25Search :many
-- BM25 full-text search via the bm25_search wrapper function.
-- Explicit casts ensure sqlc generates proper Go types.
SELECT s.id::text AS id, s.bm25_score::float8 AS bm25_score
FROM bm25_search($1, $2) s;

-- name: VectorSearch :many
-- Cosine similarity search over observation embeddings using pgvector.
-- Returns observation IDs with similarity scores (1 - cosine_distance).
SELECT
    oe.observation_id AS id,
    (1.0 - (oe.embedding <=> $1))::float8 AS vector_score
FROM observation_embeddings oe
WHERE oe.embedding IS NOT NULL
ORDER BY oe.embedding <=> $1
LIMIT $2;

-- name: GraphTraversal :many
-- Recursive graph traversal from seed observation IDs.
-- Expands up to 2 hops through graph_edges, accumulating edge weights.
-- Returns observation IDs discovered via graph with traversal scores.
WITH RECURSIVE graph_traversal AS (
    SELECT
        gn.id AS node_id,
        gn.entity_id AS observation_id,
        0 AS depth,
        0.0::float AS graph_score
    FROM graph_nodes gn
    WHERE gn.entity_id = ANY($1::text[])

    UNION ALL

    SELECT
        gn.id,
        gn.entity_id,
        gt.depth + 1,
        gt.graph_score + ge.weight
    FROM graph_nodes gn
    JOIN graph_edges ge ON gn.id = ge.to_node_id
    JOIN graph_traversal gt ON ge.from_node_id = gt.node_id
    WHERE gt.depth < 2
)
SELECT DISTINCT observation_id AS id, MAX(graph_score)::float8 AS graph_score
FROM graph_traversal
WHERE observation_id IS NOT NULL
  AND observation_id != ALL($1::text[])
GROUP BY observation_id;

-- name: HybridSearch :many
-- Full hybrid BM25 + vector search via the hybrid_search wrapper function.
-- Combines both streams with FULL OUTER JOIN.
-- Weights: BM25 * 0.4 + vector * 0.6.
SELECT
    s.id::text AS id,
    s.combined_score::float8 AS combined_score,
    s.bm25_score::float8 AS bm25_score,
    s.vector_score::float8 AS vector_score
FROM hybrid_search($1, $2, $3) s;
