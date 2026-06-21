# Data Model: AgentMemory v2

**Feature**: AgentMemory v2 Platform Migration
**Date**: 2026-06-21
**Source**: `docs/specs/10-schema-ddl.sql`, `docs/specs/06-team-user.md`, `docs/specs/07-pipeline.md`

## Entity Relationship Diagram

```
┌──────────┐    ┌──────────────┐    ┌─────────────┐
│  users   │───<│  api_keys    │    │   teams     │
└────┬─────┘    └──────────────┘    └──────┬──────┘
     │                                     │
     │  ┌──────────────┐                   │
     ├─<│ team_members │>──────────────────┘
     │  └──────────────┘
     │
     │  ┌──────────────┐    ┌─────────────────────┐
     ├─<│  sessions    │───<│   observations       │
     │  └──────┬───────┘    └──────────┬──────────┘
     │         │                       │
     │         │         ┌─────────────┴──────────┐
     │         │         │  observation_embeddings │
     │         │         └────────────────────────┘
     │         │
     │         │  ┌───────────────────┐
     │         ├─<│ session_summaries  │
     │         │  └───────────────────┘
     │         │
     │         │  ┌───────────────────┐    ┌──────────────────┐
     │         └─<│ compressed_obs     │───<│ compressed_emb   │
     │            └───────────────────┘    └──────────────────┘
     │
     │  ┌──────────────────┐    ┌──────────────────┐
     ├─<│ memories          │    │   lessons        │
     │  └──────────────────┘    └────────┬─────────┘
     │                                   │
     │                      ┌────────────┴─────────┐
     │                      │ lesson_reinforcements │
     │                      └──────────────────────┘
     │
     │  ┌──────────────────┐
     ├─<│ crystals          │
     │  └──────────────────┘
     │
     │  ┌──────────────────┐    ┌──────────────────┐
     ├─<│ insights          │    │  graph_nodes     │
     │  └──────────────────┘    └────────┬─────────┘
     │                                   │
     │                      ┌────────────┴─────────┐
     │                      │   graph_edges         │
     │                      └──────────────────────┘
     │
     │  ┌──────────────────┐
     └─<│ procedural_mem    │
        └──────────────────┘
```

## Core Entities

### users

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| email | TEXT | UNIQUE NOT NULL | Login email |
| password_hash | TEXT | NOT NULL | bcrypt hash |
| name | TEXT | NOT NULL | Display name |
| totp_secret | TEXT | NULLABLE | TOTP shared secret |
| totp_enabled | BOOLEAN | NOT NULL DEFAULT false | TOTP active flag |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Account creation |

### api_keys

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| user_id | TEXT | FK → users(id) ON DELETE CASCADE | Owner |
| label | TEXT | NOT NULL | Human-readable name |
| key_hash | TEXT | UNIQUE NOT NULL | Hashed key for `ak_` prefix |
| last_used_at | TIMESTAMPTZ | NULLABLE | Last authentication |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |
| expires_at | TIMESTAMPTZ | NULLABLE | Expiry, NULL = never |

### teams

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| name | TEXT | NOT NULL | Team name |
| owner_id | TEXT | FK → users(id) ON DELETE CASCADE | Team owner |
| default_visibility | TEXT | NOT NULL DEFAULT 'member_choice' | Default mode |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### team_members

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| team_id | TEXT | FK → teams(id) ON DELETE CASCADE | Team |
| user_id | TEXT | FK → users(id) ON DELETE CASCADE | Member |
| joined_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Join time |

Exit/re-join pattern: DELETE row on leave. Full history on re-join (new row).

### sessions

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | Session UUID |
| user_id | TEXT | FK → users(id) | Session owner |
| team_id | TEXT | NULLABLE, FK → teams(id) | Team context |
| started_at | TIMESTAMPTZ | NOT NULL | Session start |
| ended_at | TIMESTAMPTZ | NULLABLE | Session end (NULL = active) |
| status | TEXT | NOT NULL DEFAULT 'active' | active / ended |

### observations

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | Globally unique UUID |
| session_id | TEXT | FK → sessions(id) | Session context |
| owner_type | TEXT | NOT NULL | 'user' or 'team' |
| owner_user_id | TEXT | NULLABLE, FK → users(id) | Owning user |
| owner_team_id | TEXT | NULLABLE, FK → teams(id) | Owning team |
| visibility | TEXT | NOT NULL DEFAULT 'private' CHECK (= 'private') | Always private |
| type | TEXT | NOT NULL | Hook event type |
| title | TEXT | NOT NULL | Short summary |
| narrative | TEXT | NOT NULL | Full description |
| facts | TEXT | NULLABLE | Extracted facts |
| concepts | TEXT[] | NULLABLE | Concept tags |
| files | TEXT[] | NULLABLE | Related file paths |
| importance | FLOAT | NOT NULL DEFAULT 0.5 | 0.0-1.0 |
| timestamp | TIMESTAMPTZ | NOT NULL | Event time |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | DB insertion |

**Indexes**:
- BM25: `USING bm25 (id, title, narrative, facts) WITH (key_field = 'id')`
- HNSW: `USING hnsw (embedding vector_cosine_ops)` via `observation_embeddings`
- B-tree: `(timestamp, type, importance, session_id, concepts, files)`

### observation_embeddings

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| observation_id | TEXT | PK, FK → observations(id) ON DELETE CASCADE | Parent |
| embedding | vector(N) | NOT NULL | Embedding vector (model-dependent dimension) |
| model | TEXT | NOT NULL | Embedding model name |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

Partial index per active model: `WHERE model = '<active_model>'`.

### compressed_observations

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| observation_ids | TEXT[] | NOT NULL | Source observation IDs |
| session_id | TEXT | FK → sessions(id) | Session context |
| visibility | TEXT | NOT NULL DEFAULT 'private' CHECK (= 'private') | Always private |
| compressed_text | TEXT | NOT NULL | LLM-compressed summary |
| concepts | TEXT[] | NULLABLE | Extracted concepts |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### compressed_embeddings

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| compressed_id | TEXT | PK, FK → compressed_observations(id) ON DELETE CASCADE | Parent |
| embedding | vector(N) | NOT NULL | Embedding vector |
| model | TEXT | NOT NULL | Embedding model name |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### session_summaries

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| session_id | TEXT | FK → sessions(id) UNIQUE | One summary per session |
| visibility | TEXT | NOT NULL DEFAULT 'private' CHECK (= 'private') | Always private |
| summary_text | TEXT | NOT NULL | Summarized content |
| concepts | TEXT[] | NULLABLE | Extracted concepts |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

**Rule**: Context injection ONLY — never search-indexed.

### memories (SemanticMemory)

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| owner_type | TEXT | NOT NULL | 'user' or 'team' |
| owner_user_id | TEXT | NULLABLE, FK → users(id) | Owning user |
| owner_team_id | TEXT | NULLABLE, FK → teams(id) | Owning team |
| visibility | TEXT | NOT NULL | 'private', 'team', or 'public' |
| content | TEXT | NOT NULL | Memory content |
| concepts | TEXT[] | NULLABLE | Concept tags |
| source | TEXT | NOT NULL | 'consolidation', 'manual_save', 'crystallize' |
| confidence | FLOAT | NOT NULL DEFAULT 0.5 | 0.0-1.0 |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### lessons

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| team_id | TEXT | FK → teams(id) | Always team visibility |
| visibility | TEXT | NOT NULL DEFAULT 'team' CHECK (= 'team') | Always team |
| content | TEXT | NOT NULL | Lesson content |
| context | TEXT | NULLABLE | When/where applicable |
| confidence | FLOAT | NOT NULL DEFAULT 0.5 | 0.0-1.0, strengthens/diminishes |
| source | TEXT | NOT NULL | 'crystallize', 'manual_save', 'consolidation' |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |
| last_reinforced_at | TIMESTAMPTZ | NULLABLE | Last strengthening |

### lesson_reinforcements

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| lesson_id | TEXT | FK → lessons(id) ON DELETE CASCADE | Lesson |
| observation_id | TEXT | NULLABLE | Source observation |
| confidence_delta | FLOAT | NOT NULL | Change applied |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | When reinforced |

### crystals

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| action_ids | TEXT[] | NOT NULL | Completed action IDs |
| visibility | TEXT | NOT NULL DEFAULT 'private' CHECK (= 'private') | Always private |
| narrative | TEXT | NOT NULL | Compressed action chain digest |
| files | TEXT[] | NULLABLE | Files affected |
| outcome | TEXT | NULLABLE | Result summary |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### insights

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| content | TEXT | NOT NULL | Synthesized insight |
| confidence | FLOAT | NOT NULL DEFAULT 0.3 | 0.0-1.0 |
| source | TEXT | NOT NULL | 'reflect', 'pattern_detect' |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### procedural_memories

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| content | TEXT | NOT NULL | Procedural knowledge |
| trigger | TEXT | NULLABLE | When to apply |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### graph_nodes

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| node_type | TEXT | NOT NULL | 'observation', 'memory', 'lesson', 'concept' |
| entity_id | TEXT | NOT NULL | FK to source entity |
| label | TEXT | NOT NULL | Human-readable label |
| metadata | JSONB | NULLABLE | Additional properties |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

### graph_edges

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| id | TEXT | PK | UUID |
| from_node_id | TEXT | FK → graph_nodes(id) ON DELETE CASCADE | Source |
| to_node_id | TEXT | FK → graph_nodes(id) ON DELETE CASCADE | Target |
| edge_type | TEXT | NOT NULL | 'related_to', 'derived_from', 'contradicts', 'strengthens' |
| weight | FLOAT | NOT NULL DEFAULT 0.5 | Edge strength 0.0-1.0 |
| created_at | TIMESTAMPTZ | NOT NULL DEFAULT now() | Creation time |

## State Transitions

### Observation Lifecycle

```
[observe] → Raw Observation
              │
              ▼
         [compress] (async, ~30s)
              │
              ▼
         CompressedObservation (BM25 + Vector indexed)
              │
              ▼
         [summarize] (on SessionEnd)
              │
              ▼
         SessionSummary
              │
              ▼
         [consolidate] (async timer)
              │
              ├──→ SemanticMemory (configurable visibility)
              ├──→ Lesson (team visibility)
              └──→ Insight (low confidence, for review)
```

### Session Lifecycle

```
[SessionStart] → active
                   │
                   ▼
              observations accumulate
                   │
                   ▼
              [SessionEnd] → ended
                   │
                   ▼
              summarize → consolidate → reflect
```

### Team Membership Lifecycle

```
[join] → team_members row created → active member
                                       │
                                       ▼
                                  [leave/remove] → team_members row DELETED
                                       │
                                       ▼
                                  private data stays | team memories persist
```

## Ownership & Visibility Matrix

| Entity | owner_type | owner_user_id | owner_team_id | visibility | Notes |
|--------|------------|---------------|---------------|------------|-------|
| Observation | user | user_id | — | private | CHECK constraint enforced |
| CompressedObs | user | user_id | — | private | CHECK constraint enforced |
| SessionSummary | user | user_id | — | private | CHECK constraint enforced |
| Crystal | user | user_id | — | private | CHECK constraint enforced |
| Memory | user/team | user_id | team_id | private/team/public | Configurable |
| Lesson | — | — | team_id | team | CHECK constraint enforced |
