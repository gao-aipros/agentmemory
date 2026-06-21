# v2 Core Architecture & Database Access

## Database

PostgreSQL + pgvector + pg_search (ParadeDB) unified in one stack.

- **Docker:** `paradedb/paradedb` official image, pg_search + vector extensions only
- **Core table:** single observation table with BM25 (ParadeDB) + HNSW (pgvector) + B-tree indexes
- **Hybrid query:** one SQL FULL OUTER JOIN

## Frameworks

- **pgx + pgxpool:** connection pool management
- **sqlc:** type-safe SQL codegen, all CRUD and queries — no raw SQL in Go code
- **langchaingo:** embedding + LLM abstraction
- **modelcontextprotocol/go-sdk:** MCP server

## MCP Integration

- Official go-sdk -> `NewStreamableHTTPHandler` + `JSONResponse: true` + `AddTool`/`AddResource`/`AddPrompt`

## Database Access Rules

- pgxpool: connection pool management
- sqlc: all CRUD and queries, no raw SQL in Go code
- All SQL in `.sql` files, compiled to type-safe Go functions
- ParadeDB/pgvector/WITH RECURSIVE syntax passed through sqlc, zero runtime overhead
