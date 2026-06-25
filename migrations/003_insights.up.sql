-- AgentMemory v2 — Insights & Reflection Schema
-- Replaces the minimal insights table from 001_initial_schema with the full
-- decay/reinforcement model. Adds the reflected column to memories.

-- ===========================================================================
-- Insights (replaces the minimal schema from 001_initial_schema)
-- ===========================================================================
DROP TABLE IF EXISTS insights CASCADE;

CREATE TABLE insights (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 0.3 CHECK (confidence >= 0.0 AND confidence <= 1.0),
    reinforcement_count INTEGER NOT NULL DEFAULT 0,
    source_concept_cluster TEXT[] NOT NULL DEFAULT '{}',
    source_memory_ids TEXT[] NOT NULL DEFAULT '{}',
    source_lesson_ids TEXT[] NOT NULL DEFAULT '{}',
    project TEXT,
    tags TEXT[] DEFAULT '{}',
    decay_rate FLOAT NOT NULL DEFAULT 0.05,
    last_reinforced_at TIMESTAMPTZ,
    last_decayed_at TIMESTAMPTZ,
    deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_insights_confidence ON insights (confidence DESC) WHERE deleted = false;
CREATE INDEX IF NOT EXISTS idx_insights_project ON insights (project) WHERE deleted = false AND project IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_insights_tags ON insights USING GIN (tags) WHERE deleted = false;
CREATE INDEX IF NOT EXISTS idx_insights_fts ON insights USING GIN (to_tsvector('english', title || ' ' || content)) WHERE deleted = false;

-- ===========================================================================
-- Memories
-- ===========================================================================
ALTER TABLE memories ADD COLUMN IF NOT EXISTS deleted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE memories ADD COLUMN IF NOT EXISTS reflected BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS idx_memories_reflected ON memories (reflected) WHERE reflected = false AND deleted = false;
