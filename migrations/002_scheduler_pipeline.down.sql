-- AgentMemory v2 — Scheduler Pipeline Schema (rollback)

ALTER TABLE observations DROP COLUMN IF EXISTS compressed_at;
ALTER TABLE session_summaries DROP COLUMN IF EXISTS is_full;
