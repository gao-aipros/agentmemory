# v2 Team/User Model

Finalized 2026-06-20.

## Core Design

First-class User and Team entities in PostgreSQL — NOT env var strings like v0.
Every data row carries ownership and visibility metadata.

### Row-Level Ownership
- `owner_type`: `user` or `team` — who owns this row
- `owner_user_id`: the owning user (when owner_type = user)
- `owner_team_id`: the owning team (when owner_type = team)
- `visibility`: `private`, `team`, or `public` — who can see this row

### Relationship
- One-to-many: each user belongs to at most one team at a time
- Many-to-many is deferred to future

---

## Three Visibility Levels

| Level | Meaning |
|-------|---------|
| **private** | Only the owning user can see |
| **team** | All members of the owning team can see |
| **public** | All authorized users on this instance can see |

Cross-instance visibility via PostgreSQL logical replication — out of scope for v2.

---

## Visibility by Data Type

| Data Type | Visibility Rule | Rationale |
|-----------|----------------|-----------|
| Observations | **Always private** | Raw agent interactions contain sensitive context |
| CompressedObs | **Always private** | Derived from private observations |
| SessionSummary | **Always private** | Session-level summary, context injection only |
| Crystals | **Always private** | Action chain narratives are user-specific |
| Lessons | **Always team** | All sources: crystallize, manual save, consolidation |
| Memory | **Configurable** | private / team / public, set by consolidation auto-mode or manual save |

### Privacy Escape Hatch
Use `memory_save` for anything you want to keep private or scope differently from the default.
This gives users explicit control when the default visibility doesn't fit a specific case.

### Sharing Boundary
Sharing only happens at the **Memory layer**.
Observations, CompressedObs, SessionSummary, Crystals are NEVER shared — full stop.
No data copy for sharing (unlike v0's team-share which duplicated data).

---

## Three Operational Modes

| Mode | Consolidation | Sharing |
|------|--------------|---------|
| **Owned by User** | Per-user consolidation | Auto-share consolidated Memory to team |
| **Owned by Team** | Single team-level consolidation | Memory belongs to team |
| **Member Choice** | Per-user consolidation | Per-observation share flag from client |

### Mode Mechanics

**Owned by User:**
- Each user has their own consolidation pipeline
- After consolidation, Memory can be auto-shared to team
- Users maintain individual context while contributing to team knowledge

**Owned by Team:**
- One consolidation run covers the entire team
- All team members contribute to the same Memory pool
- Simpler but less personalization

**Member Choice:**
- Client sets `AGENTMEMORY_SHARE_CONSOLIDATED` flag per observation
- Maximum flexibility, more client responsibility
- Default mode for new teams

---

## Membership Lifecycle

### Join
- User added to `team_members` table with `role` (owner/member)
- Gains immediate access to all team-visible data

### Exit
- `DELETE FROM team_members WHERE team_id = ? AND user_id = ?`
- **No cleanup** of user's private data — it stays
- User's contributions to team Memory remain with the team

### Re-join
- Full access to team history is restored
- No time windows, no partial access
- User's previous private data (if not deleted) is also accessible again

---

## Search Logic

Search queries filter by ownership and visibility. The exact columns differ by table:

### observations, session_summaries, crystals (user-owned, no team ownership)

```sql
WHERE (
  -- User's own data (always private for observations)
  (user_id = ?)
  OR
  -- Public data (not applicable for observations which are always private,
  -- but session_summaries/crystals may have broader visibility in future)
  (visibility = 'public')
)
```

These tables have `user_id` + `visibility` columns. No `owner_type` — they are always user-owned.

### memories (user or team owned, configurable visibility)

```sql
WHERE (
  -- User's own memories
  (owner_type = 'user' AND user_id = ? AND visibility = 'private')
  OR
  -- Team memories visible to this user
  (owner_type = 'team' AND team_id IN (
    SELECT team_id FROM team_members WHERE user_id = ?
  ) AND visibility IN ('team', 'public'))
  OR
  -- Public memories from any user
  (visibility = 'public')
)
```

The `memories` table has `owner_type`, `user_id`, `team_id`, `visibility`.

### lessons (always team-scoped)

```sql
WHERE (
  team_id IN (SELECT team_id FROM team_members WHERE user_id = ?)
  AND deleted = false
)
```

The `lessons` table has `user_id`, `team_id`, `deleted`. Visibility is always team.

This ensures:
- Users always see their own private data
- Team members see team-scoped data
- Public data is visible to all authenticated users
- No cross-team leakage

---

## Context Injection Scope

1500 token hard limit, reference format:

| Source | Budget | Description |
|--------|--------|-------------|
| Pinned Slots | ~300t | Project-level pinned memory slots |
| Your Recent Sessions | ~250t | User's own recent session summaries |
| Team Lessons | ~200t | Lessons learned by team members |
| Team Shared Memory | ~250t | Team-visible consolidated memories |
| Project Profile | ~100t | Project-level patterns and conventions |

Each item: one-line summary + recall ID.
The recall ID lets the agent fetch full content on demand, so context injection stays lean.

The 5 source budgets sum to ~1100 tokens. The remaining ~400 tokens of the 1500-token
hard limit are consumed by:
- Section labels and formatting (~50 tokens)
- Recall IDs per item (~10 tokens each, typically 10-15 items = ~100-150 tokens)
- Newlines and structural whitespace (~50 tokens)
- Safety margin for tokenizer variance (~150-200 tokens)

---

## v0 → v2 Migration

v0: `TEAM_ID` and `USER_ID` are just env var strings. Team sharing is manual copy. No ACL, no real
entity model, no data isolation boundary between private and shared. Mesh is instance-level sync,
not user-level.

v2: Full entity model with PG tables, row-level ACL, proper isolation.
This is a complete redesign of the team/user architecture — not an incremental change.
