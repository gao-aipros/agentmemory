# Implementation Plan: AgentMemory v2 Platform Migration

**Branch**: `001-agentmemory-v2-platform` | **Date**: 2026-06-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-agentmemory-v2-platform/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command.

## Summary

Full platform migration of agentmemory from TypeScript/Node.js + SQLite/in-memory
to Go + PostgreSQL (ParadeDB: pgvector + pg_search). Preserve v0's multi-level
pipeline (observe → compress → summarize → consolidate → reflect), dual
Observation/Action pipelines, and dual Context/Search consumption lines. Add
first-class User/Team entities with row-level ownership and three visibility
modes. Deliver all 51 v1 MCP tools plus new team/auth tools on a single Go
binary with single-port HTTP.

## Technical Context

**Language/Version**: Go 1.26.4

**Primary Dependencies**: chi (HTTP router), pgx + pgxpool (PostgreSQL driver),
sqlc (type-safe SQL codegen), golang-migrate (schema migration),
langchaingo (LLM/embedding abstraction), modelcontextprotocol/go-sdk (MCP server),
testify + testcontainers-go (testing)

**Storage**: PostgreSQL via ParadeDB `paradedb/paradedb:0.24.1-pg18` with
pg_search (BM25) + pgvector (HNSW) extensions. Graph traversal via native
`WITH RECURSIVE` CTE. 25 tables, 42 indexes per `10-schema-ddl.sql`.

**Testing**: testify for assertions, testcontainers-go for ephemeral ParadeDB
containers. Both unit tests (fast, isolated business logic) and integration tests
(real PostgreSQL, BM25, HNSW, CTE) are mandatory per constitution Principle III.

**Target Platform**: Linux server (Docker container). Single binary deployment.

**Project Type**: Web service (single-port HTTP: REST API + MCP Streamable HTTP +
WebSocket + static files).

**Performance Goals**:
- Hybrid search (BM25 + vector + graph): <500ms for 100k observations
- MCP tool read ops: p95 <200ms, write ops with embedding: p95 <500ms
- Server startup to health check: <10s with pre-migrated database
- Schema migration (full DDL): <30s

**Constraints**:
- 1500 token hard limit on context injection (~1100 sources, ~400 overhead)
- Single HTTP port for all traffic
- Zero raw SQL in Go code (all queries through sqlc)
- Observations always private; sharing only at Memory layer
- env vars for all configuration

**Scale/Scope**:
- 51 v1 MCP tools migrated + new team/auth tools
- 13 hook types capturing agent session events
- 25 tables, 42 indexes
- 3 team operational modes, 3 visibility levels
- Deferred: many-to-many user-team, Apache AGE, pipeline inter-connections, cross-instance replication

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Evidence |
|-----------|--------|----------|
| I. Superpowers Workflow | ✅ PASS | Plan follows worktree → TDD → subagent → review → finish-branch; tasks.md will encode this sequence |
| II. Pipeline Integrity | ✅ PASS | FR-001 through FR-008 require full pipeline; 10-schema-ddl.sql preserves all stages |
| III. Test-First (Unit + Integration) | ✅ PASS | Both test tiers mandatory; testcontainers for real ParadeDB; testify for assertions |
| IV. Type-Safe Data Access | ✅ PASS | sqlc for all queries; zero raw SQL in application code; queries in `internal/db/queries/` |
| V. Provider Agnosticism | ✅ PASS | langchaingo interfaces; providers selected via env vars |
| VI. Single Binary Simplicity | ✅ PASS | Single Go binary, single port, Docker deployment; no microservices, queues, or external caches |

**Gate result**: ALL PASS — no violations to justify.

## Project Structure

### Documentation (this feature)

```text
specs/001-agentmemory-v2-platform/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (MCP tool contracts, REST API contracts)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

```text
cmd/agentmemory/main.go          # Entry point
internal/
  handler/                        # HTTP handlers (REST + MCP)
    rest.go                       # /v1/api/* handlers
    mcp.go                        # /v1/mcp StreamableHTTP handler
    auth.go                       # /v1/auth/* handlers
    ws.go                         # /v1/socket WebSocket handler
  service/                        # Business logic
    observe.go                    # Observation recording
    compress.go                   # Compression (async)
    summarize.go                  # Session summarization
    consolidate.go                # Memory consolidation
    reflect.go                    # Pattern reflection
    search.go                     # Hybrid search engine
    context.go                    # Context injection assembly
    lessons.go                    # Lesson management
    evict.go                      # Storage eviction
    team.go                       # Team management
    user.go                       # User management
    auth.go                       # Authentication
  store/                          # sqlc-generated DB access (never hand-edit)
  db/queries/                     # .sql query files for sqlc
    observations.sql
    sessions.sql
    search.sql
    users.sql
    teams.sql
    lessons.sql
    graph.sql
  mcp/                            # MCP tool registration
    tools.go                       # All 51+ tool definitions
  hooks/                          # Plugin hook script templates
  auth/                           # JWT + API key implementation
  team/                           # Team logic (modes, visibility)
  config/                         # env-var parsing
  cmd/                            # CLI subcommand implementations
    setup.go                      # agentmemory setup
    serve.go                      # agentmemory serve
    migrate.go                    # agentmemory migrate
    user.go                       # agentmemory user create
    connect.go                    # agentmemory connect
    team.go                       # agentmemory team
migrations/                       # golang-migrate DDL files
  001_initial_schema.up.sql
  001_initial_schema.down.sql
tests/
  unit/                           # Fast isolated tests
  integration/                    # testcontainers ParadeDB tests
```

**Structure Decision**: Single project (Go standard layout). `cmd/` for entry point,
`internal/` for all application code (not importable externally), `migrations/` for
DDL, `tests/` at root. Follows Go community conventions and the v2 project layout
specified in `docs/specs/02-project-layout.md`.

## Complexity Tracking

> No constitution violations — table intentionally left empty.
