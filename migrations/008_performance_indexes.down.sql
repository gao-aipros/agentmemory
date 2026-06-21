-- 008_performance_indexes: Remove performance indexes added in T182.

DROP INDEX IF EXISTS idx_obs_emb_ivfflat;
DROP INDEX IF EXISTS idx_observations_owner_user_id;
DROP INDEX IF EXISTS idx_observations_owner_team_id;
DROP INDEX IF EXISTS idx_memories_owner_user_id;
DROP INDEX IF EXISTS idx_memories_owner_team_id;
DROP INDEX IF EXISTS idx_observations_created_at;
DROP INDEX IF EXISTS idx_compressed_observations_session_id;
DROP INDEX IF EXISTS idx_lesson_reinforcements_lesson_id;
