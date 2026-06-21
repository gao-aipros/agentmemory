# v2 Scope Decisions

Date: 2026-06-20 (early version — Team/User was still TBD at this point, later superseded by 06-team-user.md)

## 1. Platform Migration (Core Task)

- TypeScript/Node.js → Go
- SQLite + in-memory Vector Map + in-memory BM25 Map + KV Graph → PostgreSQL + pgvector + pg_search (ParadeDB)
- Single observation table with BM25 + HNSW + B-tree indexes
- Hybrid query in one SQL

## 2. Architecture Preservation

Multi-level extract/aggregate/consolidation pipeline (observe → compress → summarize → consolidate → reflect),
dual Observation/Action pipelines, dual Context/Search consumption lines — all design decisions carry over.

- SessionSummary → context injection only (not search index)
- CompressedObservation → BM25 + Vector dual index

## 3. Architecture Gaps Deferred

- Pipeline inter-connections (observation → action auto-derivation)
- Crystals/auto timer fallback
- ProceduralMemory consumer
- All to be addressed after migration completes

## 4. Team/User Architecture (was TBD, now finalized)

At the time of this decision, the team/user model was just TEAM_ID/USER_ID string env vars
with manual copy sharing. This has since been fully designed — see 06-team-user.md.
