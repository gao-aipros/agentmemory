-- AgentMemory v2 — Observation Embeddings
-- Stores vector embeddings for semantic search over observations.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE observation_embeddings (
    observation_id TEXT PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
    embedding vector(1536),
    model TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
