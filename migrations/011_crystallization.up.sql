-- 011_crystallization: Create crystals, insights, and procedural_memories tables.
-- These tables support the agentmemory v2 crystallization pipeline:
--   crystals        — compressed action chain digests (from Crystallize)
--   insights        — synthesized higher-order observations (from Reflect)
--   procedural_memories — stored procedural knowledge (from Consolidate)

CREATE TABLE IF NOT EXISTS crystals (
    id TEXT PRIMARY KEY,
    action_ids TEXT[] NOT NULL,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    narrative TEXT NOT NULL,
    files TEXT[],
    outcome TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS insights (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    confidence FLOAT NOT NULL DEFAULT 0.3,
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS procedural_memories (
    id TEXT PRIMARY KEY,
    content TEXT NOT NULL,
    trigger TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
