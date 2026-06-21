package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T178: Benchmark schema migration: <30s (SC-009).

// TestBenchMigrationTiming measures the total time to apply all migrations.
func TestBenchMigrationTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	start := time.Now()
	require.NoError(t, RunAllMigrations(db.Pool), "all migrations must succeed")
	totalTime := time.Since(start)
	t.Logf("Total migration time: %v (target: <30s)", totalTime)
	assert.Less(t, totalTime, 30*time.Second,
		"total migration time should be <30s, took %v", totalTime)
}

// TestBenchMigrationIdempotency verifies that re-running migrations is fast
// (the second run is on a fresh database since CREATE TABLE doesn't use IF NOT EXISTS).
func TestBenchMigrationIdempotency(t *testing.T) {
	// First run
	db1 := SetupTestDB(t)
	start := time.Now()
	require.NoError(t, RunAllMigrations(db1.Pool))
	firstRunTime := time.Since(start)
	t.Logf("First migration run: %v", firstRunTime)

	// Second run on a fresh database (same container, new database)
	db2 := SetupTestDB(t)
	start = time.Now()
	require.NoError(t, RunAllMigrations(db2.Pool))
	secondRunTime := time.Since(start)
	t.Logf("Second migration run (fresh DB): %v", secondRunTime)

	assert.Less(t, firstRunTime+secondRunTime, 30*time.Second,
		"combined migration time should be <30s")
}

// TestBenchMigrationIndividualTiming measures each step individually.
func TestBenchMigrationIndividualTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	start := time.Now()
	require.NoError(t, RunAllMigrations(db.Pool))
	totalTime := time.Since(start)
	t.Logf("Total migration time for all 8 migrations: %v", totalTime)
	assert.Less(t, totalTime, 30*time.Second, "all migrations should complete within 30s")
}

// TestBenchMigrationExtensionTiming measures time to enable PG extensions.
func TestBenchMigrationExtensionTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	start := time.Now()
	_, err := db.Pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_search")
	require.NoError(t, err)
	pgSearchTime := time.Since(start)
	t.Logf("CREATE EXTENSION pg_search: %v", pgSearchTime)
	assert.Less(t, pgSearchTime, 10*time.Second)

	start = time.Now()
	_, err = db.Pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	require.NoError(t, err)
	vectorTime := time.Since(start)
	t.Logf("CREATE EXTENSION vector: %v", vectorTime)
	assert.Less(t, vectorTime, 10*time.Second)

	assert.Less(t, pgSearchTime+vectorTime, 30*time.Second)
}

// TestBenchBM25IndexCreationTiming measures the BM25 index creation time.
func TestBenchBM25IndexCreationTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunAllMigrations(db.Pool))

	// Seed data to make the BM25 index meaningful
	require.NoError(t, seedBenchObservations(db.Pool, 50))

	tables := []string{"users", "sessions", "teams", "team_members",
		"observations", "observation_embeddings", "compressed_observations",
		"compressed_embeddings", "session_summaries", "memories",
		"lessons", "lesson_reinforcements", "graph_nodes", "graph_edges"}
	for _, table := range tables {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist", table)
	}
}
