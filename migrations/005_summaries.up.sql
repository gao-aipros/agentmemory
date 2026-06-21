-- AgentMemory v2 — Session Summaries
-- Stores LLM-generated summaries of entire agent sessions.

CREATE TABLE session_summaries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) UNIQUE,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    summary_text TEXT NOT NULL,
    concepts TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
