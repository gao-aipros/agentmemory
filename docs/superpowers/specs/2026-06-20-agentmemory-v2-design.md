# AgentMemory v2 Design Specification

Date: 2026-06-20

## Overview

Platform migration: TypeScript/Node.js + SQLite/in-memory → Go + PostgreSQL (pgvector + pg_search via ParadeDB).
Preserve v0's multi-level pipeline (observe → compress → summarize → consolidate → reflect),
dual Observation/Action pipelines, and dual Context/Search consumer lines.
Add first-class User/Team entities with row-level ownership and three visibility modes.
Fix v0 bugs (Stop hook per-turn lifecycle, force-directed graph damping, etc.).

v0 source (living spec): https://github.com/Noodle05/agentmemory

---

## 1. Tech Stack

| Component | Choice |
|-----------|--------|
| Language | Go 1.26.4 |
| HTTP Router | chi (github.com/go-chi/chi/v5) |
| Logging | log/slog → stdout, Docker collects |
| DB Driver | pgx + pgxpool |
| SQL Codegen | sqlc, .sql files in internal/db/queries/ |
| DB Migration | golang-migrate (github.com/golang-migrate/migrate/v4) |
| Embedding + LLM | langchaingo (unified interfaces) |
| MCP | modelcontextprotocol/go-sdk (StreamableHTTP + JSONResponse) |
| Testing | testify + testcontainers-go (integration-test-heavy) |
| Config | env vars (consistent with v0 pattern) |

## 2. Project Layout

```
cmd/agentmemory/main.go      # entry point
internal/
  handler/                    # HTTP handlers (REST + MCP)
  service/                    # business logic
  store/                      # sqlc-generated DB access
  db/queries/                 # .sql query files
  mcp/                        # MCP tool registration (51 v1 + new team/auth tools)
  hooks/                      # plugin hook scripts → REST API
  auth/                       # JWT + API key authentication
  team/                       # team management
  config/                     # env-var parsing
migrations/                   # golang-migrate DDL files
```

## 3. Protocol Architecture

Two-channel design:

```
Plugin Hooks (shell scripts) → REST API (/v1/api/* + /v1/auth/*)
Agent MCP Tools               → MCP Streamable HTTP (/v1/mcp)
SPA Viewer                    → static files (/), WebSocket (/v1/socket)
```

| Path | Auth | Purpose |
|------|------|---------|
| `/` | st_ | SPA static files |
| `/health` | none | Docker health check |
| `/v1/mcp` | st_, ak_ | MCP Streamable HTTP |
| `/v1/api/*` | st_, ak_ | REST API |
| `/v1/auth/login` | none | Login (+ TOTP if enabled), returns JWT |
| `/v1/auth/keys` | st_ | Manage own API keys |
| `/v1/socket` | st_ | WebSocket (viewer live updates, v1 protocol) |

Token format: `Authorization: Bearer st_xxx` (JWT) / `Bearer ak_xxx` (API key hash prefix).
API keys cannot access UI routes.

## 4. Database

**Engine:** ParadeDB (`paradedb/paradedb:latest`, PG18), pg_search + pgvector enabled only.
Apache AGE deferred; graph traversal via PG native `WITH RECURSIVE` CTE.

**Core table:** Single `observations` table with triple index:
- BM25 (ParadeDB) on `(id, title, narrative, facts)`
- HNSW (pgvector) on `observation_embeddings(embedding vector_cosine_ops)`
- B-tree on `(timestamp, type, importance, session_id, concepts, files)`

**Schema:** 25 tables, 42 indexes. Full DDL in `docs/specs/10-schema-ddl.sql`.

**DB Access:** sqlc for all CRUD, zero raw SQL in Go code.

## 5. Smart Search

Single SQL FULL OUTER JOIN replacing v0's three-way independent search + JS RRF merge:

```
embed(query) → Float32Array
       ↓
Single SQL(query_text, query_vec):
  - pg_search BM25 match
  - pgvector HNSW cosine distance
  - WITH RECURSIVE graph traversal (independent of vector)
       ↓
FULL OUTER JOIN weighted merge + ORDER BY + LIMIT N
```

Weights: BM25 0.4, Vector 0.6, Graph 0.3 (same as v0).

Key changes from v0:
- HNSW index: O(n) brute → O(log n) ANN (cosine distance preserved)
- Graph independent of vector top-5
- Top-N only at final step, no per-stream over-fetch

## 6. Provider Architecture

Unified under langchaingo interfaces, config-driven via env vars:

**Embedding** (`embeddings.Embedder`):
- `EMBEDDING_PROVIDER` / `EMBEDDING_API_KEY` / `EMBEDDING_MODEL` / `EMBEDDING_BASE_URL`
- OpenAI (compat: Ollama, HF TEI) via `openai.New()` + `embeddings.NewEmbedder()`
- Voyage via `voyageai.NewVoyageAI()`

**LLM** (`llms.Model`):
- `LLM_PROVIDER` / `LLM_API_KEY` / `LLM_MODEL` / `LLM_BASE_URL`
- OpenAI (compat: DeepSeek, Ollama, vLLM) via `openai.New()`
- Anthropic (compat: native Messages API) via `anthropic.New()`

All callers depend only on interfaces, not specific providers.

## 7. Team/User Model

First-class User and Team entities in PostgreSQL with row-level ownership.

**Visibility levels:** private, team, public.

**Visibility by data type:**

| Data Type | Visibility |
|-----------|-----------|
| Observations | Always private |
| CompressedObs | Always private |
| SessionSummary | Always private |
| Crystals | Always private |
| Lessons | Always team (crystallize/manual/consolidation) |
| Memory | private/team/public (configurable) |

Privacy escape hatch: use `memory_save` for private/scoped data.

**Three modes:**

| Mode | Behavior |
|------|----------|
| Owned by User | Consolidation per-user, auto-share to team |
| Owned by Team | Single team consolidation |
| Member Choice | Per-observation share flag (`AGENTMEMORY_SHARE_CONSOLIDATED`) |

**Mechanics:**
- One-to-many user-team (many-to-many deferred)
- Exit = DELETE from team_members, no cleanup
- Re-join = full access to history
- PG replication for cross-instance (mesh out of scope)

## 8. Pipeline

Preserved from v0:

```
observe → compress → summarize → consolidate → reflect
```

| Stage | Output | Destination |
|-------|--------|-------------|
| observe | raw observation | session store |
| compress | CompressedObservation | BM25 + Vector dual index |
| summarize | SessionSummary | Context injection only |
| consolidate | SemanticMemory | reflect + retention |
| consolidate | ProceduralMemory | procedural store |
| consolidate | Insights | insights store |

## 9. Context Injection

1500 token hard limit, reference format (one-line summary + recall ID):

| Source | Budget |
|--------|--------|
| Pinned Slots | ~300t |
| Your Recent Sessions | ~250t |
| Team Lessons | ~200t |
| Team Shared Memory | ~250t |
| Project Profile | ~100t |

Enabled via `AGENTMEMORY_INJECT_CONTEXT` env var.
Only three hooks inject: SessionStart, PreToolUse (enrich), PreCompact.

## 10. Hooks

| Hook | v2 Behavior |
|------|------------|
| SessionStart | observe + conditional context injection |
| SessionEnd | session/end + consolidate + bridge sync |
| Stop | **Deleted** (v0 bug: per-turn lifecycle ops) |
| UserPromptSubmit | observe |
| PreToolUse | conditional enrich context injection |
| PostToolUse | observe (tool_name, input, output, image) |
| PostToolUseFailure | observe (tool_name, input, error), skip interrupt |
| PreCompact | conditional context injection |
| SubagentStart | observe, **no context injection** |
| SubagentStop | observe |
| Notification | observe |
| TaskCompleted | observe |
| PostCommit | session/commit (git sha → session link) |

Protocol: hooks → REST API, MCP tools → MCP Streamable HTTP.

## 11. MCP Tools

- All 51 v1 tools migrated to v2
- New tools for: team CRUD, team member management, auth key management

v1 tools registry: see v0 source `src/mcp/tools-registry.ts`

## 12. CLI

All v1 commands migrated + new:
- `serve` — start HTTP server
- `connect` — install into agent
- `viewer` — start SPA viewer
- `team` — team management (new)
- `migrate` — run DB schema migrations (new)
- `setup` — first-time initialization (new)

## 13. Testing

Integration-test-heavy approach:
- testcontainers-go with real ParadeDB PostgreSQL
- Focus on DB behavior correctness
- testify for assertions

## 14. Deployment

- Go app Docker image + `paradedb/paradedb:latest` (PG18) official image
- `docker-compose.yml`
- `DATABASE_URL` connection string

## 15. Data Migration

No v1 → v2 data migration. Breaking change, fresh start.

## 16. Deferred to Future

- Many-to-many user-team
- Apache AGE graph
- Pipeline inter-connections (observation → action auto-derivation)
- Crystals/auto timer fallback
- ProceduralMemory consumer

## Reference Files

Detailed specs in `docs/specs/`:
- 00-blueprint.md, 01-tech-stack.md, 02-project-layout.md
- 03-core-architecture.md, 04-smart-search.md, 05-provider.md
- 06-team-user.md, 07-pipeline.md, 08-hooks.md, 09-routing.md
- 10-schema-ddl.sql, 11-scope-decisions.md, 12-database-image.md
- 13-team-user-final.md
