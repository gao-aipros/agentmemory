# v2 Team/User Architecture (Final)

Date: 2026-06-20

## Core

- First-class User/Team entities in PG
- Row-level ownership (owner_type: user|team)
- Three visibility levels: private, team, public
- Sharing at Memory layer only, no data copy
- One-to-many user-team (many-to-many deferred to future)

## Visibility by Data Type

| Data Type | Visibility |
|-----------|-----------|
| Observations | Always private |
| CompressedObs | Always private |
| SessionSummary | Always private |
| Crystals | Always private |
| Lessons | Always team (all sources: crystallize, manual, consolidation) |
| Memory | private / team / public (consolidation auto by mode, manual save user-picks) |

**Privacy escape hatch:** use `memory_save` for anything you want private or differently-scoped.

## Three Modes

| Mode | Behavior |
|------|----------|
| Owned by User | Consolidation per-user, auto-share to team |
| Owned by Team | Single team consolidation |
| Member Choice | Per-observation share flag from client |

## Mechanics

- Per-obs share flag: `AGENTMEMORY_SHARE_CONSOLIDATED` from client
- Exit = DELETE from team_members, no cleanup
- Re-join = full access to history (no time windows)
- Mesh out of scope; PG replication for cross-instance

## Context Injection

1500 token hard limit, reference format (one-line summary + recall ID):

| Source | Budget |
|--------|--------|
| Pinned Slots | ~300t |
| Your Recent Sessions | ~250t |
| Team Lessons | ~200t |
| Team Shared Memory | ~250t |
| Project Profile | ~100t |

## Search Logic

Uses `owner_type`, `owner_user_id`, `owner_team_id`, and `visibility` in queries.

## Remaining for Future

- Many-to-many user-team
