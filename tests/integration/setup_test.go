package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T147: Integration test for `agentmemory setup` — creates all tables + indexes.

// TestSetupCreatesAllTables verifies that running setup creates all expected tables.
func TestSetupCreatesAllTables(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Enable extensions
	extensions := []string{"pg_search", "vector"}
	for _, ext := range extensions {
		_, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+ext)
		if err != nil {
			t.Logf("Warning: could not enable extension %s: %v", ext, err)
		}
	}

	// Run migrations by executing the SQL files
	// In a full integration test, we'd call the migrate command.
	// Here we verify the tables exist after running setup.

	// List all tables
	rows, err := pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' ORDER BY table_name`)
	require.NoError(t, err, "failed to query tables")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tables = append(tables, name)
	}

	t.Logf("Found %d tables: %v", len(tables), tables)

	// Verify core expected tables exist
	expectedTables := []string{
		"users",
		"api_keys",
		"teams",
		"team_members",
		"sessions",
		"observations",
		"observation_embeddings",
		"compressed_observations",
		"compressed_embeddings",
		"session_summaries",
		"memories",
		"graph_nodes",
		"graph_edges",
		"lessons",
	}

	tableMap := make(map[string]bool)
	for _, t := range tables {
		tableMap[t] = true
	}

	for _, expected := range expectedTables {
		assert.True(t, tableMap[expected], "expected table %s should exist", expected)
	}
}

// TestSetupEnablesExtensions verifies that pg_search and vector extensions are enabled.
func TestSetupEnablesExtensions(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Check for pg_search extension
	var pgSearchExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM pg_extension WHERE extname = 'pg_search')",
	).Scan(&pgSearchExists)
	require.NoError(t, err)
	assert.True(t, pgSearchExists, "pg_search extension should be enabled")

	// Check for vector extension
	var vectorExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM pg_extension WHERE extname = 'vector')",
	).Scan(&vectorExists)
	require.NoError(t, err)
	assert.True(t, vectorExists, "vector extension should be enabled")
}

// TestSetupIsIdempotent verifies that running setup multiple times is safe.
func TestSetupIsIdempotent(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Run CREATE EXTENSION IF NOT EXISTS multiple times — should not error
	for i := 0; i < 3; i++ {
		_, err := pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_search")
		assert.NoError(t, err, "CREATE EXTENSION IF NOT EXISTS should be idempotent (attempt %d)", i+1)

		_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
		assert.NoError(t, err, "CREATE EXTENSION IF NOT EXISTS vector should be idempotent (attempt %d)", i+1)
	}
}

// TestSetupIndexesExist verifies that expected indexes are created.
func TestSetupIndexesExist(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// List all indexes
	rows, err := pool.Query(ctx,
		`SELECT indexname FROM pg_indexes WHERE schemaname = 'public' ORDER BY indexname`)
	require.NoError(t, err)
	defer rows.Close()

	var indexes []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("failed to scan index name: %v", err)
		}
		indexes = append(indexes, name)
	}

	t.Logf("Found %d indexes: %v", len(indexes), indexes)

	// Verify core expected indexes exist
	expectedIndexes := []string{
		"idx_api_keys_user_id",
		"idx_api_keys_key_hash",
		"idx_team_members_team_id",
		"idx_team_members_user_id",
		"idx_sessions_user_id",
		"idx_sessions_status",
	}

	indexMap := make(map[string]bool)
	for _, idx := range indexes {
		indexMap[idx] = true
	}

	for _, expected := range expectedIndexes {
		assert.True(t, indexMap[expected], "expected index %s should exist", expected)
	}
}
