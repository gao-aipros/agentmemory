# AgentMemory v2 Blueprint

Platform migration: TypeScript/Node.js + SQLite/in-memory -> Go + PostgreSQL (pgvector + pg_search via ParadeDB).
Preserve v0's multi-level pipeline (observe -> compress -> summarize -> consolidate -> reflect),
dual Observation/Action pipelines, and dual Context/Search consumer lines.
Single observation table with BM25+HNSW+B-tree, hybrid smart search in one SQL FULL OUTER JOIN.

## Key Decisions

- Tech stack: Go 1.26.4, chi, pgxpool+sqlc, slog, golang-migrate, testify+testcontainers
- Database: ParadeDB (`paradedb/paradedb`), pg_search + pgvector, single observation table
- Smart search: three-way SQL FULL OUTER JOIN (BM25 0.4, Vector 0.6, Graph 0.3)
- Team/User: first-class PG entities, row-level ownership, three visibility modes
- Visibility: observations/compressed/summaries/crystals = private, lessons = team, memory = configurable
- Context injection: 1500 token hard limit, 5 source buckets
- Exit/re-join: DELETE from team_members, full history on re-join
- Single port HTTP, Bearer st_/ak_ token prefixes

## MCP Tools

- All 51 v1 tools migrated to v2
- New tools for: team management, auth UI operations

## REST API

- Hooks -> REST API (plugin hook scripts call REST endpoints)
- MCP tools -> MCP Streamable HTTP (agent-to-agentmemory communication)
- Auth endpoints: /v1/auth/login, /v1/auth/keys
- Team management endpoints

## CLI

- All v1 commands migrated (connect, viewer, serve, etc.)
- New commands: team management, schema migrate, setup/init

## Data Migration

- No v1 -> v2 data migration. Breaking change, fresh start.

## WebSocket

- /v1/socket with st_ auth for viewer live updates
- Protocol follows v1 behavior

## Testing

- Integration-test-heavy: testcontainers with real PostgreSQL
- Focus on DB behavior correctness

## References

Detailed design in companion files:
- 01-tech-stack.md
- 02-project-layout.md
- 03-core-architecture.md
- 04-smart-search.md
- 05-provider.md
- 06-team-user.md
- 07-pipeline.md
- 08-hooks.md
- 09-routing.md
- 10-schema-ddl.sql

## Remaining for Future

- Many-to-many user-team
- Apache AGE graph
- Pipeline inter-connections (observation->action auto-derivation)
- Crystals/auto timer fallback
- ProceduralMemory consumer

v0 source (living spec): https://github.com/Noodle05/agentmemory
