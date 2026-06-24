# Design: Migrate Raw SQL in Service Layer to sqlc-Generated Store

**Issue:** [#162](https://github.com/agentmemory/agentmemory/issues/162)
**Date:** 2026-06-23
**Status:** Design approved — awaiting implementation

## Goal

Eliminate 8 hand-written raw SQL queries across 4 service files and replace them with
sqlc-generated store methods. This restores architectural consistency: the project
standard is that all database access goes through `internal/store/` (sqlc-generated),
and these 8 queries are the only outliers.

## Current State

Eight raw SQL queries bypass the store layer:

| # | File | Method | SQL | sqlc Annotation |
|---|------|--------|-----|-----------------|
| 1 | `evict.go:24` | `findEvictionCandidates` | SELECT on observations (importance filter, age calc, limit) | `:many` |
| 2 | `evict.go:56` | `deleteObservation` | DELETE FROM observations WHERE id=$1 | Already exists as `DeleteObservation :exec` |
| 3 | `pipeline.go:234` | `ListMemories` (reflection) | SELECT all columns FROM memories ORDER BY created_at DESC LIMIT $1 | `:many` |
| 4 | `pipeline.go:236` | `InsertInsight` (reflection) | INSERT INTO insights (id, content, confidence, source, created_at) | `:exec` |
| 5 | `pipeline.go:416` | `batchInsertMemories` | Dynamic multi-row INSERT via `fmt.Sprintf` | `:copyfrom` |
| 6 | `pipeline.go:450` | `batchInsertLessons` | Dynamic multi-row INSERT via `fmt.Sprintf` | `:copyfrom` |
| 7 | `file_history.go:32` | `getFileHistory` | SELECT on observations with `&&` (array overlap) filter | `:many` |
| 8 | `patterns.go:52` | `getConceptFrequencies` | SELECT with `unnest()` and `cardinality()` | `:many` |

- **No SQL injection risk** — all queries use parameterized placeholders (`$1`, `$2`, ...). The `fmt.Sprintf` calls in `pipeline.go` construct placeholder lists, not values.
- **Primary concern is architectural consistency** — the codebase standard is sqlc-generated queries in `internal/store/`. Bypassing this layer makes refactoring, schema migrations, and query auditing harder.

## Target State

Zero raw SQL in the service layer. All 8 queries moved to `.sql` files, sqlc-generated
into `internal/store/`. For the batch inserts, sqlc's `:copyfrom` annotation generates
`pgx.CopyFrom` calls — same performance as the current `fmt.Sprintf` approach, but
type-safe and fully generated.

### New sqlc Query Files

**`internal/db/queries/observations_ext.sql`** — 3 queries:

```sql
-- name: ListEvictionCandidates :many
SELECT id, importance,
       EXTRACT(EPOCH FROM (now() - created_at)) / 86400 AS age_days
FROM observations
WHERE importance < $1
ORDER BY importance ASC, created_at ASC
LIMIT $2;

-- name: GetFileHistory :many
SELECT id, session_id, title, narrative, files, timestamp
FROM observations
WHERE files IS NOT NULL AND files && $1
  AND session_id != $2
ORDER BY timestamp DESC
LIMIT 100;

-- name: GetConceptFrequencies :many
SELECT unnest(concepts) AS concept, count(*) AS freq
FROM observations
WHERE concepts IS NOT NULL AND cardinality(concepts) > 0
GROUP BY concept
ORDER BY freq DESC
LIMIT 50;
```

**`internal/db/queries/memories_ext.sql`** — 2 queries:

```sql
-- name: ListAllMemories :many
SELECT * FROM memories ORDER BY created_at DESC LIMIT $1;

-- name: BatchInsertMemories :copyfrom
INSERT INTO memories (id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
```

**`internal/db/queries/insights.sql`** — new file (no existing insights queries):

```sql
-- name: InsertInsight :exec
INSERT INTO insights (id, content, confidence, source, created_at)
VALUES ($1, $2, $3, 'reflect', now());
```

**`internal/db/queries/lessons_ext.sql`** — 1 query:

```sql
-- name: BatchInsertLessons :copyfrom
INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now());
```

**Note:** `DeleteObservation` already exists in `observations.sql` — no new query needed for evict.go query #2.

After `sqlc generate`, these produce generated methods on `*store.Queries`:
- `ListEvictionCandidates(ctx, params) ([]ListEvictionCandidatesRow, error)`
- `GetFileHistory(ctx, params) ([]GetFileHistoryRow, error)`
- `GetConceptFrequencies(ctx, params) ([]GetConceptFrequenciesRow, error)`
- `ListAllMemories(ctx, limit int32) ([]Memory, error)`
- `InsertInsight(ctx, params) error`
- `BatchInsertMemories(ctx, rows []BatchInsertMemoriesParams) (int64, error)`
- `BatchInsertLessons(ctx, rows []BatchInsertLessonsParams) (int64, error)`
- `DeleteObservation(ctx, id string) error` — already exists

### Service Layer Changes

**`evict.go`:**
- Constructor changes from `NewEvictionService(pool)` → `NewEvictionService(queries *store.Queries)`
- `evictionQuerier` interface methods change to match store method signatures
- `evictionQuerierImpl` deleted; `*store.Queries` satisfies the interface directly
- `findEvictionCandidates` → calls `ListEvictionCandidates`
- `deleteObservation` → calls `DeleteObservation` (already existed!)

**`pipeline.go` — ReflectionService:**
- Constructor: `NewReflectionService(pool, interval)` → `NewReflectionService(queries *store.Queries, interval)`
- `reflectionQuerier` interface methods stay (already matching store pattern), but `reflectionQuerierImpl` deleted
- `ListMemories` → calls `ListAllMemories`
- `InsertInsight` → calls `InsertInsight`

**`pipeline.go` — ConsolidationService:**
- Already uses `*store.Queries` for other methods
- `batchInsertMemories` → calls `queries.BatchInsertMemories(ctx, rows)`
- `batchInsertLessons` → calls `queries.BatchInsertLessons(ctx, rows)`
- `pool` field removed (no longer needed for raw SQL)
- CopyFrom needs a `pgx.CopyFromSource` — ConsolidationService already holds `*store.Queries` which wraps a `DBTX` (a `*pgxpool.Pool`). The generated `:copyfrom` methods accept a `*pgx.Conn` or a `pgx.CopyFromSource` obtained from the pool. The service acquires a connection from the pool for the duration of the copy.

**`file_history.go`:**
- Constructor: `NewFileHistoryService(pool)` → `NewFileHistoryService(queries *store.Queries)`
- `fileHistoryQuerierImpl` deleted
- `getFileHistory` → calls `GetFileHistory`

**`patterns.go`:**
- Constructor: `NewPatternsService(pool)` → `NewPatternsService(queries *store.Queries)`
- `patternsQuerierImpl` deleted
- `getConceptFrequencies` → calls `GetConceptFrequencies`

### Querier Interfaces

Each service keeps its querier interface for mock-based testing — existing tests continue
working unchanged. The difference: `*store.Queries` now satisfies all interfaces directly
(no intermediate `*Impl` types). Callers pass `store.New(pool)` instead of `&evictionQuerierImpl{pool: pool}`.

### Caller Updates

Files that construct these services need updating:

| Caller | Service | Change |
|--------|---------|--------|
| `internal/handler/rest.go` | `EvictionService`, `FileHistoryService`, `PatternsService` | Pass `store.New(pool)` instead of `pool` |
| `internal/handler/rest_test.go` | Various | Update constructor args |
| Anywhere else constructing these | grep needed | Update constructor args |

## What Stays the Same

- All service public APIs
- All existing unit tests (they mock querier interfaces, not DB)
- Query behavior (`:copyfrom` produces identical row results)
- The querier interfaces keep the same method sets (though signatures may adjust to match sqlc-generated param types)
- No schema changes, no migrations

## Execution Plan

1. Write 4 `.sql` query files (`observations_ext.sql`, `memories_ext.sql`, `insights.sql`, `lessons_ext.sql`)
2. Run `sqlc generate` → generates `*_ext.sql.go`, `insights.sql.go`, updates `models.go`
3. Update `evict.go` — replace `evictionQuerierImpl` with `*store.Queries`
4. Update `pipeline.go` — replace `reflectionQuerierImpl` with `*store.Queries`, migrate batch methods to `:copyfrom`
5. Update `file_history.go` — replace `fileHistoryQuerierImpl` with `*store.Queries`
6. Update `patterns.go` — replace `patternsQuerierImpl` with `*store.Queries`
7. Update all callers that construct these services
8. Run `go test ./internal/service/... -v`
9. Run `go test ./tests/integration/... -v`

## Risk Assessment

- **Risk of regression:** Moderate — SQL behavior must be identical after migration
- **Mitigation:** Existing integration tests exercise these code paths; no SQL logic changes, only relocation
- **Biggest risk:** `:copyfrom` semantics vs current `INSERT ... VALUES` — both use the same wire protocol, but CopyFrom skips some PostgreSQL checks. When there are `BEFORE/AFTER INSERT` triggers, `COPY` fires statement-level triggers but not row-level. Our tables have no INSERT triggers, so this is safe.
- **`:copyfrom` availability:** Requires sqlc ≥ v1.19.0. This project uses sqlc v1.31.1.
