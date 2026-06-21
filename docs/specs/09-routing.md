# v2 Routing & Deployment

## Single Port HTTP

| Path | Auth | Purpose |
|------|------|---------|
| `/` | st_ | SPA static files |
| `/health` | none | Docker health check |
| `/v1/mcp` | st_, ak_ | MCP Streamable HTTP |
| `/v1/api/*` | st_, ak_ | REST API |
| `/v1/auth/login` | none | Login + TOTP, returns JWT |
| `/v1/auth/keys` | st_ | Manage own API keys |
| `/v1/socket` | st_ | WebSocket (viewer live updates) |

## Token Format

- `Authorization: Bearer st_xxx` → JWT (UI/session access)
- `Authorization: Bearer ak_xxx` → API key hash prefix (API access)
- API keys cannot access UI routes

## Deployment

- Go app Docker image + `paradedb/paradedb:latest` (PG18) official image
- `docker-compose.yml`
- `DATABASE_URL` connection string

## WebSocket

- `/v1/socket` with st_ auth for viewer live updates
- Protocol follows v1 behavior
