-- Reverse 012_cascade_fks: Restore original FK constraints.

-- #90: Remove CASCADE from observations and session_summaries.
ALTER TABLE observations DROP CONSTRAINT IF EXISTS observations_session_id_fkey;
ALTER TABLE observations ADD CONSTRAINT observations_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id);

ALTER TABLE session_summaries DROP CONSTRAINT IF EXISTS session_summaries_session_id_fkey;
ALTER TABLE session_summaries ADD CONSTRAINT session_summaries_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES sessions(id);

-- #113: Restore ON DELETE CASCADE on sessions.user_id (migration 010 original).
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_user_id_fkey;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
