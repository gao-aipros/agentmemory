-- 010_constraints_indexes: Add missing FKs, UNIQUE constraints, and performance indexes.
-- Fixes #7, #38, #39, #40, #41, #42, #58, #59.

-- #7: sessions.user_id missing ON DELETE CASCADE — user deletion breaks
DROP INDEX IF EXISTS idx_sessions_user_id;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

-- #38: observations.owner_user_id missing FK
ALTER TABLE observations DROP CONSTRAINT IF EXISTS fk_observations_owner_user_id;
ALTER TABLE observations ADD CONSTRAINT fk_observations_owner_user_id
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- #39: memories.owner_user_id missing FK
ALTER TABLE memories DROP CONSTRAINT IF EXISTS fk_memories_owner_user_id;
ALTER TABLE memories ADD CONSTRAINT fk_memories_owner_user_id
    FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL;

-- #40: Replace model-unaware IVFFlat index with per-model partial indexes
DROP INDEX IF EXISTS idx_obs_emb_ivfflat;
CREATE INDEX IF NOT EXISTS idx_obs_emb_ivfflat_ada002
    ON observation_embeddings USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100)
    WHERE model = 'text-embedding-ada-002';

-- #41: Composite B-tree indexes covering common query patterns
CREATE INDEX IF NOT EXISTS idx_observations_session_type
    ON observations(session_id, type);
CREATE INDEX IF NOT EXISTS idx_observations_owner_user_timestamp
    ON observations(owner_user_id, timestamp DESC);

-- #42: Prevent duplicate team memberships
CREATE UNIQUE INDEX IF NOT EXISTS uq_team_members_team_user
    ON team_members(team_id, user_id);

-- #58: Index on lessons.team_id for team-scoped lesson queries
CREATE INDEX IF NOT EXISTS idx_lessons_team_id ON lessons(team_id);

-- #59: Index on teams.owner_id for owner-scoped team listing
CREATE INDEX IF NOT EXISTS idx_teams_owner_id ON teams(owner_id);
