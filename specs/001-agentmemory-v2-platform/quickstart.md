# Quickstart: AgentMemory v2 Platform Migration

**Feature**: AgentMemory v2 Platform Migration
**Date**: 2026-06-21

## Prerequisites

- Go 1.26.4 installed
- Docker (for ParadeDB PostgreSQL container and testcontainers)
- `git` for version control
- Host coding agent (Claude Code / Codex) for integration testing

## Setup

```bash
# 1. Start ParadeDB PostgreSQL
docker run -d --name agentmemory-db \
  -e POSTGRES_PASSWORD=agentmemory \
  -p 5432:5432 \
  paradedb/paradedb:0.24.1-pg18

# 2. Wait for PostgreSQL to be ready
until docker exec agentmemory-db pg_isready -U postgres; do sleep 1; done

# 3. Initialize schema
go run cmd/agentmemory/main.go setup \
  --db-url "postgres://postgres:agentmemory@localhost:5432/agentmemory?sslmode=disable"

# 4. Create admin user
go run cmd/agentmemory/main.go user create \
  --email "admin@example.com" \
  --password "secure-password" \
  --name "Admin User" \
  --db-url "postgres://postgres:agentmemory@localhost:5432/agentmemory?sslmode=disable"
```

## Run Server

```bash
# Start the server (single binary, single port)
go run cmd/agentmemory/main.go serve \
  --db-url "postgres://postgres:agentmemory@localhost:5432/agentmemory?sslmode=disable" \
  --port 8080
```

## Verify Health

```bash
# Health check
curl http://localhost:8080/health
# Expected: {"status":"ok","db":"connected"}
```

## Verify Login & Auth

```bash
# Login to get session token
curl -X POST http://localhost:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"secure-password"}'

# Response includes: {"token":"st_eyJ...","user":{...}}

# Create API key (using session token)
export TOKEN="st_eyJ..."
curl -X POST http://localhost:8080/v1/auth/keys \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"label":"test-key"}'
```

## Verify Core Pipeline

```bash
# Record an observation via REST API (simulating a hook)
curl -X POST http://localhost:8080/v1/api/observe \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "user_prompt",
    "title": "Test prompt",
    "narrative": "User asked about PostgreSQL connection pooling",
    "concepts": ["postgresql", "pooling"],
    "session_id": "test-session-001"
  }'

# Expected: {"observation_id":"obs_...","status":"recorded"}

# End the session
curl -X POST http://localhost:8080/v1/api/session/end \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"test-session-001"}'

# Expected: {"session_id":"test-session-001","status":"ended","summary_queued":true}

# Wait for async compression + summarization (~30s)
sleep 35

# Search for the observation
curl -X POST http://localhost:8080/v1/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"tools/call",
    "params":{
      "name":"memory_recall",
      "arguments":{"query":"PostgreSQL connection pooling"}
    },
    "id":1
  }'

# Expected: Results containing the observation about PostgreSQL
```

## Verify Team Management

```bash
# Create a team
curl -X POST http://localhost:8080/v1/mcp \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "method":"tools/call",
    "params":{
      "name":"team_create",
      "arguments":{"name":"Dev Team","default_visibility":"member_choice"}
    },
    "id":2
  }'

# Expected: {"team_id":"team_...","name":"Dev Team"}
```

## Run Tests

```bash
# Unit tests (fast, no DB required)
go test ./tests/unit/...

# Integration tests (requires Docker for testcontainers)
go test ./tests/integration/...
```

## Verify End-to-End (Agent Integration)

```bash
# Connect agentmemory to the current workspace
go run cmd/agentmemory/main.go connect \
  --url "http://localhost:8080" \
  --token "$TOKEN"

# Run a simple agent session and verify capture:
# 1. Ask an agent to do something simple
# 2. Check sessions: memory_sessions
# 3. Check recall: memory_recall "the task you just did"
```

## Expected Outcomes

| Check | Expected Result |
|-------|----------------|
| `/health` | 200, `{"status":"ok","db":"connected"}` |
| Login with valid credentials | 200 with `st_` JWT token |
| Login with invalid credentials | 401 |
| POST `/v1/api/observe` with valid token | 200, observation recorded |
| POST `/v1/api/observe` with invalid token | 401 |
| POST `/v1/api/session/end` | 200, summary + consolidation queued |
| Search after compression (~35s wait) | Results containing test observation |
| API key authenticates for `/v1/api/*` | 200 |
| API key rejected for `/` (UI route) | 403 |
| `agentmemory setup` on clean DB | All 25 tables + 42 indexes created |
| `agentmemory migrate` with pending migration | Migrations applied in order |
| Integration tests | All pass against real ParadeDB via testcontainers |

## Troubleshooting

| Symptom | Check |
|---------|-------|
| "connection refused" | Is Docker running? Is ParadeDB container up? |
| 503 health check | Is PostgreSQL accepting connections? Are migrations pending? |
| Search returns empty | Has compression completed? (wait ~30s after observe) |
| Token rejected | Has the token expired? (default 24h) Check `JWT_EXPIRY` |
| "too many connections" | Check pgxpool max connections config |
