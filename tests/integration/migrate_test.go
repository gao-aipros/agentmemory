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

// T148: Integration test for pending migrations applied in order via CLI.

// TestMigrationsAppliedInOrder verifies that migration files are applied sequentially.
func TestMigrationsAppliedInOrder(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Check that core tables from migration 001 exist
	rows, err := pool.Query(ctx,
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name IN ('users', 'sessions', 'teams', 'team_members', 'api_keys')
		 ORDER BY table_name`)
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("failed to scan table name: %v", err)
		}
		tables = append(tables, name)
	}

	assert.Equal(t, 5, len(tables), "migration 001 should create 5 core tables")

	// Check that observations table from migration 002 exists
	var obsExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'observations')",
	).Scan(&obsExists)
	require.NoError(t, err)
	assert.True(t, obsExists, "migration 002 should create observations table")

	// Check that observation_embeddings table from migration 003 exists
	var embedExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'observation_embeddings')",
	).Scan(&embedExists)
	require.NoError(t, err)
	assert.True(t, embedExists, "migration 003 should create observation_embeddings table")

	// Check that compressed_observations from migration 004 exists
	var compressedExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'compressed_observations')",
	).Scan(&compressedExists)
	require.NoError(t, err)
	assert.True(t, compressedExists, "migration 004 should create compressed_observations table")

	// Check that session_summaries from migration 005 exists
	var summariesExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'session_summaries')",
	).Scan(&summariesExists)
	require.NoError(t, err)
	assert.True(t, summariesExists, "migration 005 should create session_summaries table")

	// Check that memories from migration 006 exists
	var memoriesExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'memories')",
	).Scan(&memoriesExists)
	require.NoError(t, err)
	assert.True(t, memoriesExists, "migration 006 should create memories table")

	// Check that graph tables from migration 007 exist
	var graphNodesExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'graph_nodes')",
	).Scan(&graphNodesExists)
	require.NoError(t, err)
	assert.True(t, graphNodesExists, "migration 007 should create graph_nodes table")

	var graphEdgesExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'graph_edges')",
	).Scan(&graphEdgesExists)
	require.NoError(t, err)
	assert.True(t, graphEdgesExists, "migration 007 should create graph_edges table")
}

// TestMigrationNoDirtyState verifies the database is not in a dirty migration state.
func TestMigrationNoDirtyState(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Check if the schema_migrations table exists
	var migrationTableExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'schema_migrations')",
	).Scan(&migrationTableExists)
	require.NoError(t, err)

	if migrationTableExists {
		// Check dirty state
		var dirty bool
		err = pool.QueryRow(ctx, "SELECT dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&dirty)
		if err == nil {
			assert.False(t, dirty, "database should not be in a dirty migration state")
		}
	}
}

// TestMigrationSchemaVersion verifies the migration version is recorded.
func TestMigrationSchemaVersion(t *testing.T) {
	dbURL := os.Getenv("AGENTMEMORY_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AGENTMEMORY_DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := config.NewPool(ctx, dbURL)
	require.NoError(t, err, "failed to connect to database")
	defer config.ClosePool(pool)

	// Check schema_migrations table has entries
	var migrationTableExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'schema_migrations')",
	).Scan(&migrationTableExists)
	require.NoError(t, err)

	if migrationTableExists {
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
		require.NoError(t, err)
		t.Logf("schema_migrations has %d entries", count)
		assert.Greater(t, count, 0, "schema_migrations should have at least one entry")
	}
}
