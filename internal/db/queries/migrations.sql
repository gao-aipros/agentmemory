-- name: GetMigrationVersion :one
SELECT version, dirty FROM schema_migrations LIMIT 1;

-- name: CheckSchemaMigrationsTableExists :one
SELECT EXISTS (
    SELECT FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'schema_migrations'
);

-- name: CreatePgSearchExtension :exec
CREATE EXTENSION IF NOT EXISTS pg_search;

-- name: CreateVectorExtension :exec
CREATE EXTENSION IF NOT EXISTS vector;

-- name: ListPublicTables :many
SELECT table_name::text AS table_name FROM information_schema.tables
WHERE table_schema = 'public' ORDER BY table_name;
