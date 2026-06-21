# v2 Go Project Layout

Standard Go convention:

```
cmd/agentmemory/main.go      # entry point
internal/
  handler/                    # HTTP handlers (REST + MCP)
  service/                    # business logic
  store/                      # sqlc-generated DB access
  db/queries/                 # .sql query files
  mcp/                        # MCP tool registration
  hooks/                      # plugin hook scripts (delegates to REST API)
  auth/                       # JWT + API key authentication
  team/                       # team management logic
  config/                     # env-var parsing
migrations/                   # golang-migrate DDL files
```
