# v2 Tech Stack

Finalized 2026-06-21.

## Go Version

Go 1.26.4

## HTTP Router

chi (github.com/go-chi/chi/v5) — lightweight, idiomatic Go router with middleware support.

## Logging

log/slog — Go standard library structured logging.
Writes to stdout. Docker manages log collection and rotation.
No external logging library needed.

## Database Migration

golang-migrate (github.com/golang-migrate/migrate/v4).
DDL files live in `migrations/` directory.
Applied at startup or via CLI `migrate` subcommand.

## SQL Codegen

sqlc — type-safe SQL codegen from `.sql` query files.
All query files in `internal/db/queries/`.
Compiles to type-safe Go functions. Zero raw SQL in application code.
ParadeDB-specific syntax (bm25, pgvector, WITH RECURSIVE) passes through sqlc
with zero runtime overhead — sqlc treats them as opaque SQL and generates
correct Go wrappers.

## DB Driver

pgx + pgxpool — native PostgreSQL driver for Go.
pgxpool manages connection pooling with configurable pool size, timeouts, and health checks.

## Embedding + LLM

langchaingo — unified interface for both embeddings and LLM.
Provider-agnostic: swap providers via env vars without code changes.
See 05-provider.md for detailed provider architecture.

## MCP

modelcontextprotocol/go-sdk — official Go SDK for MCP.
Configuration: `NewStreamableHTTPHandler` + `JSONResponse: true` + `AddTool`/`AddResource`/`AddPrompt`.
All 51 v1 tools migrated + new team/auth tools.

## Testing

testify — assertions and test suite organization.
testcontainers-go — ephemeral ParadeDB PostgreSQL containers for integration tests.
Integration-test-heavy approach: focus on real DB behavior, not mocks.
No unit-test-only requirement; prefer integration tests that exercise actual PostgreSQL.

## Config

Environment variables — consistent with v0 pattern.
No config files, no CLI flags for configuration.
All configuration via env vars, documented in a central config package.
