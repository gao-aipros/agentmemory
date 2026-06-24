-- AgentMemory v2 — Scheduler Pipeline Schema
-- Adds columns needed by the multi-tier scheduler for batch compression,
-- summarization, consolidation, and reflection.

-- ===========================================================================
-- Observations
-- ===========================================================================
ALTER TABLE observations ADD COLUMN IF NOT EXISTS compressed_at TIMESTAMPTZ;

-- ===========================================================================
-- Session Summaries
-- ===========================================================================
ALTER TABLE session_summaries ADD COLUMN IF NOT EXISTS is_full BOOLEAN NOT NULL DEFAULT false;
