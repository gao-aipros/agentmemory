# Migrate Raw SQL to sqlc Store — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate 8 hand-written raw SQL queries across 4 service files and replace them with sqlc-generated store methods, using `:copyfrom` for batch inserts.

**Architecture:** 4 new `.sql` query files → `sqlc generate` → update 4 service files to call `*store.Queries` methods instead of raw `pool.Query/Exec`. Each service keeps its querier interface for mock-based testing; the interfaces change signatures to match store types. Batch inserts use sqlc's `:copyfrom` annotation which generates `pgx.CopyFrom` calls.

**Tech Stack:** Go, pgx/v5, sqlc v1.31.1, PostgreSQL

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/db/queries/observations_ext.sql` | Eviction candidates, file history, concept frequencies |
| Create | `internal/db/queries/memories_ext.sql` | List all memories, batch insert memories |
| Create | `internal/db/queries/insights.sql` | Insert insight (new file, no prior insights queries) |
| Create | `internal/db/queries/lessons_ext.sql` | Batch insert lessons |
| Generate | `internal/store/observations_ext.sql.go` | sqlc output |
| Generate | `internal/store/memories_ext.sql.go` | sqlc output |
| Generate | `internal/store/insights.sql.go` | sqlc output |
| Generate | `internal/store/lessons_ext.sql.go` | sqlc output |
| Regenerate | `internal/store/db.go` | May add `CopyFrom` to `DBTX` interface |
| Modify | `internal/service/evict.go` | Replace `evictionQuerierImpl` with `*store.Queries` |
| Modify | `internal/service/evict_test.go` | Update mock to match new interface signatures |
| Modify | `internal/service/pipeline.go` | Replace `reflectionQuerierImpl` + batch inserts with `*store.Queries` |
| Modify | `internal/service/pipeline_test.go` | Update reflection mock |
| Modify | `internal/service/file_history.go` | Replace `fileHistoryQuerierImpl` with `*store.Queries` |
| Modify | `internal/service/file_history_test.go` | Update mock |
| Modify | `internal/service/patterns.go` | Replace `patternsQuerierImpl` with `*store.Queries` |
| Modify | `internal/service/patterns_test.go` | Update mock |
| Modify | `internal/mcp/tools.go` | Update constructor calls |

---

### Task 1: Create observations_ext.sql

**Files:**
- Create: `internal/db/queries/observations_ext.sql`

- [ ] **Step 1: Write the SQL query file**

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

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/observations_ext.sql
git commit -m "feat: add observations_ext.sql with eviction, file history, and concept queries (#162)"
```

---

### Task 2: Create memories_ext.sql

**Files:**
- Create: `internal/db/queries/memories_ext.sql`

- [ ] **Step 1: Write the SQL query file**

```sql
-- name: ListAllMemories :many
SELECT * FROM memories ORDER BY created_at DESC LIMIT $1;

-- name: BatchInsertMemories :copyfrom
INSERT INTO memories (id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
```

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/memories_ext.sql
git commit -m "feat: add memories_ext.sql with list-all and batch-insert queries (#162)"
```

---

### Task 3: Create insights.sql

**Files:**
- Create: `internal/db/queries/insights.sql`

- [ ] **Step 1: Write the SQL query file**

```sql
-- name: InsertInsight :exec
INSERT INTO insights (id, content, confidence, source, created_at)
VALUES ($1, $2, $3, 'reflect', now());
```

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/insights.sql
git commit -m "feat: add insights.sql with insert query (#162)"
```

---

### Task 4: Create lessons_ext.sql

**Files:**
- Create: `internal/db/queries/lessons_ext.sql`

- [ ] **Step 1: Write the SQL query file**

```sql
-- name: BatchInsertLessons :copyfrom
INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now());
```

- [ ] **Step 2: Commit**

```bash
git add internal/db/queries/lessons_ext.sql
git commit -m "feat: add lessons_ext.sql with batch-insert query (#162)"
```

---

### Task 5: Run sqlc generate and verify compilation

**Files:**
- Generate: `internal/store/` (multiple files regenerated)
- Verify: `go build ./...`

- [ ] **Step 1: Run sqlc generate**

```bash
cd /home/admin/work/github/agentmemory && sqlc generate
```

Expected: No errors. New files appear in `internal/store/`:
- `observations_ext.sql.go`
- `memories_ext.sql.go`
- `insights.sql.go`
- `lessons_ext.sql.go`
- `db.go` may be regenerated (check if `CopyFrom` was added to `DBTX`)

- [ ] **Step 2: Check if db.go DBTX interface includes CopyFrom**

```bash
grep "CopyFrom" internal/store/db.go
```

If `CopyFrom` is NOT in the `DBTX` interface, add it manually:

```go
type DBTX interface {
    Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
    Query(context.Context, string, ...interface{}) (pgx.Rows, error)
    QueryRow(context.Context, string, ...interface{}) pgx.Row
    CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}
```

If `*pgxpool.Pool` does not implement `CopyFrom`, also add a thin wrapper:

```go
type copyFromPool struct {
    *pgxpool.Pool
}

func (p copyFromPool) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
    conn, err := p.Acquire(ctx)
    if err != nil {
        return 0, err
    }
    defer conn.Release()
    return conn.CopyFrom(ctx, tableName, columnNames, rowSrc)
}
```

- [ ] **Step 3: Verify compilation**

```bash
cd /home/admin/work/github/agentmemory && go build ./...
```

Expected: Compiles successfully (service files still use old constructors).

- [ ] **Step 4: Inspect generated types for each new query**

Read the generated files to note exact generated type names and signatures:

```bash
grep -A 5 "type ListEvictionCandidatesRow\|type GetFileHistoryRow\|type GetConceptFrequenciesRow\|type ListAllMemoriesRow\|type BatchInsertMemoriesParams\|type BatchInsertLessonsParams\|type InsertInsightParams" internal/store/*.sql.go
```

Record these types — they will be used in service file updates.

- [ ] **Step 5: Commit**

```bash
git add internal/store/ && git commit -m "chore: run sqlc generate for new query files (#162)"
```

---

### Task 6: Update evict.go — replace evictionQuerierImpl with *store.Queries

**Files:**
- Modify: `internal/service/evict.go`
- Modify: `internal/service/evict_test.go`
- Reference: `internal/store/observations_ext.sql.go` (generated types)
- Reference: `internal/store/observations.sql.go` (existing `DeleteObservation`)

The `evictionQuerier` interface changes to match store method signatures. `*store.Queries` satisfies it directly. The `Age` string formatting moves from the querier impl to `FindCandidates`. `deleteObservation` reuses the existing `DeleteObservation` store method.

- [ ] **Step 1: Read current evict.go to confirm contents**

Already read in design phase — verify nothing changed:

```bash
git diff internal/service/evict.go
```

- [ ] **Step 2: Rewrite evict.go**

Replace the file content:

```go
package service

import (
    "context"
    "fmt"
    "log/slog"

    "github.com/agentmemory/agentmemory/internal/store"
)

// evictionQuerier is the subset of *store.Queries methods used by EvictionService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type evictionQuerier interface {
    ListEvictionCandidates(ctx context.Context, params store.ListEvictionCandidatesParams) ([]store.ListEvictionCandidatesRow, error)
    DeleteObservation(ctx context.Context, id string) error
}

// EvictionService handles pruning low-importance, old observations
// when the database approaches capacity. Compressed observations and
// lessons are preserved.
type EvictionService struct {
    queries evictionQuerier
}

// NewEvictionService creates a new EvictionService.
func NewEvictionService(queries *store.Queries) *EvictionService {
    return &EvictionService{
        queries: queries,
    }
}

// EvictionCandidate identifies an observation that may be evicted.
type EvictionCandidate struct {
    ObservationID string
    Importance    float64
    Age           string // human-readable age, e.g. "30.5 days"
}

// FindCandidates returns observations that are candidates for eviction:
// low importance (below 0.2) and old. Compressed observations and lessons
// are preserved. Results sorted by importance ascending, age ascending.
func (s *EvictionService) FindCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error) {
    if limit <= 0 {
        limit = 50
    }
    slog.Debug("searching for eviction candidates", "limit", limit)
    rows, err := s.queries.ListEvictionCandidates(ctx, store.ListEvictionCandidatesParams{
        Column1: 0.2,
        Limit:   int32(limit),
    })
    if err != nil {
        return nil, fmt.Errorf("failed to list eviction candidates: %w", err)
    }
    candidates := make([]EvictionCandidate, len(rows))
    for i, row := range rows {
        candidates[i] = EvictionCandidate{
            ObservationID: row.ID,
            Importance:    row.Importance,
            Age:           fmt.Sprintf("%.1f days", row.AgeDays),
        }
    }
    return candidates, nil
}

// EvictObservation deletes a single raw observation.
// Compressed observations and lessons are preserved.
func (s *EvictionService) EvictObservation(ctx context.Context, observationID string) error {
    slog.Info("evicting observation", "observation_id", observationID)
    return s.queries.DeleteObservation(ctx, observationID)
}
```

**Important:** The `ListEvictionCandidatesParams` struct field names depend on what sqlc generates. After Task 5 Step 4, verify the actual field names (likely `Column1` for `$1` and `Limit` for `$2`, or sqlc may generate named parameters). Adjust the struct literal accordingly if sqlc generated different names. The sqlc-generated types will be similar to:

```go
type ListEvictionCandidatesParams struct {
    Column1 float64  // importance threshold $1
    Limit   int32    // $2
}

type ListEvictionCandidatesRow struct {
    ID         string
    Importance float64
    AgeDays    float64
}
```

- [ ] **Step 3: Update evict_test.go**

The mock must implement the new interface. Replace the mock section:

```go
package service

import (
    "context"
    "fmt"
    "testing"

    "github.com/agentmemory/agentmemory/internal/store"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockEvictionQuerier implements evictionQuerier for testing.
type mockEvictionQuerier struct {
    candidates []store.ListEvictionCandidatesRow
    err        error
}

func (m *mockEvictionQuerier) ListEvictionCandidates(ctx context.Context, params store.ListEvictionCandidatesParams) ([]store.ListEvictionCandidatesRow, error) {
    if m.err != nil {
        return nil, m.err
    }
    if len(m.candidates) > int(params.Limit) {
        return m.candidates[:params.Limit], nil
    }
    return m.candidates, nil
}

func (m *mockEvictionQuerier) DeleteObservation(ctx context.Context, id string) error {
    return nil
}

func newEvictionServiceWithQuerier(q evictionQuerier) *EvictionService {
    return &EvictionService{
        queries: q,
    }
}

func TestEviction_FindCandidates_ReturnsCandidates(t *testing.T) {
    mock := &mockEvictionQuerier{
        candidates: []store.ListEvictionCandidatesRow{
            {ID: "obs-1", Importance: 0.1, AgeDays: 30.5},
            {ID: "obs-2", Importance: 0.15, AgeDays: 25.0},
        },
    }
    svc := newEvictionServiceWithQuerier(mock)

    candidates, err := svc.FindCandidates(context.Background(), 10)
    require.NoError(t, err)
    assert.Len(t, candidates, 2)
    assert.Equal(t, "obs-1", candidates[0].ObservationID)
    assert.Equal(t, 0.1, candidates[0].Importance)
    assert.Equal(t, "30.5 days", candidates[0].Age)
}

func TestEviction_FindCandidates_EmptyResult(t *testing.T) {
    mock := &mockEvictionQuerier{
        candidates: []store.ListEvictionCandidatesRow{},
    }
    svc := newEvictionServiceWithQuerier(mock)

    candidates, err := svc.FindCandidates(context.Background(), 10)
    require.NoError(t, err)
    assert.Empty(t, candidates)
}

func TestEviction_FindCandidates_ErrorPropagation(t *testing.T) {
    mock := &mockEvictionQuerier{
        err: fmt.Errorf("database connection lost"),
    }
    svc := newEvictionServiceWithQuerier(mock)

    _, err := svc.FindCandidates(context.Background(), 10)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "database connection lost")
}

func TestEviction_FindCandidates_RespectsLimit(t *testing.T) {
    mock := &mockEvictionQuerier{
        candidates: []store.ListEvictionCandidatesRow{
            {ID: "obs-1", Importance: 0.1, AgeDays: 1},
            {ID: "obs-2", Importance: 0.2, AgeDays: 2},
            {ID: "obs-3", Importance: 0.3, AgeDays: 3},
        },
    }
    svc := newEvictionServiceWithQuerier(mock)

    candidates, err := svc.FindCandidates(context.Background(), 2)
    require.NoError(t, err)
    assert.Len(t, candidates, 2)
}
```

- [ ] **Step 4: Run evict tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/ -run TestEviction -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/evict.go internal/service/evict_test.go
git commit -m "refactor: migrate evict.go raw SQL to sqlc store (#162)"
```

---

### Task 7: Update pipeline.go reflection — replace reflectionQuerierImpl

**Files:**
- Modify: `internal/service/pipeline.go`
- Modify: `internal/service/pipeline_test.go`
- Reference: `internal/store/memories_ext.sql.go` (ListAllMemories)
- Reference: `internal/store/insights.sql.go` (InsertInsight)

The `reflectionQuerier` interface changes to match `ListAllMemories` + `InsertInsight` store signatures. `*store.Queries` satisfies it directly. The `reflectionQuerierImpl` struct and its methods are deleted.

- [ ] **Step 1: Replace raw SQL constants and reflectionQuerierImpl in pipeline.go**

Delete the `listAllMemories` and `insertInsight` const block (lines 232-237), the `reflectionQuerierImpl` struct (lines 247-249), and its methods (lines 251-284). Replace the `reflectionQuerier` interface:

```go
// reflectionQuerier is the subset of *store.Queries methods used by ReflectionService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type reflectionQuerier interface {
    ListAllMemories(ctx context.Context, limit int32) ([]store.Memory, error)
    InsertInsight(ctx context.Context, params store.InsertInsightParams) error
}
```

- [ ] **Step 2: Update NewReflectionService**

Change constructor signature from `pool *pgxpool.Pool` to `queries *store.Queries`:

```go
// NewReflectionService creates a new ReflectionService.
func NewReflectionService(queries *store.Queries, timerIntervalSeconds int) *ReflectionService {
    if timerIntervalSeconds <= 0 {
        timerIntervalSeconds = 3600 // default 1 hour
    }
    return &ReflectionService{
        queries:       queries,
        timerInterval: timerIntervalSeconds,
    }
}
```

- [ ] **Step 3: Update newReflectionServiceWithQuerier**

Remove the hardcoded pool reference:

```go
// newReflectionServiceWithQuerier creates a ReflectionService with a custom querier (for testing).
func newReflectionServiceWithQuerier(q reflectionQuerier) *ReflectionService {
    return &ReflectionService{
        queries:       q,
        timerInterval: 3600,
    }
}
```

- [ ] **Step 4: Update RunReflection — InsertInsight call**

Change line ~351 from:
```go
s.queries.InsertInsight(ctx, uuid.New().String(), insight.Content, insight.Confidence)
```
to:
```go
s.queries.InsertInsight(ctx, store.InsertInsightParams{
    ID:         uuid.New().String(),
    Content:    insight.Content,
    Confidence: insight.Confidence,
})
```

- [ ] **Step 5: Remove unused imports**

After deleting the `reflectionQuerierImpl`, the `pgxpool` import may no longer be needed for the reflection code. But `pgxpool` is still used by `ConsolidationService` — check if it's still referenced. If the consolidation batch methods still reference `pool`, keep it. If they've been migrated too (Task 8), remove it.

- [ ] **Step 6: Update pipeline_test.go reflection mock**

Replace `mockReflectionQuerier`:

```go
// mockReflectionQuerier implements reflectionQuerier for testing.
type mockReflectionQuerier struct {
    listAllMemories func(ctx context.Context, limit int32) ([]store.Memory, error)
    insertInsight   func(ctx context.Context, params store.InsertInsightParams) error
}

func (m *mockReflectionQuerier) ListAllMemories(ctx context.Context, limit int32) ([]store.Memory, error) {
    return m.listAllMemories(ctx, limit)
}

func (m *mockReflectionQuerier) InsertInsight(ctx context.Context, params store.InsertInsightParams) error {
    return m.insertInsight(ctx, params)
}
```

Update `TestReflectionService_RunReflection_NoMemories` — change `insertInsight` callback signature:

```go
insertInsight: func(ctx context.Context, params store.InsertInsightParams) error {
    t.Error("insertInsight should not be called when there are no memories")
    return nil
},
```

Update `TestReflectionService_RunReflection_WithMemories` — change `insertInsight` callback:

```go
insertInsight: func(ctx context.Context, params store.InsertInsightParams) error {
    capturedInsights = append(capturedInsights, struct {
        content    string
        confidence float64
    }{params.Content, params.Confidence})
    return nil
},
```

- [ ] **Step 7: Run reflection tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/ -run TestReflectionService -v
```

Expected: PASS

- [ ] **Step 8: Run all pipeline tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/ -run "TestSummarize|TestReflection" -v
```

Expected: PASS (summarization tests still pass)

- [ ] **Step 9: Commit**

```bash
git add internal/service/pipeline.go internal/service/pipeline_test.go
git commit -m "refactor: migrate reflection raw SQL to sqlc store (#162)"
```

---

### Task 8: Update pipeline.go consolidation — migrate batch inserts to :copyfrom

**Files:**
- Modify: `internal/service/pipeline.go`

Replace `batchInsertMemories` and `batchInsertLessons` methods with calls to generated `:copyfrom` methods. Remove the `pool *pgxpool.Pool` field from `ConsolidationService`.

- [ ] **Step 1: Remove pool field from ConsolidationService struct**

Change:
```go
type ConsolidationService struct {
    queries    *store.Queries
    llmService *LLMService
    mode       ConsolidationMode
    pool       *pgxpool.Pool // for batch inserts
}
```
to:
```go
type ConsolidationService struct {
    queries    *store.Queries
    llmService *LLMService
    mode       ConsolidationMode
}
```

- [ ] **Step 2: Simplify NewConsolidationService**

Change:
```go
func NewConsolidationService(pool *pgxpool.Pool, llm *LLMService, mode ConsolidationMode) *ConsolidationService {
    return &ConsolidationService{
        queries:    store.New(pool),
        llmService: llm,
        mode:       mode,
        pool:       pool,
    }
}
```
to:
```go
func NewConsolidationService(queries *store.Queries, llm *LLMService, mode ConsolidationMode) *ConsolidationService {
    return &ConsolidationService{
        queries:    queries,
        llmService: llm,
        mode:       mode,
    }
}
```

- [ ] **Step 3: Replace batchInsertMemories call in ConsolidateSession**

Change lines ~190-203 (the batch insert call block). Replace:
```go
memories := make([]memoryRow, 0, len(result.Memories))
for _, m := range result.Memories {
    memories = append(memories, memoryRow{
        id:         uuid.New().String(),
        content:    m.Content,
        concepts:   m.Concepts,
        source:     "consolidation",
        confidence: 0.5,
    })
}
if err := s.batchInsertMemories(ctx, memories, s.mode.OwnerUserID, s.mode.OwnerTeamID, visibility); err != nil {
    slog.Warn("failed to batch insert memories", "error", err)
}
```
with:
```go
memRows := make([]store.BatchInsertMemoriesParams, 0, len(result.Memories))
for _, m := range result.Memories {
    memRows = append(memRows, store.BatchInsertMemoriesParams{
        ID:          uuid.New().String(),
        OwnerType:   "user",
        OwnerUserID: &s.mode.OwnerUserID,
        OwnerTeamID: &s.mode.OwnerTeamID,
        Visibility:  visibility,
        Content:     m.Content,
        Concepts:    m.Concepts,
        Source:      "consolidation",
        Confidence:  0.5,
    })
}
if len(memRows) > 0 {
    if _, err := s.queries.BatchInsertMemories(ctx, memRows); err != nil {
        slog.Warn("failed to batch insert memories", "error", err)
    }
}
```

**Note:** Verify `OwnerUserID` and `OwnerTeamID` field types in the generated `BatchInsertMemoriesParams`. They may be `*string` or `string` depending on the schema. Adjust the struct literal accordingly.

- [ ] **Step 4: Replace batchInsertLessons call in ConsolidateSession**

Change lines ~206-222. Replace:
```go
lessons := make([]lessonRow, 0, len(result.Lessons))
for _, l := range result.Lessons {
    lessons = append(lessons, lessonRow{
        id:         uuid.New().String(),
        teamID:     s.mode.OwnerTeamID,
        visibility: "team",
        content:    l.Content,
        context:    l.Context,
        confidence: 0.5,
        source:     "consolidation",
    })
}
if err := s.batchInsertLessons(ctx, lessons); err != nil {
    slog.Warn("failed to batch insert lessons", "error", err)
}
```
with:
```go
lessonRows := make([]store.BatchInsertLessonsParams, 0, len(result.Lessons))
for _, l := range result.Lessons {
    lessonRows = append(lessonRows, store.BatchInsertLessonsParams{
        ID:         uuid.New().String(),
        TeamID:     &s.mode.OwnerTeamID,
        Visibility: "team",
        Content:    l.Content,
        Context:    &l.Context,
        Confidence: 0.5,
        Source:     "consolidation",
    })
}
if len(lessonRows) > 0 {
    if _, err := s.queries.BatchInsertLessons(ctx, lessonRows); err != nil {
        slog.Warn("failed to batch insert lessons", "error", err)
    }
}
```

**Note:** Verify field types in generated `BatchInsertLessonsParams`. `TeamID` and `Context` may be `*string`. Adjust accordingly.

- [ ] **Step 5: Delete batchInsertMemories and batchInsertLessons methods**

Remove the entire `batchInsertMemories` method (lines ~393-424) and `batchInsertLessons` method (lines ~426-458). Also remove `memoryRow` and `lessonRow` type definitions (lines ~373-390) since they're no longer used.

- [ ] **Step 6: Remove fmt import if no longer needed**

`fmt.Sprintf` was only used by the batch methods. Check if `fmt` is used elsewhere in pipeline.go (it is — for error formatting in ConsolidateSession and other methods). Keep it.

- [ ] **Step 7: Remove pgxpool import if no longer needed**

Check if `pgxpool` is referenced anywhere else in pipeline.go after all changes:
- `SummarizationService` uses `store.New(pool)` and the constructor — but if its constructor was already `*store.Queries`, it won't reference `pgxpool`. Wait, `NewSummarizationService` currently takes `pool *pgxpool.Pool`. Check if it's still needed...

Actually, `SummarizationService` constructor (`NewSummarizationService`) still takes `pool *pgxpool.Pool` — that's separate from our refactoring scope. And the ConsolidationService now takes `*store.Queries`. So `pgxpool` import stays for now.

- [ ] **Step 8: Verify compilation**

```bash
cd /home/admin/work/github/agentmemory && go build ./internal/service/
```

Expected: Compiles.

- [ ] **Step 9: Commit**

```bash
git add internal/service/pipeline.go
git commit -m "refactor: migrate consolidation batch inserts to sqlc :copyfrom (#162)"
```

---

### Task 9: Update file_history.go — replace fileHistoryQuerierImpl

**Files:**
- Modify: `internal/service/file_history.go`
- Modify: `internal/service/file_history_test.go`
- Reference: `internal/store/observations_ext.sql.go` (GetFileHistory)

- [ ] **Step 1: Rewrite file_history.go**

```go
package service

import (
    "context"

    "github.com/agentmemory/agentmemory/internal/store"
    "github.com/jackc/pgx/v5/pgtype"
)

// FileHistoryEntry represents a past observation about a specific file.
type FileHistoryEntry struct {
    File          string           `json:"file"`
    ObservationID string           `json:"observation_id"`
    Title         string           `json:"title"`
    Narrative     string           `json:"narrative"`
    Timestamp     pgtype.Timestamptz `json:"timestamp"`
    SessionID     string           `json:"session_id"`
}

// fileHistoryQuerier is the subset of *store.Queries methods used by FileHistoryService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type fileHistoryQuerier interface {
    GetFileHistory(ctx context.Context, params store.GetFileHistoryParams) ([]store.GetFileHistoryRow, error)
}

// FileHistoryService looks up past observations about specific files.
type FileHistoryService struct {
    queries fileHistoryQuerier
}

// NewFileHistoryService creates a new FileHistoryService backed by the given store queries.
func NewFileHistoryService(queries *store.Queries) *FileHistoryService {
    return &FileHistoryService{
        queries: queries,
    }
}

// newFileHistoryServiceWithQuerier creates a FileHistoryService with a custom querier (for testing).
func newFileHistoryServiceWithQuerier(q fileHistoryQuerier) *FileHistoryService {
    return &FileHistoryService{
        queries: q,
    }
}

// GetFileHistory returns past observations about the given files, optionally
// excluding entries from the specified session.
func (s *FileHistoryService) GetFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error) {
    rows, err := s.queries.GetFileHistory(ctx, store.GetFileHistoryParams{
        Column1: files,
        Column2: excludeSessionID,
    })
    if err != nil {
        return nil, err
    }
    entries := make([]FileHistoryEntry, len(rows))
    for i, row := range rows {
        var file string
        if len(row.Files) > 0 {
            file = row.Files[0]
        }
        entries[i] = FileHistoryEntry{
            File:          file,
            ObservationID: row.ID,
            Title:         row.Title,
            Narrative:     row.Narrative,
            Timestamp:     row.Timestamp,
            SessionID:     row.SessionID,
        }
    }
    return entries, nil
}
```

**Note:** Verify generated `GetFileHistoryParams` field names after `sqlc generate`. They may be `Column1`/`Column2` or named parameters like `Files`/`ExcludeSessionID`. Adjust struct keys accordingly. Also verify that `Files` in the returned row is `[]string` (it is per the Observation model).

- [ ] **Step 2: Update file_history_test.go mock**

```go
package service

import (
    "context"
    "testing"
    "time"

    "github.com/agentmemory/agentmemory/internal/store"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockFileHistoryQuerier implements fileHistoryQuerier for testing.
type mockFileHistoryQuerier struct {
    entries []store.GetFileHistoryRow
    err     error
}

func (m *mockFileHistoryQuerier) GetFileHistory(ctx context.Context, params store.GetFileHistoryParams) ([]store.GetFileHistoryRow, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.entries, nil
}

func TestFileHistory_GetFileHistory_ReturnsEntries(t *testing.T) {
    now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
    mock := &mockFileHistoryQuerier{
        entries: []store.GetFileHistoryRow{
            {ID: "obs-1", SessionID: "sess-1", Title: "added feature", Narrative: "added login handler", Files: []string{"file1.go"}, Timestamp: now},
            {ID: "obs-2", SessionID: "sess-2", Title: "fixed bug", Narrative: "fixed nil pointer", Files: []string{"file2.go"}, Timestamp: now},
        },
    }
    svc := newFileHistoryServiceWithQuerier(mock)

    entries, err := svc.GetFileHistory(context.Background(), []string{"file1.go", "file2.go"}, "sess-3")
    require.NoError(t, err)
    assert.Len(t, entries, 2)
    assert.Equal(t, "obs-1", entries[0].ObservationID)
    assert.Equal(t, "file1.go", entries[0].File)
    assert.Equal(t, "sess-2", entries[1].SessionID)
}

func TestFileHistory_GetFileHistory_EmptyResult(t *testing.T) {
    mock := &mockFileHistoryQuerier{
        entries: []store.GetFileHistoryRow{},
    }
    svc := newFileHistoryServiceWithQuerier(mock)

    entries, err := svc.GetFileHistory(context.Background(), []string{"nonexistent.go"}, "sess-1")
    require.NoError(t, err)
    assert.Empty(t, entries)
}

func TestFileHistory_GetFileHistory_ErrorPropagation(t *testing.T) {
    mock := &mockFileHistoryQuerier{
        err: assert.AnError,
    }
    svc := newFileHistoryServiceWithQuerier(mock)

    _, err := svc.GetFileHistory(context.Background(), []string{"file.go"}, "sess-1")
    require.Error(t, err)
    assert.ErrorIs(t, err, assert.AnError)
}
```

- [ ] **Step 3: Run file history tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/ -run TestFileHistory -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/file_history.go internal/service/file_history_test.go
git commit -m "refactor: migrate file_history.go raw SQL to sqlc store (#162)"
```

---

### Task 10: Update patterns.go — replace patternsQuerierImpl

**Files:**
- Modify: `internal/service/patterns.go`
- Modify: `internal/service/patterns_test.go`
- Reference: `internal/store/observations_ext.sql.go` (GetConceptFrequencies)

- [ ] **Step 1: Rewrite patterns.go**

```go
package service

import (
    "context"
    "time"

    "github.com/agentmemory/agentmemory/internal/store"
)

// PatternSummary aggregates detected patterns across sessions, concepts,
// tool usage, and file patterns for a given project.
type PatternSummary struct {
    Project      string        `json:"project"`
    TopConcepts  []ConceptFreq `json:"top_concepts"`
    ToolUsage    []ToolFreq    `json:"tool_usage"`
    FilePatterns []FilePattern `json:"file_patterns"`
    SessionCount int           `json:"session_count"`
    GeneratedAt  time.Time     `json:"generated_at"`
}

// ConceptFreq holds a concept and its occurrence count.
type ConceptFreq struct {
    Concept string `json:"concept"`
    Count   int    `json:"count"`
}

// ToolFreq holds a tool name and its usage count.
type ToolFreq struct {
    Tool  string `json:"tool"`
    Count int    `json:"count"`
}

// FilePattern describes a recurring file naming or modification pattern.
type FilePattern struct {
    Pattern     string `json:"pattern"`
    Count       int    `json:"count"`
    Description string `json:"description"`
}

// patternsQuerier is the subset of *store.Queries methods used by PatternsService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type patternsQuerier interface {
    GetConceptFrequencies(ctx context.Context) ([]store.GetConceptFrequenciesRow, error)
}

// PatternsService detects recurring patterns across sessions for a project.
type PatternsService struct {
    queries patternsQuerier
}

// NewPatternsService creates a new PatternsService backed by the given store queries.
func NewPatternsService(queries *store.Queries) *PatternsService {
    return &PatternsService{
        queries: queries,
    }
}

// newPatternsServiceWithQuerier creates a PatternsService with a custom querier (for testing).
func newPatternsServiceWithQuerier(q patternsQuerier) *PatternsService {
    return &PatternsService{
        queries: q,
    }
}

// DetectPatterns analyzes past session data for a project and returns a
// PatternSummary with detected concept frequencies.
func (s *PatternsService) DetectPatterns(ctx context.Context, project string) (*PatternSummary, error) {
    rows, err := s.queries.GetConceptFrequencies(ctx)
    if err != nil {
        return nil, err
    }
    concepts := make([]ConceptFreq, len(rows))
    for i, row := range rows {
        concepts[i] = ConceptFreq{
            Concept: row.Concept,
            Count:   int(row.Freq),
        }
    }
    return &PatternSummary{
        Project:      project,
        TopConcepts:  concepts,
        ToolUsage:    make([]ToolFreq, 0),
        FilePatterns: make([]FilePattern, 0),
        SessionCount: 0,
        GeneratedAt:  time.Now().UTC(),
    }, nil
}
```

**Note:** Verify `GetConceptFrequenciesRow` field types after `sqlc generate`. `Concept` is likely `string` (from `unnest`), `Freq` is likely `int64` (from `count(*)`). If `Freq` is `int64`, the cast `int(row.Freq)` is needed.

- [ ] **Step 2: Update patterns_test.go**

```go
package service

import (
    "context"
    "testing"

    "github.com/agentmemory/agentmemory/internal/store"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// mockPatternsQuerier implements patternsQuerier for testing.
type mockPatternsQuerier struct {
    rows []store.GetConceptFrequenciesRow
    err  error
}

func (m *mockPatternsQuerier) GetConceptFrequencies(ctx context.Context) ([]store.GetConceptFrequenciesRow, error) {
    if m.err != nil {
        return nil, m.err
    }
    return m.rows, nil
}

func TestPatterns_DetectPatterns_ReturnsConceptFrequencies(t *testing.T) {
    mock := &mockPatternsQuerier{
        rows: []store.GetConceptFrequenciesRow{
            {Concept: "authentication", Freq: 10},
            {Concept: "database", Freq: 7},
            {Concept: "logging", Freq: 3},
        },
    }
    svc := newPatternsServiceWithQuerier(mock)

    summary, err := svc.DetectPatterns(context.Background(), "my-project")
    require.NoError(t, err)
    require.NotNil(t, summary)
    assert.Equal(t, "my-project", summary.Project)
    assert.Len(t, summary.TopConcepts, 3)
    assert.Equal(t, "authentication", summary.TopConcepts[0].Concept)
    assert.Equal(t, 10, summary.TopConcepts[0].Count)
    assert.Empty(t, summary.ToolUsage)
    assert.Empty(t, summary.FilePatterns)
    assert.Equal(t, 0, summary.SessionCount)
}

func TestPatterns_DetectPatterns_EmptyResult(t *testing.T) {
    mock := &mockPatternsQuerier{
        rows: []store.GetConceptFrequenciesRow{},
    }
    svc := newPatternsServiceWithQuerier(mock)

    summary, err := svc.DetectPatterns(context.Background(), "empty-project")
    require.NoError(t, err)
    require.NotNil(t, summary)
    assert.Empty(t, summary.TopConcepts)
    assert.Equal(t, "empty-project", summary.Project)
}

func TestPatterns_DetectPatterns_ErrorPropagation(t *testing.T) {
    mock := &mockPatternsQuerier{
        err: assert.AnError,
    }
    svc := newPatternsServiceWithQuerier(mock)

    _, err := svc.DetectPatterns(context.Background(), "fail-project")
    require.Error(t, err)
    assert.ErrorIs(t, err, assert.AnError)
}
```

- [ ] **Step 3: Run patterns tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/ -run TestPatterns -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/patterns.go internal/service/patterns_test.go
git commit -m "refactor: migrate patterns.go raw SQL to sqlc store (#162)"
```

---

### Task 11: Update callers in mcp/tools.go

**Files:**
- Modify: `internal/mcp/tools.go`

- [ ] **Step 1: Update constructor calls**

Change lines 81-94. Replace:
```go
consolidator := service.NewConsolidationService(pool, llmSvc, mode)
reflector := service.NewReflectionService(pool, 3600)
...
evictSvc := service.NewEvictionService(pool)
...
fileHistorySvc := service.NewFileHistoryService(pool)
patternsSvc := service.NewPatternsService(pool)
```

with:
```go
consolidator := service.NewConsolidationService(store.New(pool), llmSvc, mode)
reflector := service.NewReflectionService(store.New(pool), 3600)
...
evictSvc := service.NewEvictionService(store.New(pool))
...
fileHistorySvc := service.NewFileHistoryService(store.New(pool))
patternsSvc := service.NewPatternsService(store.New(pool))
```

- [ ] **Step 2: Verify import**

Ensure `"github.com/agentmemory/agentmemory/internal/store"` is in the imports of `tools.go`. If not, add it.

- [ ] **Step 3: Verify compilation**

```bash
cd /home/admin/work/github/agentmemory && go build ./internal/mcp/...
```

Expected: Compiles.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/tools.go
git commit -m "refactor: update mcp/tools.go constructors to use *store.Queries (#162)"
```

---

### Task 12: Full test suite verification

**Files:**
- All modified files

- [ ] **Step 1: Run all unit tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/service/... -v
```

Expected: All tests PASS.

- [ ] **Step 2: Run store package tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./internal/store/... -v
```

Expected: All tests PASS.

- [ ] **Step 3: Run all integration tests**

```bash
cd /home/admin/work/github/agentmemory && go test ./tests/integration/... -v
```

Expected: All tests PASS.

- [ ] **Step 4: Full project build**

```bash
cd /home/admin/work/github/agentmemory && go build ./...
```

Expected: No errors.

- [ ] **Step 5: Commit if any final adjustments needed**

```bash
git add -A && git diff --cached --stat
git commit -m "chore: final verification after raw SQL migration (#162)"
```

---

## Known Adjustments Needed After sqlc generate

The generated type and field names depend on sqlc's parameter naming. The following may differ from the plan and need adjustment at implementation time:

1. **`ListEvictionCandidatesParams`** — Fields likely `Column1` (float64) and `Limit` (int32). If sqlc generates positional names, use those.
2. **`GetFileHistoryParams`** — Fields likely `Column1` ([]string) and `Column2` (string).
3. **`GetConceptFrequenciesRow`** — `Concept` (string), `Freq` (int64). Verify `Freq` type for casting.
4. **`ListAllMemories`** — Returns `[]store.Memory` (uses existing model type from `memories.sql`).
5. **`BatchInsertMemoriesParams`** — `OwnerUserID` and `OwnerTeamID` may be `*string` (nullable). Verify.
6. **`BatchInsertLessonsParams`** — `TeamID` and `Context` may be `*string` (nullable). Verify.
