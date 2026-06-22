-- Reverse 009_owner_columns: remove owner columns and their indexes.

-- Drop indexes first, then columns.

-- compressed_observations
DROP INDEX IF EXISTS idx_compressed_observations_owner_user_id;
DROP INDEX IF EXISTS idx_compressed_observations_owner_team_id;
ALTER TABLE compressed_observations
    DROP COLUMN IF EXISTS owner_type,
    DROP COLUMN IF EXISTS owner_user_id,
    DROP COLUMN IF EXISTS owner_team_id;

-- session_summaries
DROP INDEX IF EXISTS idx_session_summaries_owner_user_id;
DROP INDEX IF EXISTS idx_session_summaries_owner_team_id;
ALTER TABLE session_summaries
    DROP COLUMN IF EXISTS owner_type,
    DROP COLUMN IF EXISTS owner_user_id,
    DROP COLUMN IF EXISTS owner_team_id;

-- lessons
DROP INDEX IF EXISTS idx_lessons_owner_user_id;
ALTER TABLE lessons
    DROP COLUMN IF EXISTS owner_user_id;
