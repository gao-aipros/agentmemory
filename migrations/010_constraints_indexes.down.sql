-- Reverse 010_constraints_indexes: remove FKs, UNIQUE, and indexes.

-- Drop indexes first, then constraints, then recreate original state.

-- #59: drop teams.owner_id index
DROP INDEX IF EXISTS idx_teams_owner_id;

-- #58: drop lessons.team_id index
DROP INDEX IF EXISTS idx_lessons_team_id;

-- #42: drop UNIQUE on team_members
DROP INDEX IF EXISTS uq_team_members_team_user;

-- #41: drop composite indexes on observations
DROP INDEX IF EXISTS idx_observations_owner_user_timestamp;
DROP INDEX IF EXISTS idx_observations_session_type;

-- #40: drop partial embedding index, recreate original full index.
-- Restore the original migration 008 index (IVFFlat) regardless of which
-- name was in effect when 010 was applied.
DROP INDEX IF EXISTS idx_obs_emb_hnsw_ada002;
DROP INDEX IF EXISTS idx_obs_emb_ivfflat_ada002;
CREATE INDEX IF NOT EXISTS idx_obs_emb_ivfflat
    ON observation_embeddings USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

-- #39: drop memories FK
ALTER TABLE memories DROP CONSTRAINT IF EXISTS fk_memories_owner_user_id;

-- #38: drop observations FK
ALTER TABLE observations DROP CONSTRAINT IF EXISTS fk_observations_owner_user_id;

-- #7: revert sessions FK to no CASCADE
DROP INDEX IF EXISTS idx_sessions_user_id;
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
