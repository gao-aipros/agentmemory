# v2 Core Architecture & Database Access

Finalized 2026-06-21.

## Database

PostgreSQL + pgvector + pg_search (ParadeDB) unified in one stack.

- **Docker image:** `paradedb/paradedb:latest` (PG18) official image
- **Extensions enabled:** pg_search (BM25 full-text search) + pgvector (HNSW vector index)
- **Extensions NOT enabled:** postgis, pg_ivm, pg_cron (pre-installed in image but kept off for minimal footprint)
- **Graph traversal:** PostgreSQL native `WITH RECURSIVE` CTE (Apache AGE NOT used, not in ParadeDB image)
- **Core table:** single `observations` table with triple index strategy:
  - BM25 full-text index (ParadeDB pg_search) on `(id, title, narrative, facts)`
  - HNSW vector index (pgvector) on `observation_embeddings(embedding vector_cosine_ops)`
  - B-tree indexes on `(timestamp, type, importance, session_id, concepts, files)`
- **Hybrid query:** one SQL `FULL OUTER JOIN` combining all three search dimensions — replaces v0's three independent JS Map searches + RRF merge

## Language

Go. Single binary deployment. Compile-time type safety.

## Frameworks

### pgx + pgxpool
Connection pool management. Handles:
- Pool size configuration
- Connection timeouts
- Health checks
- Prepared statement caching

### sqlc
Type-safe SQL codegen. Rules:
- ALL CRUD and queries go through sqlc — **zero raw SQL in Go code**
- ALL SQL lives in `.sql` files under `internal/db/queries/`
- sqlc compiles `.sql` → type-safe Go functions with proper parameter/return types
- ParadeDB-specific syntax (bm25 operator, `USING bm25`, `<->` vector distance, `WITH RECURSIVE`) passes through sqlc — sqlc treats unrecognized syntax as opaque SQL and generates correct Go wrappers with zero runtime overhead
- Generated code goes in `internal/store/` — never hand-edit

### langchaingo
Embedding + LLM abstraction layer. Provides:
- `embeddings.Embedder` interface for embedding providers
- `llms.Model` interface for LLM providers
- Provider-agnostic: swap OpenAI/Anthropic/Voyage/Ollama via env vars

### modelcontextprotocol/go-sdk
Official Go SDK for MCP server. Configuration:
- `NewStreamableHTTPHandler` — serves MCP over HTTP (not stdio)
- `JSONResponse: true` — JSON wire format
- `AddTool` / `AddResource` / `AddPrompt` — register capabilities
- Single `/v1/mcp` endpoint serves all MCP traffic

## Why This Stack

- **ParadeDB** bundles pg_search + pgvector in one image — no need to compile extensions separately. Single DB handles full-text, vector, relational, and graph (via CTE) queries.
- **sqlc** provides compile-time SQL verification — catches query errors at build time, not runtime. Generated Go code is fast (no reflection) and type-safe.
- **langchaingo** decouples provider choice from application code — add new embedding/LLM providers without changing business logic.
- **MCP go-sdk** is the official implementation — stays current with protocol changes, reduces maintenance burden vs hand-rolled MCP server.
- **chi** is the most widely-used idiomatic Go router — lightweight, middleware-friendly, no magic.
