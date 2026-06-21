# AgentMemory v2 Blueprint

Platform migration: TypeScript/Node.js + SQLite/in-memory → Go + PostgreSQL (pgvector + pg_search via ParadeDB).
Preserve v0's multi-level pipeline (observe → compress → summarize → consolidate → reflect),
dual Observation/Action pipelines, and dual Context/Search consumer lines.
Add first-class User/Team entities with row-level ownership and three visibility modes.
Fix v0 bugs (Stop hook per-turn lifecycle, force-directed graph damping, etc.).

Single observation table with BM25+HNSW+B-tree, hybrid smart search in one SQL FULL OUTER JOIN.

## Key Decisions

- Tech stack: Go 1.26.4, chi, pgxpool+sqlc, slog, golang-migrate, testify+testcontainers
- Database: ParadeDB `paradedb/paradedb:latest` (PG18), pg_search + pgvector extensions only
- Smart search: three-way SQL FULL OUTER JOIN (BM25 0.4, Vector 0.6, Graph 0.3)
- Team/User: first-class PG entities, row-level ownership, three visibility modes
- Visibility: observations/compressed/summaries/crystals = private, lessons = team, memory = configurable
- Context injection: 1500 token hard limit, 5 source buckets, reference format with recall IDs
- Exit/re-join: DELETE from team_members, full history on re-join
- Single port HTTP, Bearer st_/ak_ token prefixes

## MCP Tools

- All 51 v1 tools migrated to v2
- New tools for: team CRUD, team member management, auth key management
- v1 tools registry reference: `src/mcp/tools-registry.ts` in v0 source

## REST API

- Hooks → REST API (plugin hook scripts call REST endpoints via HTTP)
- MCP tools → MCP Streamable HTTP (agent-to-agentmemory communication)
- Auth endpoints: /v1/auth/login (with TOTP), /v1/auth/keys (API key management)
- Team management endpoints via REST

## CLI

- All v1 commands migrated (connect, viewer, serve, etc.)
- New commands: `team` (team management), `migrate` (schema migration), `setup` (first-time init)

## Data Migration

- No v1 → v2 data migration. Breaking change, fresh start.

## WebSocket

- /v1/socket with st_ auth for viewer live updates
- Protocol follows v1 viewer behavior (see v0 `src/viewer/index.html`)

## Testing

- Integration-test-heavy: testcontainers with real ParadeDB PostgreSQL
- Focus on DB behavior correctness
- testify for assertions

## Protocol Architecture

```
Plugin Hooks (shell scripts) → REST API (/v1/api/* + /v1/auth/*)
Agent MCP Tools               → MCP Streamable HTTP (/v1/mcp)
SPA Viewer                    → static files (/), WebSocket (/v1/socket)
```

## Detailed Specs

Each topic has a dedicated file with full technical detail:

- 01-tech-stack.md — complete stack decisions with rationale
- 02-project-layout.md — directory structure, package responsibilities
- 03-core-architecture.md — ParadeDB, sqlc, MCP integration, design rationale
- 04-smart-search.md — SQL FULL OUTER JOIN, execution flow, weights, key decisions
- 05-provider.md — langchaingo interfaces, embedding + LLM config, provider list
- 06-team-user.md — entity model, visibility rules, three modes, membership lifecycle, search logic, context injection scope
- 07-pipeline.md — observe→compress→summarize→consolidate→reflect, data flow, context injection, v0 living spec rule, deferred gaps
- 08-hooks.md — 13 hooks with full behavior table, key design decisions, protocol architecture
- 09-routing.md — route table, token format and scope, Docker deployment, WebSocket
- 10-schema-ddl.sql — complete PostgreSQL DDL: 25 tables, 42 indexes
- 11-scope-decisions.md — early scope decisions (historical record, Team/User was TBD at this point)
- 12-database-image.md — ParadeDB Docker image details, extensions, AGE vs CTE
- 13-team-user-final.md — final team/user architecture with escape hatch and Chinese version

## Remaining for Future

- Many-to-many user-team
- Apache AGE graph (currently using PG WITH RECURSIVE CTE)
- Pipeline inter-connections (observation → action auto-derivation)
- Crystals/auto timer fallback
- ProceduralMemory consumer

## v0 Source

https://github.com/Noodle05/agentmemory — living behavioral spec for v2.
