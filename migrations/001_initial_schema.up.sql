-- AgentMemory v2 — Initial Schema (consolidated from migrations 001–012)
-- Since the system is pre-production, all incremental migrations are merged
-- into this single canonical schema. Every table, constraint, index, FK,
-- and function is defined in its final form.

-- ===========================================================================
-- Extensions
-- ===========================================================================
CREATE EXTENSION IF NOT EXISTS vector;

-- ===========================================================================
-- Users & Authentication
-- ===========================================================================
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    totp_secret TEXT,
    totp_enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);

-- ===========================================================================
-- Teams
-- ===========================================================================
CREATE TABLE IF NOT EXISTS teams (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    default_visibility TEXT NOT NULL DEFAULT 'member_choice',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS team_members (
    id TEXT PRIMARY KEY,
    team_id TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Sessions
-- ===========================================================================
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    team_id TEXT REFERENCES teams(id),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'active'
);

-- ===========================================================================
-- Observations
-- ===========================================================================
CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    owner_type TEXT NOT NULL DEFAULT 'user',
    owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    narrative TEXT NOT NULL,
    facts TEXT,
    concepts TEXT[],
    files TEXT[],
    importance FLOAT NOT NULL DEFAULT 0.5 CHECK (importance >= 0 AND importance <= 1),
    timestamp TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Observation Embeddings (vector search)
-- ===========================================================================
CREATE TABLE IF NOT EXISTS observation_embeddings (
    observation_id TEXT PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
    embedding vector(1536),
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Compressed Observations
-- ===========================================================================
CREATE TABLE IF NOT EXISTS compressed_observations (
    id TEXT PRIMARY KEY,
    observation_ids TEXT[] NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    owner_type TEXT NOT NULL DEFAULT 'user',
    owner_user_id TEXT,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    compressed_text TEXT NOT NULL,
    concepts TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS compressed_embeddings (
    compressed_id TEXT PRIMARY KEY REFERENCES compressed_observations(id) ON DELETE CASCADE,
    embedding vector(1536),
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Session Summaries
-- ===========================================================================
CREATE TABLE IF NOT EXISTS session_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE UNIQUE,
    owner_type TEXT NOT NULL DEFAULT 'user',
    owner_user_id TEXT,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    summary_text TEXT NOT NULL,
    concepts TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Memories & Lessons
-- ===========================================================================
CREATE TABLE IF NOT EXISTS memories (
    id TEXT PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    content TEXT NOT NULL,
    concepts TEXT[],
    source TEXT NOT NULL DEFAULT 'consolidation',
    confidence FLOAT NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lessons (
    id TEXT PRIMARY KEY,
    team_id TEXT REFERENCES teams(id),
    owner_user_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'team' CHECK (visibility = 'team'),
    content TEXT NOT NULL,
    context TEXT,
    confidence FLOAT NOT NULL DEFAULT 0.5,
    source TEXT NOT NULL DEFAULT 'consolidation',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_reinforced_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS lesson_reinforcements (
    id TEXT PRIMARY KEY,
    lesson_id TEXT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    observation_id TEXT,
    confidence_delta FLOAT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Knowledge Graph
-- ===========================================================================
CREATE TABLE IF NOT EXISTS graph_nodes (
    id TEXT PRIMARY KEY,
    node_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    label TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS graph_edges (
    id TEXT PRIMARY KEY,
    from_node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    to_node_id TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    edge_type TEXT NOT NULL,
    weight FLOAT NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Crystallization
-- ===========================================================================
CREATE TABLE IF NOT EXISTS crystals (
    id TEXT PRIMARY KEY,
    action_ids TEXT[] NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private' CONSTRAINT chk_crystals_visibility CHECK (visibility = 'private'),
    narrative TEXT NOT NULL,
    files TEXT[],
    outcome TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insights (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 0.3,
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS procedural_memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    trigger TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ===========================================================================
-- Indexes
-- ===========================================================================

-- api_keys
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);

-- team_members
CREATE INDEX IF NOT EXISTS idx_team_members_team_id ON team_members(team_id);
CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_team_members_team_user ON team_members(team_id, user_id);

-- teams
CREATE INDEX IF NOT EXISTS idx_teams_owner_id ON teams(owner_id);

-- sessions
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_team_id ON sessions(team_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_user_status ON sessions(user_id, status);

-- observations
CREATE INDEX IF NOT EXISTS idx_observations_session ON observations(session_id);
CREATE INDEX IF NOT EXISTS idx_observations_type ON observations(type);
CREATE INDEX IF NOT EXISTS idx_observations_timestamp ON observations(timestamp);
CREATE INDEX IF NOT EXISTS idx_observations_importance ON observations(importance);
CREATE INDEX IF NOT EXISTS idx_observations_concepts ON observations USING GIN(concepts);
CREATE INDEX IF NOT EXISTS idx_observations_files ON observations USING GIN(files);
CREATE INDEX IF NOT EXISTS idx_observations_owner_user_id ON observations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_observations_owner_team_id ON observations(owner_team_id);
CREATE INDEX IF NOT EXISTS idx_observations_created_at ON observations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_observations_session_type ON observations(session_id, type);
CREATE INDEX IF NOT EXISTS idx_observations_owner_user_timestamp ON observations(owner_user_id, timestamp DESC);

-- observation_embeddings (HNSW approximate nearest neighbor, per-model partial)
CREATE INDEX IF NOT EXISTS idx_obs_emb_hnsw_ada002
    ON observation_embeddings USING hnsw (embedding vector_cosine_ops)
    WHERE model = 'text-embedding-ada-002';

-- compressed_observations
CREATE INDEX IF NOT EXISTS idx_compressed_observations_session_id ON compressed_observations(session_id);
CREATE INDEX IF NOT EXISTS idx_compressed_observations_owner_user_id ON compressed_observations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_compressed_observations_owner_team_id ON compressed_observations(owner_team_id);

-- session_summaries
CREATE INDEX IF NOT EXISTS idx_session_summaries_owner_user_id ON session_summaries(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_session_summaries_owner_team_id ON session_summaries(owner_team_id);

-- memories
CREATE INDEX IF NOT EXISTS idx_memories_owner_user_id ON memories(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_memories_owner_team_id ON memories(owner_team_id);

-- lessons
CREATE INDEX IF NOT EXISTS idx_lessons_team_id ON lessons(team_id);
CREATE INDEX IF NOT EXISTS idx_lessons_owner_user_id ON lessons(owner_user_id);

-- lesson_reinforcements
CREATE INDEX IF NOT EXISTS idx_lesson_reinforcements_lesson_id ON lesson_reinforcements(lesson_id);

-- graph
CREATE INDEX IF NOT EXISTS idx_graph_nodes_type ON graph_nodes(node_type);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_entity ON graph_nodes(entity_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_from ON graph_edges(from_node_id);
CREATE INDEX IF NOT EXISTS idx_graph_edges_to ON graph_edges(to_node_id);

-- ===========================================================================
-- BM25 Full-Text Search (ParadeDB)
-- ===========================================================================
CREATE INDEX IF NOT EXISTS idx_observations_bm25 ON observations
USING bm25 (id, title, narrative, facts)
WITH (key_field='id');

-- ===========================================================================
-- Hybrid Search Functions (BM25 + vector)
-- ===========================================================================
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
