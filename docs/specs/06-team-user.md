# v2 Team/User Model

## Core Design

- **First-class User and Team entities** in PostgreSQL (not env var strings)
- **Row-level ownership:** every row has `owner_type` (user|team) and visibility
- **Three visibility levels:** private, team, public
- **Sharing boundary at Memory layer only** — no data copy for sharing
- **One-to-many user-team** relationship (many-to-many deferred to future)

## Three Modes

| Mode | Behavior |
|------|----------|
| **Owned by User** | Consolidation per-user, auto-share to team |
| **Owned by Team** | Single team consolidation |
| **Member Choice** | Per-observation share flag from client |

## Mechanics

- Per-obs share flag: `AGENTMEMORY_SHARE_CONSOLIDATED` from client
- Exit = DELETE from team_members, no cleanup
- Re-join = full access to history (no time windows)
- Mesh out of scope; PG replication for cross-instance

## Visibility Rules per Data Type

| Data Type | Visibility |
|-----------|-----------|
| Observations | Always private |
| CompressedObs | Always private |
| SessionSummary | Always private |
| Crystals | Always private |
| Lessons | Always team |
| Memory | Configurable (private/team/public) |

## Search Logic

Uses pseudo-SQL with `owner_type`, `owner_user_id`, `owner_team_id`, and `visibility`.

## Context Injection Scope

1500 token hard limit with per-item one-line summary + recall ID:

| Source | Budget |
|--------|--------|
| Pinned Slots | ~300t |
| Your Recent Sessions | ~250t |
| Team Lessons | ~200t |
| Team Shared Memory | ~250t |
| Project Profile | ~100t |
