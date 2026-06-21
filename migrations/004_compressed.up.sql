-- AgentMemory v2 — Compressed Observations
-- Stores LLM-compressed summaries of raw observations for efficient retrieval.

CREATE TABLE compressed_observations (
    id TEXT PRIMARY KEY,
    observation_ids TEXT[] NOT NULL,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    compressed_text TEXT NOT NULL,
    concepts TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE compressed_embeddings (
    compressed_id TEXT PRIMARY KEY REFERENCES compressed_observations(id) ON DELETE CASCADE,
    embedding vector(1536),
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
