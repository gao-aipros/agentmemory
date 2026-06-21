-- AgentMemory v2 — Memories, Lessons, and Lesson Reinforcements
-- Stores extracted semantic memories, team lessons, and confidence tracking.

CREATE TABLE memories (
    id TEXT PRIMARY KEY,
    owner_type TEXT NOT NULL,
    owner_user_id TEXT,
    owner_team_id TEXT,
    visibility TEXT NOT NULL DEFAULT 'private',
    content TEXT NOT NULL,
    concepts TEXT[],
    source TEXT NOT NULL DEFAULT 'consolidation',
    confidence FLOAT NOT NULL DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE lessons (
    id TEXT PRIMARY KEY,
    team_id TEXT REFERENCES teams(id),
    visibility TEXT NOT NULL DEFAULT 'team' CHECK (visibility = 'team'),
    content TEXT NOT NULL,
    context TEXT,
    confidence FLOAT NOT NULL DEFAULT 0.5,
    source TEXT NOT NULL DEFAULT 'consolidation',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_reinforced_at TIMESTAMPTZ
);

CREATE TABLE lesson_reinforcements (
    id TEXT PRIMARY KEY,
    lesson_id TEXT NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
    observation_id TEXT,
    confidence_delta FLOAT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
