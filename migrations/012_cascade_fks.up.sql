-- 012_cascade_fks: Add missing CASCADE paths through the observation/summary/session chain.
-- Fixes #90 and #113.

-- #90: observations.session_id and session_summaries.session_id need ON DELETE CASCADE
-- so deleting a session cascades through its summary to its observations.
ALTER TABLE observations DROP CONSTRAINT IF EXISTS observations_session_id_fkey;
ALTER TABLE observations ADD CONSTRAINT observations_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

ALTER TABLE session_summaries DROP CONSTRAINT IF EXISTS session_summaries_session_id_fkey;
ALTER TABLE session_summaries ADD CONSTRAINT session_summaries_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE;

-- #113: sessions.user_id uses ON DELETE CASCADE from migration 010, which blocks
-- deleting team owner users. Change to ON DELETE SET NULL so session history
-- is preserved when a user is deleted.
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
