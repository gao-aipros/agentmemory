# Research: AgentMemory v2 Platform Migration

**Feature**: AgentMemory v2 Platform Migration
**Date**: 2026-06-21
**Source**: `docs/specs/00-blueprint.md` through `docs/specs/13-team-user-final.md`

## Decisions

All technology decisions were finalized during the v2 design phase (2026-06-20 to
2026-06-21) and are documented in the `docs/specs/` directory. This research file
consolidates those decisions for the implementation plan.

### Decision 1: Go over TypeScript/Node.js

**Decision**: Go 1.26.4
**Rationale**: Single binary deployment, compile-time type safety, superior
concurrency model (goroutines) for async pipeline stages, no runtime dependency
on Node.js.
**Alternatives considered**:
- Keep TypeScript/Node.js — rejected: SQLite in-memory stores hit scaling limits;
  single-binary deployment impossible
- Rust — rejected: longer development cycle, smaller ecosystem for MCP/LLM libraries

### Decision 2: ParadeDB over Separate PostgreSQL + Extensions

**Decision**: ParadiseDB `paradedb/paradedb:0.24.1-pg18` with pg_search + pgvector
**Rationale**: Single Docker image bundles both extensions; no manual compilation;
pg_search provides production-grade BM25 without external search engine.
**Alternatives considered**:
- Vanilla PostgreSQL + pgvector only — rejected: no BM25, must add Elasticsearch
- Elasticsearch for full-text — rejected: two databases to manage, sync, and deploy

### Decision 3: sqlc over ORM or Raw Queries

**Decision**: sqlc — type-safe codegen from `.sql` files
**Rationale**: Compile-time SQL verification; generated Go is fast (no reflection);
ParadeDB-specific syntax passes through transparently.
**Alternatives considered**:
- GORM — rejected: ORM overhead, difficult to express BM25/HNSW/CTE queries
- Raw pgx queries — rejected: no compile-time safety, SQL injection risk

### Decision 4: langchaingo for LLM/Embedding Abstraction

**Decision**: langchaingo (unified `embeddings.Embedder` + `llms.Model` interfaces)
**Rationale**: Provider-agnostic; swap OpenAI/Anthropic/Voyage/Ollama via env vars;
no code changes required to add providers.
**Alternatives considered**:
- Direct OpenAI SDK — rejected: vendor lock-in
- Custom abstraction layer — rejected: reinventing the wheel

### Decision 5: Single observations Table with Triple Index

**Decision**: One `observations` table with BM25 (pg_search), HNSW (pgvector),
and B-tree indexes
**Rationale**: Hybrid search in one SQL `FULL OUTER JOIN` replaces v0's three
independent in-memory JS Map searches + RRF merge. Simpler, faster, more correct.
**Alternatives considered**:
- Separate tables per search dimension — rejected: complex joins, data duplication
- Keep v0 in-memory approach — rejected: doesn't scale, not persistent

### Decision 6: PostgreSQL WITH RECURSIVE CTE over Apache AGE

**Decision**: Native `WITH RECURSIVE` CTE for graph traversal
**Rationale**: ParadeDB image doesn't include Apache AGE; CTE is sufficient for
the graph depth needed; AGE deferred to future.
**Alternatives considered**:
- Apache AGE — rejected: not available in ParadeDB image, adds complexity
- Custom graph database — rejected: violates single binary principle

### Decision 7: Three Team Operational Modes

**Decision**: Owned by User, Owned by Team, Member Choice
**Rationale**: Covers solo use, team-wide shared memory, and per-observation control.
Default to Member Choice for maximum flexibility.
**Alternatives considered**:
- Single mode (team-only) — rejected: no solo use case support
- Per-observation flag only — rejected: too much client burden for simple cases

### Decision 8: Context Injection Token Budget

**Decision**: 1500 token hard limit with 5 source buckets and priority truncation
**Rationale**: Keeps context injection from consuming too much of the agent's
context window. Priority-based truncation protects high-value sources.
**Alternatives considered**:
- Dynamic budget based on model — rejected: unpredictable behavior
- No budget — rejected: can consume entire context window

### Decision 9: Exit/Re-join Pattern for Team Membership

**Decision**: DELETE from team_members on leave; full history preserved on re-join
**Rationale**: Simple, no soft-delete complexity. Private data stays with user.
Team-shared memories persist.
**Alternatives considered**:
- Soft-delete with membership status — rejected: extra state column, query filtering
- Cascade delete all data on leave — rejected: data loss for departing member

### Decision 10: No v1 → v2 Data Migration

**Decision**: Breaking change, fresh start. No migration path from v1.
**Rationale**: v1 uses SQLite + in-memory stores; v2 uses PostgreSQL with
different schema. Migration would be complex and error-prone.
**Alternatives considered**:
- Export/import tool — rejected: v1 data model too different, not worth effort

## Technology Best Practices

| Area | Practice | Source |
|------|----------|--------|
| Go project layout | Standard `cmd/` + `internal/` | Go community conventions |
| SQL codegen | All queries in `.sql` files, never raw in Go | sqlc best practices |
| Testing | testcontainers for integration; testify for assertions | Go testing patterns |
| Config | env vars (12-factor app) | Consistent with v0 |
| Logging | slog → stdout; Docker collects | 12-factor app |
| Error handling | Exponential backoff retry; no circuit breaker | v2 design decision |
| Health check | 200 on healthy DB; 503 on failure/pending migrations | Docker healthcheck pattern |

## Unresolved

None. All technical decisions are locked by the v2 design specification.
