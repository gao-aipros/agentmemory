-- 009_owner_columns: Add owner columns for cross-tenant isolation.
-- compressed_observations, session_summaries, and lessons currently lack
-- owner columns that observations and memories already have. This migration
-- brings them into alignment so all queryable tables support tenant scoping.

-- 1. compressed_observations — add full owner triplet
ALTER TABLE compressed_observations
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'user',
    ADD COLUMN owner_user_id TEXT,
    ADD COLUMN owner_team_id TEXT;
CREATE INDEX IF NOT EXISTS idx_compressed_observations_owner_user_id ON compressed_observations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_compressed_observations_owner_team_id ON compressed_observations(owner_team_id);

-- 2. session_summaries — add full owner triplet
ALTER TABLE session_summaries
    ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'user',
    ADD COLUMN owner_user_id TEXT,
    ADD COLUMN owner_team_id TEXT;
CREATE INDEX IF NOT EXISTS idx_session_summaries_owner_user_id ON session_summaries(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_session_summaries_owner_team_id ON session_summaries(owner_team_id);

-- 3. lessons — already has team_id; add owner_user_id for user-scoped filtering
ALTER TABLE lessons
    ADD COLUMN owner_user_id TEXT;
CREATE INDEX IF NOT EXISTS idx_lessons_owner_user_id ON lessons(owner_user_id);
