# AgentMemory — Deploy & Operations Guide

End-to-end reference: deploy, create the first admin user, manage teams, and every REST API call as copy-paste `curl` commands.

---

## 1. Deploy

### Option A: docker-compose (quick start)

```bash
git clone <repo>
cd agentmemory
docker-compose up
```

This starts ParadeDB (PostgreSQL 18 + pg_search + pgvector) and the agentmemory server on `:8080`.
Migrations run automatically (`MIGRATE_ON_STARTUP=true` in compose).

### Option B: Manual

**Prerequisites:** PostgreSQL with the `vector` extension (ParadeDB recommended for BM25 full-text search).

```bash
# Build
CGO_ENABLED=0 go build -o agentmemory ./cmd/agentmemory/

# Set required env vars
export DB_URL="postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable"
export JWT_SECRET="$(openssl rand -hex 64)"

# Run DB setup (extensions + migrations, idempotent)
./agentmemory setup

# Start the server
./agentmemory serve --port 8080 --migrate-on-startup
```

### Required environment variables

| Variable | Default | Notes |
|---|---|---|
| `DB_URL` | — | PostgreSQL connection string |
| `JWT_SECRET` | — | Strong random secret for JWT signing |

### Optional environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `MIGRATE_ON_STARTUP` | `false` | Auto-apply migrations on `serve` |
| `JWT_EXPIRY` | `24h` | Token expiry duration (e.g. `72h`, `30m`) |
| `DB_MAX_CONNS` | `25` | Pool max connections |
| `DB_MIN_CONNS` | `5` | Pool min connections |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | `text` | `json` for structured logs |
| `LLM_API_KEY` | — | LLM provider API key |
| `LLM_PROVIDER` | — | LLM provider name |
| `LLM_MODEL` | — | LLM model name |
| `LLM_BASE_URL` | — | Custom LLM API base URL |
| `EMBEDDING_API_KEY` | — | Embedding provider API key |
| `EMBEDDING_PROVIDER` | — | Embedding provider name |
| `EMBEDDING_MODEL` | — | Embedding model name |
| `EMBEDDING_BASE_URL` | — | Custom embedding API base URL |
| `AGENTMEMORY_RATE_LIMIT` | `100` | Requests/second per IP |

---

## 2. Create the First Admin User

There is no seed/bootstrap mechanism. Create the first user via CLI:

```bash
# With password on the command line
./agentmemory user create \
  --db-url "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --email "admin@example.com" \
  --name "Admin" \
  --password "your-secure-password"

# Or let it prompt for the password (more secure)
./agentmemory user create \
  --db-url "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --email "admin@example.com" \
  --name "Admin"
# Enter password: ********
```

Output:
```
User created successfully.
  ID:    a1b2c3d4-...
  Email: admin@example.com
  Name:  Admin
```

**Save the user ID** — you need it to create teams.

> **Note:** There is no "admin" role. The first user has the same privileges as any other user. Access control is team-based.

---

## 3. REST API Reference

All endpoints are served from `http://localhost:8080`.

### Auth token format

- **Session tokens** — returned by `/v1/auth/login` and `/v1/auth/register`, prefixed `st_`. Pass as `?token=st_...` query param or `Authorization: Bearer st_...` header.
- **API keys** — created via `/v1/auth/keys`, prefixed `ak_`. Use the same way. API keys skip CSRF checks.

In the examples below, `$TOKEN` holds a session token (`st_...`).

---

### 3.1 Health check

```bash
# No auth required
curl -s http://localhost:8080/health | jq
```

Response `200`:
```json
{
  "status": "ok",
  "db": "connected",
  "migrations": "ok",
  "version": "2.0.0"
}
```

---

### 3.2 Register a new user

```bash
curl -s -X POST http://localhost:8080/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "secure-password",
    "name": "Alice"
  }' | jq
```

Response `201`:
```json
{
  "token": "st_a1b2c3d4...",
  "expires_at": "2026-06-25T12:00:00Z",
  "user": {
    "id": "uuid",
    "email": "user@example.com",
    "name": "Alice"
  }
}
```

> After registration the user is automatically logged in — the `token` in the response is a valid session token.

---

### 3.3 Login

```bash
curl -s -X POST http://localhost:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "admin@example.com",
    "password": "your-secure-password"
  }' | jq
```

Response `200`:
```json
{
  "token": "st_...",
  "expires_at": "2026-06-25T12:00:00Z",
  "user": {
    "id": "uuid",
    "email": "admin@example.com",
    "name": "Admin"
  }
}
```

Save the token:
```bash
TOKEN="st_..."
```

---

### 3.4 Get current user profile

```bash
curl -s http://localhost:8080/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" | jq
```

Response `200`:
```json
{
  "user": {
    "id": "uuid",
    "email": "admin@example.com",
    "name": "Admin"
  },
  "team": null
}
```

`team` is `null` until the user joins a team.

---

### 3.5 Create an API key

```bash
curl -s -X POST http://localhost:8080/v1/auth/keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label": "my-cli-key"}' | jq
```

Optional: add `"expires_at": "2026-12-31T23:59:59Z"` for an expiry.

Response `201`:
```json
{
  "id": "uuid",
  "label": "my-cli-key",
  "prefix": "ak_abc123...",
  "key": "ak_full_secret_key..."
}
```

> **The full `key` is shown only once.** Save it immediately.

---

### 3.6 List API keys

```bash
curl -s http://localhost:8080/v1/auth/keys \
  -H "Authorization: Bearer $TOKEN" | jq
```

Response `200`:
```json
{
  "keys": [
    {
      "id": "uuid",
      "label": "my-cli-key",
      "prefix": "ak_abc123...",
      "last_used_at": null,
      "created_at": "2026-06-24T...",
      "expires_at": null
    }
  ]
}
```

> Full keys are never returned by the list endpoint — only metadata.

---

### 3.7 Delete (revoke) an API key

```bash
KEY_ID="uuid-from-list"
curl -s -X DELETE http://localhost:8080/v1/auth/keys/$KEY_ID \
  -H "Authorization: Bearer $TOKEN"
```

Response `204` (no body).

---

### 3.8 Record an observation

```bash
curl -s -X POST http://localhost:8080/v1/api/observe \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "my-session-001",
    "type": "note",
    "title": "First observation",
    "narrative": "This is a test observation recorded via the REST API.",
    "concepts": ["test", "api"],
    "files": ["docs/operations.md"],
    "importance": 0.7
  }' | jq
```

Required fields: `session_id`, `type`, `title`, `narrative`.

Response `201`:
```json
{
  "observation_id": "uuid",
  "status": "recorded"
}
```

---

### 3.9 End a session (trigger memory pipeline)

```bash
curl -s -X POST http://localhost:8080/v1/api/session/end \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "my-session-001"}' | jq
```

Response `200`:
```json
{
  "session_id": "my-session-001",
  "status": "ended",
  "summary_queued": true,
  "consolidation_queued": true
}
```

This triggers the full memory pipeline: summarization → consolidation → reflection.

---

### 3.10 Link a git commit to a session

```bash
curl -s -X POST http://localhost:8080/v1/api/session/commit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "my-session-001",
    "sha": "abc123def456",
    "branch": "main",
    "message": "Fix memory leak in session end handler"
  }' | jq
```

Required: `session_id`, `sha`. Optional: `branch`, `message`.

Response `200`:
```json
{
  "session_id": "my-session-001",
  "commit_sha": "abc123def456",
  "status": "linked"
}
```

---

### 3.11 Connect a coding agent (Claude Code / Codex)

```bash
./agentmemory connect \
  --url "http://localhost:8080" \
  --token "$TOKEN"
```

This writes the MCP server config into `~/.claude/settings.json` and/or `~/.codex/settings.json`.

---

### 3.12 Error response format

All errors follow this shape:

```json
{
  "error": "Human-readable message",
  "code": "BAD_REQUEST"
}
```

Error codes: `BAD_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`, `RATE_LIMITED`, `INTERNAL_ERROR`, `SERVICE_UNAVAILABLE`.

---

## 4. Team Management (CLI only)

Team CRUD is **not exposed as REST endpoints**. Use the CLI or MCP tools.

### 4.1 Create a team

```bash
./agentmemory team create \
  --db-url "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --name "my-team" \
  --owner-id "USER_UUID" \
  --default-visibility "member_choice"
```

`--default-visibility` options: `member_choice` (default), `team`, `public`.

### 4.2 List teams

```bash
# All teams
./agentmemory team list --db-url "postgres://..."

# Filter by owner
./agentmemory team list --db-url "postgres://..." --owner-id "USER_UUID"

# JSON output
./agentmemory team list --db-url "postgres://..." --json
```

### 4.3 Add a member to a team

```bash
# Requires the caller to be the team owner
./agentmemory team add \
  --db-url "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --team-id "TEAM_UUID" \
  --user-id "USER_TO_ADD_UUID" \
  --caller-id "OWNER_UUID"
```

### 4.4 Remove a member from a team

```bash
./agentmemory team remove \
  --db-url "postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --team-id "TEAM_UUID" \
  --user-id "USER_TO_REMOVE_UUID" \
  --caller-id "OWNER_UUID"
```

---

## 5. End-to-End Workflow

```bash
# ─── 1. Deploy ───
export DB_URL="postgres://agentmemory:agentmemory@localhost:5432/agentmemory?sslmode=disable"
export JWT_SECRET="$(openssl rand -hex 64)"
./agentmemory setup
./agentmemory serve --port 8080 &

# ─── 2. Create admin user ───
./agentmemory user create \
  --email "admin@example.com" \
  --name "Admin" \
  --password "admin-pass"
# → USER_ID = "aaaa-1111-..."

# ─── 3. Create a second user ───
./agentmemory user create \
  --email "alice@example.com" \
  --name "Alice" \
  --password "alice-pass"
# → ALICE_ID = "bbbb-2222-..."

# ─── 4. Login as admin ───
TOKEN=$(curl -s -X POST http://localhost:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"admin-pass"}' | jq -r .token)

# ─── 5. Create a team via CLI ───
./agentmemory team create \
  --name "engineering" \
  --owner-id "aaaa-1111-..." \
  --default-visibility "member_choice"
# → TEAM_ID = "cccc-3333-..."

# ─── 6. Add Alice to the team ───
./agentmemory team add \
  --team-id "cccc-3333-..." \
  --user-id "bbbb-2222-..." \
  --caller-id "aaaa-1111-..."

# ─── 7. Record observations ───
curl -s -X POST http://localhost:8080/v1/api/observe \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "sprint-42",
    "type": "decision",
    "title": "Adopt ParadeDB for search",
    "narrative": "Decided to use ParadeDB for BM25 full-text search.",
    "concepts": ["search", "architecture"],
    "files": ["go.mod", "go.sum"],
    "importance": 0.8
  }' | jq

# ─── 8. End session (trigger memory pipeline) ───
curl -s -X POST http://localhost:8080/v1/api/session/end \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "sprint-42"}' | jq

# ─── 9. Link a git commit ───
curl -s -X POST http://localhost:8080/v1/api/session/commit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "session_id": "sprint-42",
    "sha": "'$(git rev-parse HEAD)'",
    "branch": "main",
    "message": "Add ParadeDB search integration"
  }' | jq

# ─── 10. Connect Claude Code ───
./agentmemory connect --url "http://localhost:8080" --token "$TOKEN"
```

---

## 6. Authentication Summary

| Scenario | Token type | Prefix | CSRF |
|---|---|---|---|
| User session (login/register) | JWT | `st_` | Required |
| API / automation | API key | `ak_` | Skipped |
| WebSocket (`/v1/socket`) | JWT only | `st_` | N/A (upgraded) |

Pass tokens via `?token=...` query param or `Authorization: Bearer ...` header.

---

## 7. Architecture Notes

- **REST API** (13 endpoints): auth, health, observation recording, session lifecycle, SPA viewer.
- **MCP interface** (`/v1/mcp`, 50+ tools): team CRUD, user management, memory/lesson CRUD, search, pipeline, slots, signals, sentinels, checkpoints, sketches, routines, snapshots, file history.
- **CLI** (`agentmemory`): user creation, team management, database setup, agent connection.
- Team management is **not** available via REST — use the CLI or MCP tools.
