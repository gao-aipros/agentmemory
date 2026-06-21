# AgentMemory v2

Long-term memory for coding agents. AgentMemory captures observations from agent sessions, consolidates them through a 4-tier memory pipeline, and surfaces relevant context when agents need it.

## Quickstart

```bash
# 1. Start ParadeDB PostgreSQL (with pgvector + BM25)
docker compose up -d db

# 2. Run database setup (migrations + admin user)
go run cmd/agentmemory/main.go setup \
  --db-url "postgres://postgres:agentmemory@localhost:5432/agentmemory?sslmode=disable"

# 3. Start the server (single binary, single port 8080)
go run cmd/agentmemory/main.go serve \
  --db-url "postgres://postgres:agentmemory@localhost:5432/agentmemory?sslmode=disable"

# Or use docker compose for the full stack
docker compose up -d
```

See [specs/001-agentmemory-v2-platform/quickstart.md](specs/001-agentmemory-v2-platform/quickstart.md) for detailed setup instructions.

## Key Features

### Memory Pipeline (4-tier)
Observations flow through **working** -> **episodic** -> **semantic** -> **procedural** tiers:
- **Session End** — summarization, consolidation, reflection run automatically
- **Compression** — groups related observations into compact digests
- **Lessons** — extracted from session summaries with confidence scoring and decay

### Smart Search
- **Hybrid BM25 + vector search** — combines full-text and semantic relevance
- **Progressive disclosure** — compact results first, expand on demand
- **Graph traversal** — discovers related observations through the knowledge graph

### Teams & Users
- **Single-team membership** — users belong to one team at a time
- **Visibility modes** — member_choice, team, public
- **API key auth** — create, list, revoke keys via REST or MCP
- **JWT session tokens** — for browser-based access (viewer UI)

### MCP Tools (55 total)
All memory, search, team, governance, and utility operations exposed via MCP:
- **Memory Operations**: observe, save, recall, smart_search, forget, compress_file
- **Session Operations**: sessions, timeline, handoff, recap
- **Lesson Operations**: lesson_save, lesson_recall
- **Team Operations**: create, delete, add_member, remove_member, list_members, feed, share
- **Auth Operations**: create_key, list_keys, revoke_key
- **Action Operations**: action_create, action_update, frontier, next, lease
- **Pipeline & Governance**: consolidate, crystallize, reflect, diagnose, heal, verify, audit, export
- **Working Memory**: slot_create, slot_get, slot_list, slot_replace, slot_delete, slot_append
- **Signals & Coordination**: signal_read, signal_send, sentinel_create, sentinel_trigger, checkpoint
- **Workflow**: sketch_create, sketch_promote, routine_run, snapshot_create, claude_bridge_sync
- **Graph & Insights**: graph_query, relations, profile, patterns, facet_query, facet_tag, insight_list, file_history

### Context Injection
Agent prompts are enriched with 5 context source buckets:
1. Relevant observations from recent sessions
2. Session recap summaries
3. Relevant lessons
4. Graph neighbors via traversal
5. Working memory from slots

Context is assembled, budgeted, and formatted before injection at session start, pre-tool-use, and pre-compact hooks.

### Hooks (13 hook types)
Observations are captured automatically across the agent session lifecycle:
`session_start`, `user_prompt_submit`, `pre_tool_use`, `post_tool_use`, `pre_tool_permission`, `post_tool_permission`, `pre_llm_call`, `post_llm_call`, `pre_compact`, `post_compact`, `pre_session_save`, `post_session_save`, `session_end`

## Architecture

```
agentmemory/
├── cmd/agentmemory/     # CLI entry point (serve, setup, user, migrate, version)
├── internal/
│   ├── auth/            # JWT + API key authentication
│   ├── cmd/             # CLI command implementations
│   ├── config/          # Environment-based configuration
│   ├── db/              # SQL query definitions (sqlc source)
│   ├── handler/         # HTTP handlers (REST, MCP, WebSocket, viewer)
│   ├── mcp/             # MCP tool registration (55 tools)
│   ├── service/         # Business logic services
│   ├── store/           # Generated sqlc database layer
│   └── team/            # Team-specific logic
├── migrations/          # SQL migration files
├── tests/               # Integration tests
├── specs/               # Feature specifications and plans
└── docs/                # Additional documentation
```

## Configuration

Key environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_URL` | (required) | PostgreSQL connection string |
| `DB_MAX_CONNS` | 25 | Max connection pool size |
| `DB_MIN_CONNS` | 5 | Min connection pool size |
| `JWT_SECRET` | `change-me-in-production` | JWT signing secret |
| `PORT` | 8080 | HTTP server port |
| `MIGRATE_ON_STARTUP` | `false` | Run migrations on server start |
| `AGENTMEMORY_RATE_LIMIT` | 100 | Rate limit (req/s, future use) |
| `AGENTMEMORY_SHARE_CONSOLIDATED` | `false` | Share consolidated memories with teams |

## Development

```bash
# Run tests
go test ./...

# Run integration tests (requires Docker)
go test -tags=integration ./tests/integration/

# Regenerate sqlc code
sqlc generate

# Build
go build -o agentmemory ./cmd/agentmemory/
```

## Documentation

- [Specification](specs/001-agentmemory-v2-platform/spec.md) — Full feature specification
- [Plan](specs/001-agentmemory-v2-platform/plan.md) — Implementation plan
- [Tasks](specs/001-agentmemory-v2-platform/tasks.md) — Task breakdown
- [Data Model](specs/001-agentmemory-v2-platform/data-model.md) — Database schema
- [Quickstart](specs/001-agentmemory-v2-platform/quickstart.md) — Setup guide
- [Research](specs/001-agentmemory-v2-platform/research.md) — Technology decisions
