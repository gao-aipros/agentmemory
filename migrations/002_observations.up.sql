-- AgentMemory v2 — Observations table
-- Stores raw agent session events (observations) captured via hooks.

CREATE TABLE observations (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    owner_type TEXT NOT NULL DEFAULT 'user',
    owner_user_id TEXT,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private' CHECK (visibility = 'private'),
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    narrative TEXT NOT NULL,
    facts TEXT,
    concepts TEXT[],
    files TEXT[],
    importance FLOAT NOT NULL DEFAULT 0.5 CHECK (importance >= 0 AND importance <= 1),
    timestamp TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_observations_session ON observations(session_id);
CREATE INDEX idx_observations_type ON observations(type);
CREATE INDEX idx_observations_timestamp ON observations(timestamp);
CREATE INDEX idx_observations_importance ON observations(importance);
CREATE INDEX idx_observations_concepts ON observations USING GIN(concepts);
CREATE INDEX idx_observations_files ON observations USING GIN(files);
