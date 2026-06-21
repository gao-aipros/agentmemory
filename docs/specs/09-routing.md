# v2 Routing & Deployment

Finalized 2026-06-21.

## Single Port HTTP

ALL traffic goes through one port. No separate ports for MCP, API, or UI.

## Route Table

| Path | Auth Required | Purpose | Notes |
|------|--------------|---------|-------|
| `/` | st_ (JWT) | SPA static files | Viewer web app, served from embedded filesystem |
| `/health` | none | Docker health check | Returns 200 if DB connection is alive |
| `/v1/mcp` | st_ (JWT) or ak_ (API key) | MCP Streamable HTTP | All 51+ MCP tools via this single endpoint |
| `/v1/api/*` | st_ (JWT) or ak_ (API key) | REST API | Hook scripts call this. General CRUD for authorized clients |
| `/v1/auth/login` | none | Login + TOTP | Accepts email + password, optionally TOTP code. Returns JWT (`st_` token) |
| `/v1/auth/keys` | st_ (JWT) | Manage API keys | Create, list, revoke personal API keys |
| `/v1/socket` | st_ (JWT) | WebSocket | Viewer live updates. Protocol follows v1 behavior |

## Token Format

```
Authorization: Bearer st_<jwt_token>
Authorization: Bearer ak_<api_key_hash_prefix>
```

### st_ Token (Session Token)
- Type: JWT
- Issued by `/v1/auth/login` after successful authentication (including TOTP if enabled)
- Contains: user_id, team_id (if any), issued_at, expires_at
- Used for: UI access, viewer WebSocket, initial MCP/session auth
- Expires: configurable, default TBD

### ak_ Token (API Key)
- Type: opaque token with `ak_` prefix
- Created by user via `/v1/auth/keys`
- Stored as hash in `api_keys` table (never stored in plaintext)
- Used for: MCP access from agents, REST API access from scripts
- Can be revoked individually
- CANNOT access UI routes (`/`, `/v1/socket`) — API keys are for programmatic access only

## Token Scope Enforcement

| Route Group | st_ (JWT) | ak_ (API Key) |
|------------|-----------|---------------|
| UI (`/`, `/v1/socket`) | ✅ | ❌ |
| MCP (`/v1/mcp`) | ✅ | ✅ |
| REST API (`/v1/api/*`) | ✅ | ✅ |
| Auth management (`/v1/auth/keys`) | ✅ | ❌ |

API keys cannot manage other API keys — only the user's session token can.

---

## Deployment

### Docker Compose
```yaml
services:
  agentmemory:
    build: .
    ports:
      - "${PORT:-3113}:3113"
    environment:
      - DATABASE_URL=postgres://user:pass@db:5432/agentmemory
      - EMBEDDING_PROVIDER=openai
      - EMBEDDING_API_KEY=${EMBEDDING_API_KEY}
      - LLM_PROVIDER=anthropic
      - LLM_API_KEY=${LLM_API_KEY}
    depends_on:
      db:
        condition: service_healthy

  db:
    image: paradedb/paradedb:latest
    environment:
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=pass
      - POSTGRES_DB=agentmemory
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "user", "-d", "agentmemory"]
      interval: 5s
      retries: 5

volumes:
  pgdata:
```

### Go App Docker Image
- Multi-stage build: compile Go binary → minimal runtime image
- Single binary, no external dependencies
- Embedded SPA static files (via `embed` package)
- Exposes port from `PORT` env var (default 3113, consistent with v0)

### Database Connection
- `DATABASE_URL` env var — standard PostgreSQL connection string
- App runs migrations on startup (or via CLI `migrate` command)
- Health check endpoint verifies DB connectivity

---

## WebSocket

### Endpoint
`/v1/socket` with st_ (JWT) authentication.

### Protocol
Follows v1 viewer protocol — the existing viewer HTML at `src/viewer/index.html`
defines the message format. v2 implements the server side of this protocol.

### Purpose
Real-time updates for the viewer dashboard:
- New observations arriving
- Session status changes
- Consolidation events
