-- 008_performance_indexes: Add indexes for common query patterns identified in T182 review.
-- These indexes address N+1 query patterns and missing indexes on frequently filtered columns.

-- Vector index for similarity search (prevents full table scan on vector search).
-- Uses HNSW for high-performance approximate nearest neighbor search.
CREATE INDEX IF NOT EXISTS idx_obs_emb_hnsw
    ON observation_embeddings USING hnsw (embedding vector_cosine_ops);

-- Index on owner columns for visibility-filtered queries.
CREATE INDEX IF NOT EXISTS idx_observations_owner_user_id ON observations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_observations_owner_team_id ON observations(owner_team_id);
CREATE INDEX IF NOT EXISTS idx_memories_owner_user_id ON memories(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_memories_owner_team_id ON memories(owner_team_id);

-- Index on created_at for ListRecentObservations ORDER BY created_at DESC.
CREATE INDEX IF NOT EXISTS idx_observations_created_at ON observations(created_at DESC);

-- Index on session_id for compressed_observations lookups.
CREATE INDEX IF NOT EXISTS idx_compressed_observations_session_id ON compressed_observations(session_id);

-- Index on lesson_id for lesson_reinforcements lookups and cascade operations.
CREATE INDEX IF NOT EXISTS idx_lesson_reinforcements_lesson_id ON lesson_reinforcements(lesson_id);
