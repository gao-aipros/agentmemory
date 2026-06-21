-- ============ 用户与认证 ============

CREATE TABLE users (
    id              TEXT PRIMARY KEY,
    email           TEXT UNIQUE NOT NULL,
    password_hash   TEXT NOT NULL,
    name            TEXT NOT NULL,
    totp_secret     TEXT,
    totp_enabled    BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label         TEXT NOT NULL,
    key_hash      TEXT UNIQUE NOT NULL,
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ
);

-- ============ 团队 ============

CREATE TABLE teams (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    owner_id            TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    default_visibility  TEXT NOT NULL DEFAULT 'choice',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id     TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role        TEXT NOT NULL DEFAULT 'member',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

-- ============ 项目与会话 ============

CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    signals     TEXT[],
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id                TEXT PRIMARY KEY,
    project_id        TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id           TEXT REFERENCES users(id) ON DELETE SET NULL,
    cwd               TEXT NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ,
    status            TEXT NOT NULL DEFAULT 'active',
    observation_count INT NOT NULL DEFAULT 0,
    model             TEXT,
    first_prompt      TEXT,
    agent_id          TEXT
);

CREATE INDEX idx_sessions_user        ON sessions (user_id);
CREATE INDEX idx_sessions_project     ON sessions (project_id);
CREATE INDEX idx_sessions_proj_user   ON sessions (project_id, user_id, started_at DESC);
CREATE INDEX idx_sessions_proj_start  ON sessions (project_id, started_at DESC);

-- ============ Observation（单表 raw + compressed） ============

CREATE TABLE observations (
    id              TEXT NOT NULL,
    session_id      TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    timestamp       TIMESTAMPTZ NOT NULL,
    hook_type       TEXT NOT NULL,
    tool_name           TEXT,
    tool_input          JSONB,
    tool_output         JSONB,
    user_prompt         TEXT,
    assistant_response  TEXT,
    raw_payload         JSONB,
    image_path          TEXT,
    agent_id            TEXT,
    type            TEXT,
    title           TEXT,
    subtitle        TEXT,
    narrative       TEXT,
    facts           TEXT[],
    concepts        TEXT[],
    files           TEXT[],
    importance      INT,
    confidence      REAL,
    modality        TEXT,
    embedding_model TEXT,
    user_id         TEXT REFERENCES users(id) ON DELETE SET NULL,
    visibility      TEXT NOT NULL DEFAULT 'private',
    PRIMARY KEY (id)
);

CREATE INDEX idx_obs_bm25 ON observations USING bm25 (id, title, narrative, facts) WITH (key_field = 'id');
CREATE INDEX idx_obs_timestamp     ON observations (timestamp);
CREATE INDEX idx_obs_type          ON observations (type);
CREATE INDEX idx_obs_importance    ON observations (importance);
CREATE INDEX idx_obs_concepts      ON observations USING gin (concepts);
CREATE INDEX idx_obs_files         ON observations USING gin (files);
CREATE INDEX idx_obs_session_ts    ON observations (session_id, timestamp DESC);

-- ============ Observation Embedding（多 model） ============

CREATE TABLE observation_embeddings (
    observation_id  TEXT NOT NULL REFERENCES observations(id) ON DELETE CASCADE,
    model           TEXT NOT NULL,
    dimension       INT NOT NULL,
    embedding       VECTOR,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (observation_id, model)
);

CREATE INDEX idx_obs_emb_hnsv  ON observation_embeddings USING hnsw (embedding vector_cosine_ops);
CREATE INDEX idx_obs_emb_model ON observation_embeddings (model);

-- ============ Session Summary ============

CREATE TABLE session_summaries (
    session_id       TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    project_id       TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title            TEXT NOT NULL,
    narrative        TEXT NOT NULL,
    key_decisions    TEXT[],
    files_modified   TEXT[],
    concepts         TEXT[],
    observation_count INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_summaries_project ON session_summaries (project_id);

-- ============ Memory ============

CREATE TABLE memories (
    id                      TEXT PRIMARY KEY,
    user_id                 TEXT REFERENCES users(id) ON DELETE CASCADE,
    team_id                 TEXT REFERENCES teams(id) ON DELETE CASCADE,
    owner_type              TEXT NOT NULL DEFAULT 'user',
    visibility              TEXT NOT NULL DEFAULT 'private',
    type                    TEXT NOT NULL,
    title                   TEXT NOT NULL,
    content                 TEXT NOT NULL,
    concepts                TEXT[],
    files                   TEXT[],
    session_ids             TEXT[],
    source_observation_ids  TEXT[],
    supersedes              TEXT[],
    strength                REAL NOT NULL DEFAULT 1.0,
    version                 INT NOT NULL DEFAULT 1,
    parent_id               TEXT REFERENCES memories(id) ON DELETE SET NULL,
    is_latest               BOOLEAN NOT NULL DEFAULT true,
    expires_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_memory_owner CHECK (user_id IS NOT NULL OR team_id IS NOT NULL)
);

CREATE INDEX idx_memories_user          ON memories (user_id);
CREATE INDEX idx_memories_team          ON memories (team_id);
CREATE INDEX idx_memories_type          ON memories (type);
CREATE INDEX idx_memories_latest        ON memories (is_latest);
CREATE INDEX idx_memories_concepts      ON memories USING gin (concepts);
CREATE INDEX idx_memories_type_latest   ON memories (type, is_latest, updated_at DESC);

-- ============ 知识图谱 ============

CREATE TABLE graph_nodes (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    name            TEXT NOT NULL,
    properties      JSONB,
    source_obs_ids  TEXT[],
    stale           BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_graph_nodes_type ON graph_nodes (type);
CREATE INDEX idx_graph_nodes_name ON graph_nodes (name);

CREATE TABLE graph_edges (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    source_node_id  TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    target_node_id  TEXT NOT NULL REFERENCES graph_nodes(id) ON DELETE CASCADE,
    weight          REAL NOT NULL DEFAULT 1.0,
    source_obs_ids  TEXT[],
    stale           BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    tcommit         TIMESTAMPTZ,
    tvalid          TIMESTAMPTZ,
    tvalid_end      TIMESTAMPTZ,
    superseded_by   TEXT,
    is_latest       BOOLEAN NOT NULL DEFAULT true
);

CREATE INDEX idx_edges_source ON graph_edges (source_node_id);
CREATE INDEX idx_edges_target ON graph_edges (target_node_id);

-- ============ Consolidation 产出 ============

CREATE TABLE semantic_memories (
    id                  TEXT PRIMARY KEY,
    fact                TEXT NOT NULL,
    confidence          REAL NOT NULL DEFAULT 0.5,
    source_session_ids  TEXT[],
    source_memory_ids   TEXT[],
    access_count        INT NOT NULL DEFAULT 0,
    last_accessed_at    TIMESTAMPTZ,
    strength            REAL NOT NULL DEFAULT 0.5,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE procedural_memories (
    id                    TEXT PRIMARY KEY,
    name                  TEXT NOT NULL,
    steps                 TEXT[] NOT NULL,
    trigger_condition     TEXT,
    expected_outcome      TEXT,
    frequency             INT NOT NULL DEFAULT 1,
    source_session_ids    TEXT[],
    source_observation_ids TEXT[],
    tags                  TEXT[],
    concepts              TEXT[],
    strength              REAL NOT NULL DEFAULT 0.5,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE insights (
    id                      TEXT PRIMARY KEY,
    title                   TEXT NOT NULL,
    content                 TEXT NOT NULL,
    confidence              REAL NOT NULL DEFAULT 0.5,
    reinforcements          INT NOT NULL DEFAULT 0,
    last_reinforced_at      TIMESTAMPTZ,
    last_decayed_at         TIMESTAMPTZ,
    decay_rate              REAL NOT NULL DEFAULT 0.05,
    source_concept_cluster  TEXT[],
    source_memory_ids       TEXT[],
    source_lesson_ids       TEXT[],
    source_crystal_ids      TEXT[],
    project_id              TEXT REFERENCES projects(id) ON DELETE SET NULL,
    tags                    TEXT[],
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted                 BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_insights_project ON insights (project_id);

-- ============ Lessons & Crystals ============

CREATE TABLE lessons (
    id                  TEXT PRIMARY KEY,
    user_id             TEXT REFERENCES users(id) ON DELETE CASCADE,
    team_id             TEXT REFERENCES teams(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,
    context             TEXT,
    confidence          REAL NOT NULL DEFAULT 0.5,
    reinforcements      INT NOT NULL DEFAULT 0,
    last_reinforced_at  TIMESTAMPTZ,
    last_decayed_at     TIMESTAMPTZ,
    decay_rate          REAL NOT NULL DEFAULT 0.05,
    source              TEXT NOT NULL,
    source_ids          TEXT[],
    tags                TEXT[],
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted             BOOLEAN NOT NULL DEFAULT false
);

CREATE INDEX idx_lessons_bm25     ON lessons USING bm25 (id, content, context) WITH (key_field = 'id');
CREATE INDEX idx_lessons_user     ON lessons (user_id);
CREATE INDEX idx_lessons_team_conf ON lessons (team_id, confidence DESC) WHERE deleted = false;

CREATE TABLE crystals (
    id                TEXT PRIMARY KEY,
    session_id        TEXT REFERENCES sessions(id) ON DELETE SET NULL,
    narrative         TEXT NOT NULL,
    key_outcomes      TEXT[],
    files_affected    TEXT[],
    lessons           TEXT[],
    source_action_ids TEXT[],
    user_id           TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_crystals_session ON crystals (session_id);

-- ============ Actions / Orchestration ============

CREATE TABLE actions (
    id                    TEXT PRIMARY KEY,
    title                 TEXT NOT NULL,
    description           TEXT,
    status                TEXT NOT NULL DEFAULT 'pending',
    priority              INT NOT NULL DEFAULT 5,
    assigned_to           TEXT,
    user_id               TEXT REFERENCES users(id) ON DELETE SET NULL,
    project_id            TEXT REFERENCES projects(id) ON DELETE SET NULL,
    tags                  TEXT[],
    parent_id             TEXT REFERENCES actions(id) ON DELETE SET NULL,
    source_observation_ids TEXT[],
    source_memory_ids     TEXT[],
    sketch_id             TEXT,
    result                TEXT,
    crystallized_into     TEXT,
    metadata              JSONB,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_actions_status   ON actions (status);
CREATE INDEX idx_actions_project  ON actions (project_id);
CREATE INDEX idx_actions_user     ON actions (user_id);
CREATE INDEX idx_actions_frontier ON actions (priority DESC) WHERE status != 'done';

CREATE TABLE action_edges (
    id                TEXT PRIMARY KEY,
    type              TEXT NOT NULL,
    source_action_id  TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
    target_action_id  TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
    metadata          JSONB,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_action_edges_source ON action_edges (source_action_id);
CREATE INDEX idx_action_edges_target ON action_edges (target_action_id);

CREATE TABLE leases (
    id            TEXT PRIMARY KEY,
    action_id     TEXT NOT NULL REFERENCES actions(id) ON DELETE CASCADE,
    agent_id      TEXT NOT NULL,
    acquired_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    renewed_at    TIMESTAMPTZ,
    status        TEXT NOT NULL DEFAULT 'active'
);

CREATE INDEX idx_leases_action_agent ON leases (action_id, agent_id);
CREATE INDEX idx_leases_expiry       ON leases (expires_at) WHERE status = 'active';

-- ============ 辅助表 ============

CREATE TABLE commits (
    sha         TEXT PRIMARY KEY,
    branch      TEXT,
    repo        TEXT,
    message     TEXT,
    author      TEXT,
    authored_at TIMESTAMPTZ,
    files       TEXT[],
    session_ids TEXT[],
    linked_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_commits_sessions ON commits USING gin (session_ids);

CREATE TABLE slots (
    label       TEXT NOT NULL,
    project_id  TEXT REFERENCES projects(id) ON DELETE CASCADE,
    scope       TEXT NOT NULL DEFAULT 'project',
    content     TEXT NOT NULL DEFAULT '',
    size_limit  INT NOT NULL DEFAULT 2000,
    description TEXT NOT NULL DEFAULT '',
    pinned      BOOLEAN NOT NULL DEFAULT true,
    read_only   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, project_id, label)
);

CREATE UNIQUE INDEX idx_slots_global_label ON slots (label) WHERE scope = 'global';

CREATE TABLE profiles (
    project_id        TEXT PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    top_concepts      JSONB NOT NULL DEFAULT '[]',
    top_files         JSONB NOT NULL DEFAULT '[]',
    conventions       TEXT[],
    common_errors     TEXT[],
    recent_activity   TEXT[],
    session_count     INT NOT NULL DEFAULT 0,
    total_observations INT NOT NULL DEFAULT 0,
    summary           TEXT
);

CREATE TABLE recent_searches (
    session_id  TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    query       TEXT NOT NULL,
    result_ids  TEXT[],
    at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id)
);

CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,
    timestamp   TIMESTAMPTZ NOT NULL DEFAULT now(),
    operation   TEXT NOT NULL,
    function_id TEXT NOT NULL,
    user_id     TEXT,
    target_ids  TEXT[],
    details     JSONB
);

CREATE INDEX idx_audit_timestamp ON audit_log (timestamp);
CREATE INDEX idx_audit_operation ON audit_log (operation);

-- ============
-- 25 tables, 41 indexes
-- ============
