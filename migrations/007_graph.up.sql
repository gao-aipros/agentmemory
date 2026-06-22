-- AgentMemory v2 — Graph Schema for Knowledge Graph Traversal
-- graph_nodes tracks entities in the knowledge graph (observations, memories, concepts).
-- graph_edges connects nodes with typed relationships and weights.

CREATE TABLE graph_nodes (
    id TEXT PRIMARY KEY,
    node_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    label TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE graph_edges (
    id TEXT PRIMARY KEY,
    from_node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    to_node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    edge_type TEXT NOT NULL,
    weight FLOAT NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_graph_nodes_type ON graph_nodes(node_type);
CREATE INDEX idx_graph_nodes_entity ON graph_nodes(entity_id);
CREATE INDEX idx_graph_edges_from ON graph_edges(from_node_id);
CREATE INDEX idx_graph_edges_to ON graph_edges(to_node_id);

-- BM25 full-text search index for hybrid search over observations
-- ParadeDB bm25 indexes on id, title, narrative, and facts columns.
CREATE INDEX idx_observations_bm25 ON observations
USING bm25 (id, title, narrative, facts)
WITH (key_field='id');

-- PL/pgSQL wrapper function for BM25 search.
-- Hides the ParadeDB @@? operator from sqlc so queries pass through transparently.
-- owner_user_id filters results to a single tenant (cross-tenant isolation).
-- When owner_user_id is NULL, no user filter is applied (admin use only).
CREATE OR REPLACE FUNCTION bm25_search(query_text text, result_limit int, owner_user_id text)
RETURNS TABLE(id text, bm25_score float8) AS $$
BEGIN
    RETURN QUERY
    SELECT observations.id, paradedb.score(observations.id)::float8
    FROM observations
    WHERE observations @@@ paradedb.parse(query_text)
      AND (bm25_search.owner_user_id IS NULL OR observations.owner_user_id = bm25_search.owner_user_id)
    ORDER BY paradedb.score(observations.id) DESC
    LIMIT result_limit;
END;
$$ LANGUAGE plpgsql STABLE;

-- PL/pgSQL wrapper for full hybrid BM25 + vector search.
-- Combines both search streams via FULL OUTER JOIN and applies weights.
-- owner_user_id filters results to a single tenant (cross-tenant isolation).
-- When owner_user_id is NULL, no user filter is applied (admin use only).
CREATE OR REPLACE FUNCTION hybrid_search(
    query_text text,
    query_embedding vector,
    result_limit int,
    owner_user_id text
)
RETURNS TABLE(id text, combined_score float8, bm25_score float8, vector_score float8) AS $$
BEGIN
    RETURN QUERY
    WITH
    bm25_hits AS (
        SELECT b.id, b.bm25_score
        FROM bm25_search(query_text, result_limit, owner_user_id) b
    ),
    vector_hits AS (
        SELECT oe.observation_id AS id, (1.0 - (oe.embedding <=> query_embedding))::float8 AS vector_score
        FROM observation_embeddings oe
        JOIN observations o ON oe.observation_id = o.id
        WHERE oe.embedding IS NOT NULL
          AND (hybrid_search.owner_user_id IS NULL OR o.owner_user_id = hybrid_search.owner_user_id)
        ORDER BY oe.embedding <=> query_embedding
        LIMIT result_limit
    )
    SELECT
        COALESCE(b.id, v.id) AS id,
        (COALESCE(b.bm25_score, 0.0) * 0.4 + COALESCE(v.vector_score, 0.0) * 0.6)::float8 AS combined_score,
        COALESCE(b.bm25_score, 0.0)::float8 AS bm25_score,
        COALESCE(v.vector_score, 0.0)::float8 AS vector_score
    FROM bm25_hits b
    FULL OUTER JOIN vector_hits v ON b.id = v.id
    ORDER BY combined_score DESC
    LIMIT result_limit;
END;
$$ LANGUAGE plpgsql STABLE;
