# Feature Specification: AgentMemory v2 Platform Migration

**Feature Branch**: `001-agentmemory-v2-platform`

**Created**: 2026-06-21

**Status**: Draft

**Input**: User description: "Get feature spec requirement designs from docs/specs/00-blueprint.md and related document to create specs"

## Clarifications

### Session 2026-06-21

- Q: When a team owner deletes their account, what happens to other members' observations linked to that team via `owner_team_id`? → A: Orphan them — set `owner_type='user'`, `owner_team_id=NULL`; data preserved, visibility reverts to private user ownership.
- Q: What are the retry limits and terminal behavior when the embedding API is unreachable during compression? → A: Max 5 retries with exponential backoff (5-minute cap). On exhaustion: mark observation with `compression_error` flag, skip, continue with next observation, log CRITICAL.
- Q: Can users opt out of sharing in Owned-by-User or Owned-by-Team modes, or override team-wide consolidation? → A: Fixed rules per mode. Owned-by-User auto-shares ALL consolidated Memory to team (no opt-out). Owned-by-Team consolidates all members into single pool (no opt-out). Only Member Choice mode supports per-observation control via `AGENTMEMORY_SHARE_CONSOLIDATED`.
- Q: Is SC-005 (5-minute first session) an automatable CI metric or a manual UX target? → A: Split into SC-005a (account creation + API key generation <30s, automated CI gate) and SC-005b (time-to-first-session <5min, UX target, manual validation only).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Memory Pipeline Core (Priority: P1)

An AI agent runs through a session: the user asks questions, the agent uses tools,
commits code, and completes tasks. Throughout this session, agentmemory silently
captures every interaction — prompts, tool calls, tool outputs, task completions —
and compresses them into searchable memory. When the session ends, agentmemory
summarizes the entire session, consolidates learnings, and reflects on patterns
across sessions.

**Why this priority**: The memory pipeline is the core value proposition. Without
observation capture, compression, and consolidation, no other feature (search,
recall, context injection) can function. Every downstream capability depends on
data flowing through this pipeline.

**Independent Test**: Run a simulated agent session through all 13 hook events
(SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure,
PreCompact, SubagentStart, SubagentStop, Notification, TaskCompleted, PostCommit,
SessionEnd). Verify that observations are recorded, compressed within 30 seconds,
and session summaries appear after SessionEnd fires.

**Acceptance Scenarios**:

1. **Given** an agent session starts (SessionStart hook fires), **When** the hook
   processes, **Then** a session record is created and the observe call stores the
   session start event as a raw observation.
2. **Given** a user submits a prompt (UserPromptSubmit hook fires), **When** the hook
   processes, **Then** the prompt text and metadata are stored as an observation with
   type `user_prompt`.
3. **Given** a tool executes successfully (PostToolUse hook fires), **When** the hook
   processes, **Then** the tool name, input parameters, and output are stored as an
   observation with type `tool_use`.
4. **Given** a tool execution fails (PostToolUseFailure hook fires), **When** the hook
   processes, **Then** the failure is recorded as an observation without interrupting
   the agent's workflow.
5. **Given** raw observations exist, **When** the compression async goroutine runs,
   **Then** raw observations are compressed into searchable CompressedObservations
   with BM25 and vector embeddings indexed.
6. **Given** a session ends (SessionEnd hook fires), **When** the hook processes,
   **Then** the session is closed, summarization runs to produce a SessionSummary,
   consolidation extracts SemanticMemory and Lessons, and the reflection timer is
   started.

---

### User Story 2 - Smart Search & Context Injection (Priority: P2)

An agent in a new session needs relevant context from past sessions. Agentmemory
performs a hybrid search combining full-text (BM25), semantic (vector), and
relationship (graph) dimensions, then injects the top results into the agent's
context window. The injection respects a strict 1500-token budget.

**Why this priority**: Search and context injection are the retrieval half of the
memory system. Without them, stored memories are write-only — agents cannot benefit
from past work. This is the primary consumption path for all captured observations.

**Independent Test**: Populate the database with known observations across multiple
sessions. Execute a `memory_recall` query and verify results combine BM25, vector,
and graph matches in a single ranked list. Execute `memory_smart_search` and verify
progressive disclosure (compact → expandable). Trigger SessionStart with
`AGENTMEMORY_INJECT_CONTEXT=true` and verify the agent receives a context summary
within 1500 tokens.

**Acceptance Scenarios**:

1. **Given** the database contains observations about "PostgreSQL connection pooling,"
   **When** an agent queries "database pool configuration," **Then** the search returns
   the PostgreSQL-related observations ranked by combined BM25 (0.4), vector (0.6),
   and graph (0.3) scores.
2. **Given** context injection is enabled, **When** a session starts, **Then** the
   agent receives a formatted context block containing relevant observations, a session
   recap if recent sessions exist, and any relevant lessons — all within a 1500-token
   budget.
3. **Given** an agent is about to use a tool (PreToolUse hook fires), **When** context
   enrichment is enabled, **Then** a file-specific search runs for context relevant to
   the file paths mentioned in the tool input.
4. **Given** a `memory_recall` query, **When** no exact keyword match exists, **Then**
   semantic vector search still surfaces conceptually related observations.
5. **Given** the database has related observations connected via graph edges, **When** a
   search matches one observation, **Then** the graph traversal surfaces connected
   observations that the user didn't explicitly search for.

---

### User Story 3 - Team & User Management (Priority: P3)

An organization deploys agentmemory for multiple developers. Each developer has
their own user account with API keys. Developers can form teams where consolidated
memory is shared while raw observations remain private. A team owner manages
membership, visibility defaults, and operational modes.

**Why this priority**: Multi-tenant isolation is essential for real-world deployment.
Without it, all memory is either globally visible (no privacy) or completely siloed
(no collaboration). Team/user management bridges this gap with fine-grained ownership.

**Independent Test**: Create two users (Alice and Bob). Alice creates a team and
invites Bob. Alice runs an agent session; verify her observations are private.
Both users run sessions; verify consolidated memories are visible to both per team
visibility rules. Bob leaves the team; verify his private data remains his but
team-shared memories persist.

**Acceptance Scenarios**:

1. **Given** no users exist, **When** an admin creates a user account with email and
   password, **Then** the user can log in and receive a session token (JWT).
2. **Given** a user has TOTP enabled, **When** they log in with correct email and
   password, **Then** they must also provide a valid TOTP code to receive a session token.
3. **Given** a logged-in user, **When** they create an API key with a label, **Then**
   the key is generated with `ak_` prefix and can be used for API/MCP authentication.
4. **Given** a user (team owner), **When** they create a team, **Then** the team is
   created with the specified name and default visibility mode.
5. **Given** a team exists, **When** the owner adds a member, **Then** the member's
   future observations are governed by the team's visibility and sharing rules.
6. **Given** a team member, **When** they leave or are removed from the team, **Then**
   their private observations remain theirs, team-shared memories persist, and their
   team membership record is deleted (exit/re-join pattern).
7. **Given** a team in "Member Choice" mode, **When** `AGENTMEMORY_SHARE_CONSOLIDATED=true`
   is set on a member's observe call, **Then** only that observation's consolidated output
   is shared with the team.

---

### User Story 4 - MCP Tools & REST API (Priority: P4)

Host coding agents (Claude Code, Codex, etc.) communicate with agentmemory through
two channels: MCP tools for direct agent-to-memory operations, and REST API for
plugin hook scripts. All 51 v1 MCP tools are available, plus new tools for team and
auth management.

**Why this priority**: The MCP tools and REST API are the integration surface. The
pipeline (P1) and search (P2) are the engine; this story provides the steering wheel
and dashboard. Without it, the memory system has no external interface.

**Independent Test**: Start the agentmemory server. Connect an MCP client to
`/v1/mcp` and call `memory_observe` to store a test observation. Call
`memory_recall` to retrieve it. Verify the REST API accepts hook-equivalent calls
on `/v1/api/*`. Verify authentication: valid `st_` token grants access, invalid
token returns 401.

**Acceptance Scenarios**:

1. **Given** the server is running, **When** an MCP client connects to `/v1/mcp`
   with a valid `st_` token, **Then** all registered MCP tools are listed and
   callable.
2. **Given** a hook script fires PostToolUse, **When** it POSTs to
   `/v1/api/observe` with the tool execution data, **Then** the observation is
   stored and the response returns within 200ms.
3. **Given** an agent calls `memory_recall` via MCP, **When** the query is
   processed, **Then** results return in the same format as v1 (compact vs full
   modes, progressive disclosure).
4. **Given** a team owner, **When** they call `team_create` via MCP, **Then** a
   new team is created and the response includes team ID, name, and visibility mode.
5. **Given** an API key with `ak_` prefix, **When** used for MCP or REST API calls,
   **Then** the key authenticates successfully for API-scoped routes but is rejected
   for UI routes (`/`, `/v1/socket`).

---

### User Story 5 - Deployment, CLI & Operations (Priority: P5)

An operator deploys agentmemory as a single Docker container. They run `agentmemory
setup` to initialize the database schema, `agentmemory serve` to start the server,
and `agentmemory migrate` to apply schema changes. A health check endpoint monitors
the service. The `agentmemory connect` CLI command configures a host coding agent
to use the memory server.

**Why this priority**: Operations concerns (deployment, health, migration) gate
production use but are independent of core memory functionality. The system can
be fully functional with manual setup; this story makes it operational.

**Independent Test**: Build the Go binary. Run `agentmemory setup` against a
ParadeDB PostgreSQL container. Verify all 25 tables and 42 indexes are created.
Run `agentmemory serve` and verify `/health` returns 200. Run `agentmemory
connect` in a Claude Code workspace and verify the settings are written correctly.

**Acceptance Scenarios**:

1. **Given** a ParadeDB PostgreSQL container is running, **When** the operator runs
   `agentmemory setup`, **Then** all 25 tables, 42 indexes, and extensions
   (pg_search, pgvector) are created successfully.
2. **Given** the server is running, **When** Docker probes `/health`, **Then** the
   endpoint returns 200 if the database connection is alive, 503 if the database is
   unreachable or migrations are pending.
3. **Given** a new schema migration exists in `migrations/`, **When** the operator
   runs `agentmemory migrate`, **Then** pending migrations are applied in order and
   the server continues to function.
4. **Given** a Claude Code workspace, **When** `agentmemory connect` runs, **Then**
   the MCP server configuration is written to the correct settings file with the
   server URL and authentication token.
5. **Given** the server is running, **When** the operator accesses `/` in a browser
   with a valid `st_` token, **Then** the SPA viewer loads and displays session
   data via WebSocket at `/v1/socket`.
6. **Given** an invalid JWT or expired API key, **When** the client makes a request
   to a protected route, **Then** the server returns 401 with a descriptive error
   message.

---

### Edge Cases

- What happens when the embedding API is unreachable during compression? The system
  MUST retry with exponential backoff (max 5 retries, 5-minute backoff cap). On
  exhaustion: mark the observation with a `compression_error` flag, skip it, continue
  with the next observation, and log CRITICAL. Flagged observations are retried on the
  next compression cycle if the API has recovered.
- What happens when a session exceeds the token budget during summarization? The
  summarizer MUST chunk the session observations and produce a multi-part summary,
  then merge into a single SessionSummary within the context injection budget.
- What happens when a team owner deletes their account? The team and all team-shared
  memories are deleted (CASCADE from users → teams → team_shared_memories). Other
  members' observations that had `owner_team_id` pointing to the deleted team are
  orphaned: `owner_type` reverts to `user`, `owner_team_id` set to NULL. Other
  members' private data is preserved. Team memberships (team_members rows) are
  CASCADE-deleted with the team.
- What happens when the database is at capacity? The eviction system MUST prioritize
  low-importance, old observations for removal. Compressed observations and lessons
  are preserved over raw observations.
- What happens when two sessions end simultaneously? Each session's consolidation and
  summarization MUST run independently without data corruption. PostgreSQL row-level
  locking ensures isolation.
- What happens when an API key expires mid-session? The server returns 401 on the next
  request. The agent MUST re-authenticate with a valid token.
- What happens when context injection exceeds the 1500-token budget? The context
  assembly MUST truncate lower-priority sources (graph, lessons) before touching
  higher-priority sources (session recap, relevant observations).

## Requirements *(mandatory)*

### Functional Requirements

#### Memory Pipeline

- **FR-001**: System MUST capture agent session events via 13 hook types: SessionStart,
  UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure, PreCompact,
  SubagentStart, SubagentStop, Notification, TaskCompleted, PostCommit, SessionEnd,
  and PermissionPrompt (Notification filter).
- **FR-002**: System MUST store all observations in a single `observations` table with
  BM25 full-text index, HNSW vector index, and B-tree indexes on metadata columns.
- **FR-003**: System MUST compress raw observations into CompressedObservations via
  async goroutine (triggered on each observe call, non-blocking).
- **FR-004**: System MUST summarize sessions into SessionSummary when SessionEnd fires
  (async, runs after session closure).
- **FR-005**: System MUST consolidate SessionSummaries into SemanticMemory, Lessons,
  and Insights (async, via consolidation timer).
- **FR-006**: System MUST run reflection periodically to detect patterns and clusters
  across sessions (async, via reflect timer).
- **FR-007**: Observations MUST always have `visibility = 'private'` — no exception.
  Sharing only happens at the Memory layer after consolidation.
- **FR-008**: System MUST support configurable visibility for Memory entities:
  `private`, `team`, or `public`.

#### Search & Context Injection

- **FR-009**: System MUST execute hybrid search as a single SQL query combining BM25
  (weight 0.4), vector cosine similarity (weight 0.6), and graph traversal (weight 0.3)
  via FULL OUTER JOIN.
- **FR-010**: System MUST support `memory_recall` with progressive disclosure: compact
  results first, expandable to full observation details.
- **FR-011**: System MUST support `memory_smart_search` combining hybrid search with
  optional result expansion by observation ID.
- **FR-012**: System MUST inject context into agent sessions within a 1500-token hard
  limit, with documented budget allocations: ~1100 tokens for source content, ~400
  tokens for formatting and recall IDs.
- **FR-013**: Context injection MUST draw from five source buckets: relevant
  observations, session recap (if recent sessions exist), relevant lessons, graph
  neighbors, and working memory.
- **FR-014**: Only three hooks MUST inject context: SessionStart (initial load),
  PreToolUse (file-specific enrichment), and PreCompact (refresh before compaction).

#### Team & User Management

- **FR-015**: System MUST support first-class User entities with email, password hash,
  optional TOTP secret, and session token (JWT) authentication.
- **FR-016**: System MUST support API key generation with `ak_` prefix, configurable
  expiration, and last-used tracking.
- **FR-017**: System MUST support first-class Team entities with owner, name, default
  visibility mode, and member list.
- **FR-018**: System MUST support three team operational modes with fixed behavioral rules:
  - **Owned by User**: Each user runs their own consolidation pipeline. ALL consolidated
    Memory is auto-shared to the team (no per-observation opt-out). Users maintain
    individual context while contributing to team knowledge.
  - **Owned by Team**: A single team-level consolidation run covers all members. All
    observations from all members feed one shared Memory pool (no per-member opt-out).
  - **Member Choice**: Per-user consolidation. The `AGENTMEMORY_SHARE_CONSOLIDATED`
    env var is read on each `observe()` call; when true, that observation's consolidated
    output is shared with the team. This is the default mode for new teams.
- **FR-019**: Every data row MUST carry `owner_type`, `owner_user_id`/`owner_team_id`,
  and `visibility` metadata for row-level access control.
- **FR-020**: When a member leaves a team, their team_members row MUST be deleted
  (exit/re-join pattern). Their private data remains theirs. Team-shared memories
  persist.

#### MCP Tools & REST API

- **FR-021**: System MUST serve MCP Streamable HTTP at `/v1/mcp` with all 44 public
  v1 MCP tools plus new team and auth management tools (the original v1 had 51 tools
  total; 7 are internal/utility tools not exposed as public MCP tools).
- **FR-022**: System MUST serve REST API at `/v1/api/*` for hook scripts and
  external clients.
- **FR-023**: System MUST authenticate requests via `Authorization: Bearer st_<jwt>`
  (session tokens) or `Bearer ak_<prefix>` (API keys).
- **FR-024**: API keys MUST be rejected for UI routes (`/`, `/v1/socket`).
- **FR-025**: JWT session tokens MUST have configurable expiry (default 24h via
  `JWT_EXPIRY` env var).

#### Deployment & Operations

- **FR-026**: System MUST deploy as a single Go binary in a Docker container serving
  all traffic on one HTTP port.
- **FR-027**: System MUST provide a `/health` endpoint returning 200 on healthy
  database connection, 503 on database failure or pending migrations.
- **FR-028**: System MUST apply database migrations via golang-migrate at startup
  or via `agentmemory migrate` CLI command.
- **FR-029**: System MUST log structured output to stdout via `log/slog`. Docker
  manages log collection and rotation.
- **FR-030**: All configuration MUST use environment variables, consistent with
  the v0 pattern.

### Key Entities *(include if feature involves data)*

- **Observation**: Raw captured event from an agent session (tool call, prompt,
  response, notification). Always private. Indexed by BM25 + HNSW vector + B-tree.
  Links to session and user.
- **CompressedObservation**: LLM-compressed summary of one or more raw observations.
  Always private. Dual-indexed in BM25 and vector for search.
- **SessionSummary**: Per-session summary produced when SessionEnd fires. Always
  private. Used for context injection only, never search-indexed.
- **SemanticMemory**: Fact extracted during consolidation from session summaries.
  Configurable visibility (private/team/public). Searchable.
- **Lesson**: Learned pattern or rule with confidence score. Always team visibility.
  Strengthens on reinforcement, decays when unused.
- **Crystal**: Compressed action chain digest (completed task narratives). Always
  private. Used for action-to-lesson track.
- **User**: Authenticated account with email, password hash, optional TOTP, JWT
  session tokens, and API keys.
- **Team**: Named group with an owner, members, default visibility mode, and
  operational mode.
- **Session**: Bounded agent work period identified by session ID. Links observations,
  commits, and summaries to a specific work context.
- **Graph Node / Edge**: Knowledge graph representation connecting related observations,
  memories, and concepts. Traversed via `WITH RECURSIVE` CTE for graph search.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent session from start to end captures all 13 hook event types
  without data loss (100% hook event capture rate).
- **SC-002**: Raw observations are compressed and searchable within 30 seconds of
  the observe call completing.
- **SC-003**: Hybrid search (BM25 + vector + graph) returns results in a single
  ranked list within 500ms for a database of 100,000 observations.
- **SC-004**: Context injection assembles and delivers a context summary within the
  1500-token budget without truncating high-priority sources.
- **SC-005a**: Account creation + API key generation completes in under 30 seconds
  (automated CI gate — programmatic API calls, no human interaction required).
- **SC-005b**: A new user can create an account, generate an API key, and run their
  first agent session with memory capture in under 5 minutes (UX target, manual
  validation only — NOT a CI gate).
- **SC-006**: Team-shared memories are visible to all team members within 60 seconds
  of consolidation completing.
- **SC-007**: The server starts and passes health check within 10 seconds of Docker
  container launch with a pre-migrated database.
- **SC-008**: 95% of MCP tool calls complete within 200ms (read operations) or 500ms
  (write operations with embedding generation).
- **SC-009**: Schema migration from zero to full DDL (25 tables, 42 indexes) completes
  in under 30 seconds.
- **SC-010**: All 44 public v1 MCP tools are available and behaviorally compatible
  with the v0 reference implementation. Compatibility defined as: same parameter
  signatures, same return schemas, same error codes. Acceptable deviations: scoring
  algorithm changes (v0 JS in-memory → v2 SQL), performance improvements. Verified
  by automated tool-by-tool contract test.

## Assumptions

- The ParadeDB Docker image `paradedb/paradedb:0.24.1-pg18` remains available and
  stable throughout v2 development.
- Host coding agents (Claude Code, Codex) support MCP Streamable HTTP as their
  integration protocol with agentmemory.
- The v0 source at https://github.com/Noodle05/agentmemory is the authoritative
  behavioral reference for all v1 tool behavior.
- No data migration from v1 (TypeScript/Node.js + SQLite) to v2 (Go + PostgreSQL)
  is required. This is a breaking change with a fresh start.
- Embedding and LLM providers are accessible via langchaingo interfaces and
  configured through environment variables.
- The system operates within a single Docker host; cross-instance visibility
  (via PostgreSQL logical replication) is out of scope for v2.
- Many-to-many user-team relationships, Apache AGE graph engine, and pipeline
  inter-connections (observation → action auto-derivation) are deferred to
  future versions.
- The SPA viewer (static files at `/`) follows v1 behavior and protocol.
- All hook latency is bounded by a single database insert for recording hooks;
  only SessionEnd triggers the full consolidation pipeline.
- The Notification hook filter (only permission allow prompts) is sufficient for
  capturing user decision context without excessive storage.
