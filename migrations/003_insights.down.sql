-- AgentMemory v2 — Insights & Reflection Schema (rollback)

DROP INDEX IF EXISTS idx_insights_fts;
DROP INDEX IF EXISTS idx_insights_tags;
DROP INDEX IF EXISTS idx_insights_project;
DROP INDEX IF EXISTS idx_insights_confidence;
DROP TABLE IF EXISTS insights CASCADE;
DROP INDEX IF EXISTS idx_memories_reflected;
ALTER TABLE memories DROP COLUMN IF EXISTS reflected;
ALTER TABLE memories DROP COLUMN IF EXISTS deleted;
