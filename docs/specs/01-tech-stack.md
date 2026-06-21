# v2 Tech Stack

- **Go version:** 1.26.4
- **HTTP router:** chi (github.com/go-chi/chi/v5)
- **Logging:** log/slog, writes to stdout, Docker manages log collection
- **Database migration:** golang-migrate (github.com/golang-migrate/migrate/v4), DDL files in migrations/
- **SQL codegen:** sqlc, .sql query files in internal/db/queries/
- **DB driver:** pgx + pgxpool
- **Embedding + LLM:** langchaingo (unified interface)
- **MCP:** modelcontextprotocol/go-sdk
- **Testing:** testify + testcontainers-go (for PostgreSQL integration tests)
- **Config:** env vars (consistent with v0 pattern)
