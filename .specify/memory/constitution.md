<!--
  =============================================================================
  SYNC IMPACT REPORT
  =============================================================================
  Version change: [unset] → 1.0.0 (initial constitution) → 1.0.1 (PATCH: refined Principle III to mandate both unit and integration tests)
  Modified principles:
    - III. Integration-Test-First → III. Test-First (Unit + Integration) — expanded to require both test tiers
  Added sections:
    - Core Principles (6 principles)
    - Architecture Constraints
    - Development Workflow
    - Governance
  Removed sections: N/A
  Templates requiring updates:
    - .specify/templates/plan-template.md          ✅ compatible (Constitution Check gate defers to this file)
    - .specify/templates/spec-template.md           ✅ compatible
    - .specify/templates/tasks-template.md          ✅ compatible (constitution mandates both test tiers)
    - .specify/templates/checklist-template.md      ✅ compatible
  Follow-up TODOs: none
  =============================================================================
-->

# AgentMemory Constitution

## Core Principles

### I. Superpowers Workflow (NON-NEGOTIABLE)

Implementation of any task list MUST follow the Superpowers workflow:

1. **Worktree** — Isolate feature work in a git worktree via `/using-git-worktrees`
2. **TDD (Red-Green-Refactor)** — Write tests first, verify they fail, then implement, then refactor
3. **Subagent-Driven Execution** — Execute independent tasks in parallel via subagents
4. **Code Review** — Request review via `/requesting-code-review` before merging
5. **Finish Branch** — Complete integration via `/finishing-a-development-branch`

No implementation task may begin without an active worktree. No task is complete
until it passes code review and follows the finish-branch integration step. This
workflow is mandatory for all feature work, bug fixes, and refactoring efforts.

### II. Pipeline Integrity

The multi-level extract/aggregate/consolidation pipeline (observe → compress →
summarize → consolidate → reflect) is the core architectural invariant. Every
change MUST preserve or enhance this pipeline; nothing may bypass it.

- Dual Observation/Action pipelines and dual Context/Search consumption lines
  carry forward from v0 unchanged in structure
- SessionSummary feeds context injection ONLY, never search index
- CompressedObservation populates BM25 + Vector dual index
- Consolidation produces SemanticMemory, ProceduralMemory, and Insights in
  separate stores

**Rationale**: The pipeline is the product. All higher-level capabilities
(recall, search, reflection, lessons) depend on data flowing through every
stage in order. Shortcuts corrupt downstream consumers.

### III. Test-First (Unit + Integration)

TDD red-green-refactor cycle is mandatory: write tests → verify failure →
implement → verify success. Both unit tests and integration tests are required;
neither tier is optional.

- **Unit tests**: Fast, isolated tests for pure logic (parsing, validation,
  algorithms, configuration). MUST cover all business logic paths. Use testify
  for assertions.
- **Integration tests**: Tests against real ParadeDB PostgreSQL via
  testcontainers-go. MUST exercise actual SQL, BM25 full-text search, HNSW
  vector operations, and WITH RECURSIVE CTE graph traversal. Use testify for
  assertions and test suite organization.
- Unit tests run first (fast feedback); integration tests gate merge (correctness
  verification). Both MUST pass before any task is considered complete.
- Mock-based testing is acceptable for external HTTP services only, not for
  database-dependent behavior.

**Rationale**: Unit tests catch logic errors cheaply and quickly. Integration
tests validate that the full stack — Go code, SQL queries, ParadeDB extensions —
works correctly together. ParadeDB-specific behavior (bm25 scoring, vector
distance operators, CTE traversal) cannot be faithfully reproduced in mocks.

### IV. Type-Safe Data Access

Zero raw SQL in Go application code. ALL database queries MUST go through sqlc.

- Query files live in `internal/db/queries/` (`.sql` files)
- Compiled to type-safe Go functions in `internal/store/` (never hand-edit)
- ParadeDB-specific syntax (bm25 operator, `<->` vector distance, `WITH
  RECURSIVE`) passes through sqlc transparently
- Direct `pgx`/`pgxpool` queries in handler or service layers are forbidden

**Rationale**: Compile-time SQL verification catches query errors at build
time, not runtime. Type-safe generated code eliminates SQL injection vectors
and provides IDE autocompletion for query results.

### V. Provider Agnosticism

LLM and embedding provider selection MUST be decoupled from business logic via
langchaingo interfaces.

- `embeddings.Embedder` interface for embedding providers
- `llms.Model` interface for LLM providers
- Adding a new provider MUST NOT require code changes — only env var
  configuration
- Supported providers include OpenAI, Anthropic, Voyage, Ollama, and any
  other langchaingo-compatible provider

**Rationale**: Operators must be able to swap providers for cost, latency,
or compliance reasons without touching application code. Provider choice is
an operational concern, not a development one.

### VI. Single Binary Simplicity

The system MUST deploy as a single Go binary in a Docker container. Start
simple; add complexity only when proven necessary (YAGNI).

- One HTTP port serves all traffic: REST API, MCP Streamable HTTP, WebSocket,
  static files
- Standard library logging (`log/slog` → stdout); Docker manages log
  collection and rotation
- Environment variables for all configuration (consistent with v0 pattern)
- No microservices, no message queues, no external caches unless an explicit
  requirement justifies them

**Rationale**: A single binary eliminates deployment coordination, simplifies
health checks, and reduces operational surface area. Every additional moving
part must earn its place.

## Architecture Constraints

The following technical decisions are locked by the v2 design specification
(`docs/specs/00-blueprint.md` through `docs/specs/13-team-user-final.md`):

| Component | Constraint |
|-----------|-----------|
| Language | Go 1.26.4 |
| HTTP Router | chi (go-chi/chi/v5) |
| DB Driver | pgx + pgxpool (connection pooling, health checks) |
| SQL Codegen | sqlc (zero raw SQL in application code) |
| DB Migration | golang-migrate (DDL in `migrations/`) |
| LLM/Embedding | langchaingo (provider-agnostic interfaces) |
| MCP | modelcontextprotocol/go-sdk (StreamableHTTP, JSONResponse) |
| Testing | testify + testcontainers-go (real ParadeDB PostgreSQL) |
| Config | Environment variables |

**Database**: ParadeDB `paradedb/paradedb:0.24.1-pg18` with pg_search + pgvector
extensions enabled. Apache AGE deferred; graph traversal uses PostgreSQL native
`WITH RECURSIVE` CTE.

**Schema**: 25 tables, 42 indexes per `10-schema-ddl.sql`. Single `observations`
table with BM25 + HNSW + B-tree triple index. Observations are always private
(`visibility = 'private'` CHECK constraint).

**Context injection**: 1500 token hard limit with documented budget allocations
(~1100 tokens for sources, ~400 for overhead including recall IDs).

**Protocol**: Single-port HTTP. Token format: `Bearer st_` (JWT session tokens)
and `Bearer ak_` (API key hash prefixes). JWT expiry default 24h via `JWT_EXPIRY`
env var.

**v0 reference**: The v0 source at https://github.com/Noodle05/agentmemory is
the living behavioral spec. All v2 feature work MUST reference v0 behavior.
Breaking changes from v0 require explicit justification and documentation.

## Development Workflow

### Task Implementation

Every task from `tasks.md` MUST follow the Superpowers workflow defined in
Principle I. The full sequence:

```
worktree → write tests (RED) → implement (GREEN) → refactor → code review → finish-branch
```

- **Worktree**: `EnterWorktree` provides isolation; each feature gets its own
  branch and working directory
- **RED**: Write integration tests against real ParadeDB; verify they fail
- **GREEN**: Implement the minimum code to make tests pass
- **Refactor**: Clean up within the safety of passing tests
- **Code Review**: Use `/requesting-code-review` for adversarial verification
- **Finish**: Use `/finishing-a-development-branch` for merge, PR, or cleanup

### Quality Gates

- All integration tests MUST pass against real ParadeDB PostgreSQL
- Health check MUST return 503 on DB failure or pending migrations
- Error handling: exponential backoff retry, no circuit breaker
- Code review must verify compliance with all six Core Principles

### Testing Discipline

- **Both tiers required**: Unit tests for fast feedback on business logic;
  integration tests for database correctness verification. Both MUST pass.
- Unit tests MUST cover all business logic paths (parsing, validation,
  algorithms, configuration)
- Every pipeline stage (observe, compress, summarize, consolidate, reflect)
  MUST have integration tests verifying its data flow
- BM25, HNSW vector, and CTE graph queries MUST have dedicated integration
  test coverage
- TDD is mandatory: no implementation before failing tests
- Unit tests run first for rapid iteration; integration tests gate merge

## Governance

### Amendment Procedure

1. Propose amendment via PR to this file (`.specify/memory/constitution.md`)
2. Amendment must include: rationale, impact assessment on dependent templates,
   and migration plan if breaking
3. All six Core Principles require justification to modify; Principle I
   (Superpowers Workflow) requires explicit approval to alter
4. Version number MUST be updated per semantic versioning rules (see below)

### Versioning Policy

- **MAJOR**: Backward incompatible governance/principle removals or redefinitions
- **MINOR**: New principle/section added or materially expanded guidance
- **PATCH**: Clarifications, wording fixes, typo corrections, non-semantic
  refinements

### Compliance Review

- Every PR MUST verify compliance with all Core Principles
- Plan documents (`plan.md`) MUST include a Constitution Check section
  validating alignment before Phase 0 research and after Phase 1 design
- Complexity that violates Principle VI (Single Binary Simplicity) MUST be
  justified in the plan's Complexity Tracking table
- The Governance section supersedes all other project practices and conventions

**Version**: 1.0.1 | **Ratified**: 2026-06-21 | **Last Amended**: 2026-06-21
