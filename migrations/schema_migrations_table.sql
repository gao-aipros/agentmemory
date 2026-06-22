-- sqlc schema definition for the golang-migrate-managed schema_migrations table.
-- This file is NOT a migration; it is only read by sqlc during code generation.
-- golang-migrate ignores it because it does not match the <version>_<description>.up.sql pattern.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    dirty BOOLEAN NOT NULL
);
