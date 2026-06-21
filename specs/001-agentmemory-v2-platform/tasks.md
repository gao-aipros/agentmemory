# Tasks: AgentMemory v2 Platform Migration

**Input**: Design documents from `specs/001-agentmemory-v2-platform/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Tests are MANDATORY per constitution Principle III (Test-First Unit + Integration).
Both unit tests and integration tests (real ParadeDB PostgreSQL via testcontainers) are required.
Write tests FIRST, verify they FAIL, then implement.

**Integration Test Infrastructure**: ALL integration tests MUST use the ParadeDB Docker image
`paradedb/paradedb:0.24.1-pg18` via testcontainers-go. No mocks, no SQLite, no embedded databases.
The testcontainers helper MUST pull and start this exact image version for deterministic builds.
See `tests/integration/testhelper.go` (T008).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Go project**: `cmd/agentmemory/`, `internal/`, `migrations/`, `tests/`
- `internal/handler/` — HTTP handlers (REST + MCP)
- `internal/service/` — Business logic
- `internal/store/` — sqlc-generated (never hand-edit)
- `internal/db/queries/` — .sql query files for sqlc
- `tests/unit/` — Fast isolated tests
- `tests/integration/` — testcontainers ParadeDB tests

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [ ] T001 Create Go module and project directory structure per plan.md in `cmd/agentmemory/`, `internal/`, `migrations/`, `tests/`
- [ ] T002 [P] Initialize Go module with dependencies: chi, pgx, pgxpool, sqlc, golang-migrate, langchaingo, testify, testcontainers-go in `go.mod`
- [ ] T003 [P] Configure golang-migrate and create initial migration scaffolding in `migrations/`
- [ ] T004 [P] Create env var configuration package with all v2 settings in `internal/config/config.go`
- [ ] T005 [P] Create structured logging setup (slog → stdout) with level configuration in `internal/config/logging.go`
- [ ] T006 [P] Configure sqlc codegen with pgx driver and output to `internal/store/` in `sqlc.yaml`
- [ ] T007 [P] Write unit tests for config parsing and env var defaults in `tests/unit/config_test.go`
- [ ] T008 Create ParadeDB testcontainers helper that pulls and starts `paradedb/paradedb:0.24.1-pg18`, enables pg_search+pgvector extensions, returns connection string in `tests/integration/testhelper.go`

**Checkpoint**: Project compiles, config loads, sqlc generates, testcontainers helper works

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T009 Create initial DDL migration: users, api_keys, teams, team_members tables in `migrations/001_initial_schema.up.sql`
- [ ] T010 [P] Create sqlc query file for users CRUD (insert, select by id, select by email, update, delete) in `internal/db/queries/users.sql`
- [ ] T011 [P] Create sqlc query file for api_keys CRUD (insert, select by id, select by user, update last_used, delete) in `internal/db/queries/api_keys.sql`
- [ ] T012 [P] Create sqlc query file for sessions CRUD (insert, select, update status, update ended_at) in `internal/db/queries/sessions.sql`
- [ ] T013 Run sqlc generate and verify generated code compiles in `internal/store/`
- [ ] T014 Create pgxpool connection manager with pool config, health check, and graceful shutdown in `internal/config/database.go`
- [ ] T015 [P] Create chi router setup with middleware (logging, recovery, auth) in `internal/handler/router.go`
- [ ] T016 [P] Create base HTTP server with graceful shutdown, health check handler in `cmd/agentmemory/main.go`
- [ ] T017 [P] Write unit tests for database connection manager (pool config validation, DSN parsing) in `tests/unit/database_test.go`
- [ ] T018 Write integration tests for schema migration (all tables created, indexes exist, constraints enforced) in `tests/integration/schema_test.go`

**Checkpoint**: Foundation ready — database migrations run, sqlc generates, server starts, health check responds. User story implementation can now begin.

---

## Phase 3: User Story 1 - Memory Pipeline Core (Priority: P1) 🎯 MVP

**Goal**: Capture agent session events via 13 hooks, compress observations, summarize sessions, consolidate memories, and reflect on patterns.

**Independent Test**: Run a simulated agent session through all 13 hook events. Verify observations are recorded, compressed within 30 seconds, and session summaries appear after SessionEnd fires.

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T019 [P] [US1] Unit test for observation type validation and importance range (0.0-1.0) in `tests/unit/observation_test.go`
- [ ] T020 [P] [US1] Unit test for compression prompt assembly (observation → LLM prompt) in `tests/unit/compress_test.go`
- [ ] T021 [P] [US1] Unit test for summarization chunking logic (token budget overflow) in `tests/unit/summarize_test.go`
- [ ] T022 [P] [US1] Unit test for consolidation input assembly (session summary → memory extraction) in `tests/unit/consolidate_test.go`
- [ ] T023 [P] [US1] Unit test for reflection clustering algorithm in `tests/unit/reflect_test.go`
- [ ] T024 [US1] Integration test for full pipeline: observe → compress → searchable in `tests/integration/pipeline_test.go`
- [ ] T025 [US1] Integration test for SessionEnd: summarize + consolidate + reflect triggered in `tests/integration/session_end_test.go`
- [ ] T026 [US1] Integration test for all 13 hook event types recorded correctly in `tests/integration/hooks_test.go`

### Implementation for User Story 1

#### Database Layer

- [ ] T027 [P] [US1] Create DDL migration: observations table with BM25+HNSW+B-tree indexes in `migrations/002_observations.up.sql`
- [ ] T028 [P] [US1] Create DDL migration: observation_embeddings table with HNSW partial index in `migrations/003_embeddings.up.sql`
- [ ] T029 [P] [US1] Create DDL migration: compressed_observations + compressed_embeddings tables in `migrations/004_compressed.up.sql`
- [ ] T030 [P] [US1] Create DDL migration: session_summaries table in `migrations/005_summaries.up.sql`
- [ ] T031 [P] [US1] Create DDL migration: memories, lessons, lesson_reinforcements tables in `migrations/006_memories.up.sql`
- [ ] T032 [US1] Create sqlc queries for observations (insert, select by session, select by id, delete) with ParadeDB bm25 syntax in `internal/db/queries/observations.sql`
- [ ] T033 [US1] Create sqlc queries for observation_embeddings (insert, select by observation_id, delete) in `internal/db/queries/embeddings.sql`
- [ ] T034 [US1] Create sqlc queries for compressed_observations (insert, select, delete) in `internal/db/queries/compressed.sql`
- [ ] T035 [US1] Create sqlc queries for session_summaries (insert, upsert, select by session) in `internal/db/queries/summaries.sql`
- [ ] T036 [US1] Create sqlc queries for memories (insert, select, update visibility, delete) in `internal/db/queries/memories.sql`
- [ ] T037 [US1] Create sqlc queries for lessons (insert, select, update confidence, delete) in `internal/db/queries/lessons.sql`
- [ ] T038 [US1] Run sqlc generate and verify all observation pipeline queries compile

#### Observe Service

- [ ] T039 [US1] Implement observation recording: validate type, importance, visibility=private CHECK enforcement in `internal/service/observe.go`
- [ ] T040 [US1] Implement session management: create session (SessionStart), update ended_at (SessionEnd), query active session in `internal/service/session.go`
- [ ] T041 [US1] Implement hook event type mapping (13 hook types → observation types) in `internal/service/hook_types.go`

#### Compression Service

- [ ] T042 [US1] Implement compression async goroutine: trigger on observe, non-blocking, 30s target in `internal/service/compress.go`
- [ ] T043 [US1] Implement compression LLM call: observation → compressed summary via langchaingo in `internal/service/compress_llm.go`
- [ ] T044 [US1] Implement embedding generation: compressed text → vector via langchaingo embedder in `internal/service/embed.go`

#### Summarization Service

- [ ] T045 [US1] Implement session summarization: gather all session observations, chunk if over token budget, merge in `internal/service/summarize.go`
- [ ] T046 [US1] Implement SessionEnd handler: close session, trigger summarize + consolidate + reflect in `internal/service/session_end.go`

#### Consolidation Service

- [ ] T047 [US1] Implement consolidation: session summary → extract SemanticMemory, Lessons, Insights in `internal/service/consolidate.go`
- [ ] T048 [US1] Implement memory consolidation auto-mode (private/team/public visibility per config) in `internal/service/consolidate_mode.go`
- [ ] T049 [US1] Implement lesson strength tracking: confidence boost on reinforce, decay on non-use in `internal/service/lessons.go`

#### Reflection Service

- [ ] T050 [US1] Implement reflection timer: periodic async run (configurable interval) in `internal/service/reflect.go`
- [ ] T051 [US1] Implement reflection clustering: group related memories, detect patterns, synthesize insights in `internal/service/reflect_cluster.go`

#### Eviction Service

- [ ] T052 [US1] Implement eviction: prioritize low-importance, old observations; preserve compressed + lessons in `internal/service/evict.go`

#### REST Handler (Pipeline)

- [ ] T053 [US1] Implement POST `/v1/api/observe` handler (validate input, call observe service) in `internal/handler/rest.go`
- [ ] T054 [US1] Implement POST `/v1/api/session/end` handler (call session end service) in `internal/handler/rest.go`
- [ ] T055 [US1] Implement POST `/v1/api/session/commit` handler (link git commit to session) in `internal/handler/rest.go`

**Checkpoint**: Full pipeline functional — observe → compress → searchable works end-to-end. All 13 hooks produce valid observations. SessionEnd triggers full consolidation chain.

---

## Phase 4: User Story 2 - Smart Search & Context Injection (Priority: P2)

**Goal**: Hybrid search combining BM25, vector, and graph in a single SQL query. Context injection with 1500-token budget from 5 source buckets.

**Independent Test**: Populate DB with known observations. Execute `memory_recall` and verify combined BM25+vector+graph ranking. Trigger SessionStart with `AGENTMEMORY_INJECT_CONTEXT=true` and verify context within 1500 tokens.

### Tests for User Story 2

- [ ] T056 [P] [US2] Unit test for search weight normalization (BM25 0.4, vector 0.6, graph 0.3) in `tests/unit/search_test.go`
- [ ] T057 [P] [US2] Unit test for context injection budget allocation (~1100 sources, ~400 overhead) in `tests/unit/context_test.go`
- [ ] T058 [P] [US2] Unit test for context injection source priority truncation in `tests/unit/context_priority_test.go`
- [ ] T059 [US2] Integration test for hybrid search: BM25 + vector + graph combined ranking in `tests/integration/search_hybrid_test.go`
- [ ] T060 [US2] Integration test for progressive disclosure: compact → expandable results in `tests/integration/search_progressive_test.go`
- [ ] T061 [US2] Integration test for context injection: 1500-token budget respected with real data in `tests/integration/context_injection_test.go`

### Implementation for User Story 2

#### Graph Schema

- [ ] T062 [P] [US2] Create DDL migration: graph_nodes + graph_edges tables in `migrations/007_graph.up.sql`
- [ ] T063 [P] [US2] Create sqlc queries for graph_nodes (insert, select, delete) in `internal/db/queries/graph_nodes.sql`
- [ ] T064 [P] [US2] Create sqlc queries for graph_edges (insert, select, delete) in `internal/db/queries/graph_edges.sql`

#### Search Engine

- [ ] T065 [US2] Implement BM25 search query: ParadeDB `USING bm25` on (id, title, narrative, facts) in `internal/db/queries/search.sql`
- [ ] T066 [US2] Implement vector search query: pgvector `<->` cosine distance with HNSW index in `internal/db/queries/search.sql`
- [ ] T067 [US2] Implement graph traversal query: `WITH RECURSIVE` CTE on graph_nodes/edges in `internal/db/queries/search.sql`
- [ ] T068 [US2] Implement hybrid search FULL OUTER JOIN: BM25 (0.4) + vector (0.6) + graph (0.3) weighted merge in `internal/db/queries/search.sql`
- [ ] T069 [US2] Run sqlc generate and verify search queries compile
- [ ] T070 [US2] Implement search service: embed query → execute hybrid SQL → rank → return in `internal/service/search.go`
- [ ] T071 [US2] Implement progressive disclosure: compact results first, expandable by ID in `internal/service/search_progressive.go`
- [ ] T072 [US2] Implement `memory_recall` service: full hybrid search with format options in `internal/service/recall.go`
- [ ] T073 [US2] Implement `memory_smart_search` service: hybrid + expand IDs in `internal/service/smart_search.go`

#### Context Injection

- [ ] T074 [US2] Implement context injection assembly: gather 5 buckets (observations, recap, lessons, graph neighbors, working memory) in `internal/service/context.go`
- [ ] T075 [US2] Implement context budget management: 1500-token hard limit, priority truncation (graph → lessons → observations → recap) in `internal/service/context_budget.go`
- [ ] T076 [US2] Implement context injection reference format with recall IDs in `internal/service/context_format.go`
- [ ] T077 [US2] Implement 3-hook context injection triggers: SessionStart, PreToolUse (file-specific), PreCompact in `internal/service/context_hooks.go`
- [ ] T078 [US2] Implement conditional context injection gate (`AGENTMEMORY_INJECT_CONTEXT` env var) in `internal/service/context_gate.go`

**Checkpoint**: Hybrid search returns combined BM25+vector+graph results. Context injection delivers 1500-token summary on SessionStart. PreToolUse enrichment works with file paths.

---

## Phase 5: User Story 3 - Team & User Management (Priority: P3)

**Goal**: First-class User and Team entities with row-level ownership, three visibility modes, and three team operational modes. JWT + API key authentication.

**Independent Test**: Create Alice and Bob. Alice creates team, invites Bob. Alice's observations are private. Both see shared team memories. Bob leaves; private data stays, team memories persist.

### Tests for User Story 3

- [ ] T079 [P] [US3] Unit test for JWT token generation, validation, and expiry in `tests/unit/auth_jwt_test.go`
- [ ] T080 [P] [US3] Unit test for API key hash generation and `ak_` prefix validation in `tests/unit/auth_apikey_test.go`
- [ ] T081 [P] [US3] Unit test for visibility rules engine (private/team/public matrix) in `tests/unit/visibility_test.go`
- [ ] T082 [P] [US3] Unit test for team operational mode logic (owned-by-user, owned-by-team, member-choice) in `tests/unit/team_modes_test.go`
- [ ] T083 [US3] Integration test for user creation → login → JWT → API key lifecycle in `tests/integration/auth_flow_test.go`
- [ ] T084 [US3] Integration test for team membership lifecycle: create → invite → leave → re-join in `tests/integration/team_lifecycle_test.go`
- [ ] T085 [US3] Integration test for row-level ownership: cross-user visibility enforcement in `tests/integration/ownership_test.go`

### Implementation for User Story 3

#### Auth Foundation

- [ ] T086 [US3] Implement password hashing (bcrypt) and verification in `internal/auth/password.go`
- [ ] T087 [US3] Implement JWT generation (st_ prefix), validation, and expiry (configurable via JWT_EXPIRY) in `internal/auth/jwt.go`
- [ ] T088 [US3] Implement API key generation (ak_ prefix), hashing, and validation in `internal/auth/apikey.go`
- [ ] T089 [US3] Implement TOTP setup, verification, and enable/disable in `internal/auth/totp.go`

#### Auth Handlers

- [ ] T090 [US3] Implement POST `/v1/auth/login` handler: email + password + optional TOTP → JWT in `internal/handler/auth.go`
- [ ] T091 [US3] Implement GET/POST/DELETE `/v1/auth/keys` handlers: list, create, revoke API keys in `internal/handler/auth.go`
- [ ] T092 [US3] Implement auth middleware: extract Bearer token, validate st_/ak_, inject user context in `internal/handler/middleware_auth.go`
- [ ] T093 [US3] Implement route-level token scope enforcement (ak_ rejected for UI routes) in `internal/handler/middleware_auth.go`

#### User Management

- [ ] T094 [US3] Implement user service: create, select, update, delete, list in `internal/service/user.go`

#### Team Management

- [ ] T095 [US3] Implement team service: create, select, update, delete (owner only) in `internal/service/team.go`
- [ ] T096 [US3] Implement team membership: add member, remove member, list members, exit/re-join pattern in `internal/service/team_members.go`
- [ ] T097 [US3] Implement three team operational modes: Owned by User, Owned by Team, Member Choice in `internal/service/team_modes.go`
- [ ] T098 [US3] Implement visibility resolution engine: row-level `owner_type` + `owner_user_id` + `owner_team_id` + `visibility` → access decision in `internal/service/visibility.go`

#### Row-Level Ownership

- [ ] T099 [US3] Add owner_type, owner_user_id, owner_team_id, visibility columns to observations queries in `internal/db/queries/observations.sql`
- [ ] T100 [US3] Add owner_type, owner_user_id, owner_team_id, visibility columns to memories queries in `internal/db/queries/memories.sql`
- [ ] T101 [US3] Implement ownership injection on observe: set owner from authenticated user context in `internal/service/observe.go`
- [ ] T102 [US3] Implement visibility filtering: all read queries scope by user's teams + visibility level in `internal/service/visibility_filter.go`

**Checkpoint**: Users can create accounts, log in, get JWT, generate API keys. Teams can be created with members. Row-level ownership enforced on all observations and memories.

---

## Phase 6: User Story 4 - MCP Tools & REST API (Priority: P4)

**Goal**: All 44 MCP tools and REST API endpoints serving agent-to-agentmemory communication. MCP Streamable HTTP at `/v1/mcp`, REST at `/v1/api/*`.

**Independent Test**: Start server. Connect MCP client to `/v1/mcp`, call `memory_observe`, then `memory_recall`. Verify REST API accepts hook-equivalent calls. Verify st_ token grants access, invalid token returns 401.

### Tests for User Story 4

- [ ] T103 [P] [US4] Unit test for MCP tool parameter validation (required fields, types) in `tests/unit/mcp_validation_test.go`
- [ ] T104 [P] [US4] Unit test for REST request deserialization and error responses in `tests/unit/rest_test.go`
- [ ] T105 [US4] Integration test for full MCP tool lifecycle: observe → compress → recall → forget in `tests/integration/mcp_tools_test.go`
- [ ] T106 [US4] Integration test for REST API hook endpoints: all 13 hook POSTs accepted in `tests/integration/rest_hooks_test.go`
- [ ] T107 [US4] Integration test for authentication: valid st_ → 200, invalid → 401, ak_ rejected for UI in `tests/integration/auth_routing_test.go`

### Implementation for User Story 4

#### MCP Server

- [ ] T108 [US4] Initialize MCP StreamableHTTP server with modelcontextprotocol/go-sdk at `/v1/mcp` in `internal/handler/mcp.go`
- [ ] T109 [US4] Configure MCP server: `NewStreamableHTTPHandler`, `JSONResponse: true` in `internal/handler/mcp.go`

#### MCP Tools - Memory Operations

- [ ] T110 [P] [US4] Register `memory_observe` MCP tool (wrapper around observe service) in `internal/mcp/tools.go`
- [ ] T111 [P] [US4] Register `memory_save` MCP tool (explicit memory save) in `internal/mcp/tools.go`
- [ ] T112 [P] [US4] Register `memory_recall` MCP tool (hybrid search) in `internal/mcp/tools.go`
- [ ] T113 [P] [US4] Register `memory_smart_search` MCP tool (progressive disclosure) in `internal/mcp/tools.go`
- [ ] T114 [P] [US4] Register `memory_forget` MCP tool (delete observations) in `internal/mcp/tools.go`
- [ ] T115 [P] [US4] Register `memory_compress_file` MCP tool (file compression) in `internal/mcp/tools.go`

#### MCP Tools - Session Operations

- [ ] T116 [P] [US4] Register `memory_sessions` MCP tool (list sessions) in `internal/mcp/tools.go`
- [ ] T117 [P] [US4] Register `memory_timeline` MCP tool (chronological query) in `internal/mcp/tools.go`
- [ ] T118 [P] [US4] Register `memory_handoff` MCP tool (resume latest session) in `internal/mcp/tools.go`
- [ ] T119 [P] [US4] Register `memory_recap` MCP tool (summarize N sessions) in `internal/mcp/tools.go`

#### MCP Tools - Lesson Operations

- [ ] T120 [P] [US4] Register `memory_lesson_save` MCP tool in `internal/mcp/tools.go`
- [ ] T121 [P] [US4] Register `memory_lesson_recall` MCP tool in `internal/mcp/tools.go`

#### MCP Tools - Team Operations

- [ ] T122 [P] [US4] Register `team_create`, `team_delete` MCP tools in `internal/mcp/tools.go`
- [ ] T123 [P] [US4] Register `team_add_member`, `team_remove_member` MCP tools in `internal/mcp/tools.go`
- [ ] T124 [P] [US4] Register `team_list_members`, `team_feed` MCP tools in `internal/mcp/tools.go`

#### MCP Tools - Auth Operations

- [ ] T125 [P] [US4] Register `auth_create_key`, `auth_list_keys`, `auth_revoke_key` MCP tools in `internal/mcp/tools.go`

#### MCP Tools - Action Operations

- [ ] T126 [P] [US4] Register `memory_action_create`, `memory_action_update` MCP tools in `internal/mcp/tools.go`
- [ ] T127 [P] [US4] Register `memory_frontier`, `memory_next` MCP tools in `internal/mcp/tools.go`

#### MCP Tools - Pipeline + Governance + Export + Graph + Context

- [ ] T128 [P] [US4] Register `memory_consolidate`, `memory_crystallize`, `memory_reflect` MCP tools in `internal/mcp/tools.go`
- [ ] T129 [P] [US4] Register `memory_diagnose`, `memory_heal`, `memory_verify`, `memory_audit` MCP tools in `internal/mcp/tools.go`
- [ ] T130 [P] [US4] Register `memory_export`, `memory_obsidian_export`, `memory_commit_lookup`, `memory_commits`, `memory_mesh_sync` MCP tools in `internal/mcp/tools.go`
- [ ] T131 [P] [US4] Register `memory_graph_query`, `memory_relations` MCP tools in `internal/mcp/tools.go`
- [ ] T132 [P] [US4] Register `memory_profile`, `memory_patterns`, `memory_facet_query`, `memory_facet_tag`, `memory_vision_search` MCP tools in `internal/mcp/tools.go`

#### MCP Tools - v1 Service Tools (Slots, Signals, Sentinels, Sketches, etc.)

- [ ] T144 [P] [US4] Register `memory_slot_create`, `memory_slot_get`, `memory_slot_list`, `memory_slot_replace`, `memory_slot_delete`, `memory_slot_append` MCP tools in `internal/mcp/tools.go`
- [ ] T145 [P] [US4] Register `memory_signal_read`, `memory_signal_send` MCP tools in `internal/mcp/tools.go`
- [ ] T146 [P] [US4] Register `memory_sentinel_create`, `memory_sentinel_trigger` MCP tools in `internal/mcp/tools.go`
- [ ] T147 [P] [US4] Register `memory_checkpoint` MCP tool in `internal/mcp/tools.go`
- [ ] T148 [P] [US4] Register `memory_sketch_create`, `memory_sketch_promote` MCP tools in `internal/mcp/tools.go`
- [ ] T149 [P] [US4] Register `memory_routine_run` MCP tool in `internal/mcp/tools.go`
- [ ] T150 [P] [US4] Register `memory_snapshot_create` MCP tool in `internal/mcp/tools.go`
- [ ] T151 [P] [US4] Register `memory_file_history` MCP tool in `internal/mcp/tools.go`
- [ ] T152 [P] [US4] Register `memory_lease` MCP tool in `internal/mcp/tools.go`
- [ ] T153 [P] [US4] Register `memory_insight_list` MCP tool in `internal/mcp/tools.go`
- [ ] T154 [P] [US4] Register `memory_team_share`, `memory_claude_bridge_sync` MCP tools in `internal/mcp/tools.go`

#### REST API Handlers

- [ ] T155 [US4] Wire all REST endpoints into chi router: `/v1/api/*`, `/v1/auth/*` in `internal/handler/router.go`

**Checkpoint**: All 55 MCP tools (44 public v1 + 11 v1 service tools) callable via `/v1/mcp`. REST API accepts all 13 hook event POSTs. Authentication enforces token scopes correctly.

---

## Phase 7: User Story 5 - Deployment, CLI & Operations (Priority: P5)

**Goal**: Single binary Docker deployment. CLI commands for setup, serve, migrate, connect. Health check endpoint. SPA viewer with WebSocket.

**Independent Test**: Build Go binary. Run `agentmemory setup` against ParadeDB container. Run `agentmemory serve` and verify `/health` returns 200. Run `agentmemory connect` in a workspace.

### Tests for User Story 5

- [ ] T145 [P] [US5] Unit test for CLI flag parsing and subcommand routing in `tests/unit/cli_test.go`
- [ ] T146 [P] [US5] Unit test for health check logic (DB alive → 200, DB dead → 503, migrations pending → 503) in `tests/unit/health_test.go`
- [ ] T147 [US5] Integration test for `agentmemory setup` → all tables + indexes created in `tests/integration/setup_test.go`
- [ ] T148 [US5] Integration test for `agentmemory migrate` → pending migrations applied in `tests/integration/migrate_test.go`
- [ ] T149 [US5] Integration test for WebSocket viewer: connect → receive session events in `tests/integration/websocket_test.go`

### Implementation for User Story 5

#### CLI

- [ ] T150 [US5] Implement CLI entry point with cobra/pflag: `serve`, `setup`, `migrate`, `connect`, `team`, `user` subcommands in `cmd/agentmemory/main.go`
- [ ] T151 [US5] Implement `agentmemory setup` command: run all migrations, create extensions in `internal/cmd/setup.go`
- [ ] T152 [US5] Implement `agentmemory serve` command: start HTTP server with graceful shutdown in `internal/cmd/serve.go`
- [ ] T153 [US5] Implement `agentmemory migrate` command: apply pending migrations in `internal/cmd/migrate.go`
- [ ] T154 [US5] Implement `agentmemory user create` command: create admin user via CLI in `internal/cmd/user.go`
- [ ] T155 [US5] Implement `agentmemory connect` command: write MCP config to host agent settings file in `internal/cmd/connect.go`
- [ ] T156 [US5] Implement `agentmemory team` commands: CLI team management in `internal/cmd/team.go`

#### Health & Operations

- [ ] T157 [US5] Implement `/health` endpoint: DB ping → 200, failure → 503, pending migrations → 503 in `internal/handler/health.go`
- [ ] T158 [US5] Implement startup sequence: check DB, run pending migrations (if configured), start server in `cmd/agentmemory/main.go`
- [ ] T159 [US5] Implement error handling with exponential backoff retry (no circuit breaker) in `internal/service/retry.go`

#### Docker

- [ ] T160 [P] [US5] Create multi-stage Dockerfile: build Go binary → minimal runtime image in `Dockerfile`
- [ ] T161 [P] [US5] Create docker-compose.yml: agentmemory + ParadeDB PostgreSQL in `docker-compose.yml`

#### SPA Viewer & WebSocket

- [ ] T162 [US5] Implement embedded SPA static file serving (v1 viewer) at `/` in `internal/handler/viewer.go`
- [ ] T163 [US5] Implement WebSocket handler at `/v1/socket` with st_ auth for viewer live updates in `internal/handler/ws.go`

**Checkpoint**: Full deployment: `docker-compose up` → setup → serve → health OK. CLI all commands functional. Viewer loads with live WebSocket updates.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T164 [P] Implement auto-forget with contradiction detection (v1 feature: when new observation contradicts existing memory, auto-forget the weaker one; documented in v1 source but not yet ported) in `internal/service/auto_forget.go`
- [ ] T165 [P] Implement crystals + action-to-lesson track in `internal/service/crystallize.go`
- [ ] T166 [P] Implement procedural memory consumer in `internal/service/procedural.go`
- [ ] T167 [P] Implement working memory slot system (MCP tools: slot_create, slot_get, slot_list, slot_replace, slot_delete, slot_append) in `internal/service/working_memory.go`
- [ ] T168 [P] Implement signal/sentinel/checkpoint systems (MCP tools) in `internal/service/signals.go`
- [ ] T169 [P] Implement sketch create/promote workflow in `internal/service/sketch.go`
- [ ] T170 [P] Implement routine run instantiation in `internal/service/routine.go`
- [ ] T171 [P] Implement snapshot create with git versioning in `internal/service/snapshot.go`
- [ ] T172 [P] Implement file history lookups (commit context) in `internal/service/file_history.go`
- [ ] T173 [P] Implement patterns detection across sessions in `internal/service/patterns.go`
- [ ] T174 [P] Implement performance benchmark for hybrid search latency: p95 <500ms with 100k observations (SC-003) in `tests/integration/bench_search_test.go`
- [ ] T175 [P] Implement performance benchmark for MCP tool latency: p95 <200ms reads, <500ms writes (SC-008) in `tests/integration/bench_mcp_test.go`
- [ ] T176 [P] Implement performance benchmark for team visibility propagation: <60s (SC-006) in `tests/integration/bench_team_visibility_test.go`
- [ ] T177 [P] Implement performance benchmark for server startup time: <10s (SC-007) in `tests/integration/bench_startup_test.go`
- [ ] T178 [P] Implement performance benchmark for schema migration time: <30s (SC-009) in `tests/integration/bench_migration_test.go`
- [ ] T179 [P] Implement MCP tool-by-tool contract compatibility test against v0 reference behavior (SC-010) in `tests/integration/mcp_compat_test.go`
- [ ] T180 End-to-end integration test: full agent session with all 13 hooks + search + context injection in `tests/integration/e2e_test.go`
- [ ] T181 Security hardening: input sanitization, SQL injection prevention (sqlc already covers this), rate limiting stubs in `internal/handler/middleware_security.go`
- [ ] T182 Performance optimization: query plan review, index tuning, connection pool tuning in relevant service files
- [ ] T183 Documentation: update `README.md` with v2 setup and usage instructions
- [ ] T184 Run quickstart.md end-to-end validation

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion — BLOCKS all user stories
- **User Stories (Phase 3-7)**: All depend on Foundational phase completion
  - US1 (P1): Can start after Foundational. No story dependencies.
  - US2 (P2): Can start after Foundational. Depends on US1 for observations table + embeddings (search requires data).
  - US3 (P3): Can start after Foundational. Depends on US1 for sessions (auth ties to sessions).
  - US4 (P4): Can start after Foundational. Depends on US1+US2+US3 for tools wrapping services.
  - US5 (P5): Can start after Foundational. Depends on US1+US4 (serve requires handlers).
- **Polish (Phase 8)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational — No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational — Needs US1 observations table and embeddings
- **User Story 3 (P3)**: Can start after Foundational — Needs US1 session management
- **User Story 4 (P4)**: Needs US1 (observe, recall), US2 (search), US3 (auth) services to wrap
- **User Story 5 (P5)**: Needs US1+US4 handlers to wire into router

### Within Each User Story

- Tests MUST be written and FAIL before implementation (TDD per Principle III)
- Database migrations → sqlc queries → sqlc generate → service → handler
- Models before services, services before endpoints
- Core implementation before integration

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Within US1: T027-T031 (DDL migrations) can run in parallel; T019-T023 (unit tests) can run in parallel
- Within US4: T110-T132 (23 MCP tool registrations) can run in parallel — each is a separate wrapper
- US2 and US3 can start in parallel once US1 core is complete (different service files)
- All unit tests within a story marked [P] can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all DDL migrations together:
Task: "Create DDL migration: observations table in migrations/002_observations.up.sql"
Task: "Create DDL migration: observation_embeddings table in migrations/003_embeddings.up.sql"
Task: "Create DDL migration: compressed_observations in migrations/004_compressed.up.sql"
Task: "Create DDL migration: session_summaries in migrations/005_summaries.up.sql"
Task: "Create DDL migration: memories/lessons in migrations/006_memories.up.sql"

# Launch all unit tests together:
Task: "Unit test for observation type validation in tests/unit/observation_test.go"
Task: "Unit test for compression prompt assembly in tests/unit/compress_test.go"
Task: "Unit test for summarization chunking in tests/unit/summarize_test.go"
Task: "Unit test for consolidation input assembly in tests/unit/consolidate_test.go"
Task: "Unit test for reflection clustering in tests/unit/reflect_test.go"
```

## Parallel Example: User Story 4 (MCP Tools)

```bash
# All 23 MCP tool registrations can run in parallel (T110-T132)
# Each is a separate wrapper function in internal/mcp/tools.go
# Only dependency: the underlying service exists (from US1/US2/US3)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1 (Memory Pipeline Core)
4. **STOP and VALIDATE**: Run full pipeline — observe → compress → summarize → consolidate → reflect
5. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Pipeline works end-to-end → Deploy/Demo (MVP!)
3. Add User Story 2 → Search + Context injection → Deploy/Demo
4. Add User Story 3 → Multi-tenant with teams → Deploy/Demo
5. Add User Story 4 → Full MCP + REST API → Deploy/Demo
6. Add User Story 5 → Production deployment → Deploy/Demo
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 1 (Pipeline Core)
   - Developer B: Wait for US1 core tables → User Story 2 (Search) + User Story 3 (Auth) in parallel
   - Developer C: Wait for US1-3 services → User Story 4 (MCP Tools)
   - Developer D: Wait for US4 handlers → User Story 5 (Deployment)
3. Stories complete and integrate incrementally

---

## Notes

- [P] tasks = different files, no dependencies — can run in parallel
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- **TDD is MANDATORY** per constitution Principle III: write tests FIRST, verify they FAIL, then implement
- All SQL lives in `internal/db/queries/` — zero raw SQL in Go code per Principle IV
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution compliance: Superpowers Workflow (worktree → TDD → subagent → review → finish), Pipeline Integrity, Test-First, Type-Safe Data Access, Provider Agnosticism, Single Binary Simplicity
