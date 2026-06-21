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

	ctx := context.Background()

	// Measure migration time using the embedded SQL from search_hybrid_test.go
	start := time.Now()

	migrations := []struct {
		name string
		sql  string
	}{
		{"001_initial_schema", migration001},
		{"002_observations", migration002},
		{"003_embeddings", migration003},
		{"004_compressed", migration004},
		{"005_summaries", migration005},
		{"006_memories", migration006},
		{"007_graph", migration007},
	}

	var timings []time.Duration
	for _, m := range migrations {
		mStart := time.Now()
		_, err := db.Pool.Exec(ctx, m.sql)
		require.NoError(t, err, "migration %s must succeed", m.name)
		elapsed := time.Since(mStart)
		timings = append(timings, elapsed)
		t.Logf("Migration %s: %v", m.name, elapsed)
	}

	totalTime := time.Since(start)
	t.Logf("Total migration time: %v (target: <30s)", totalTime)
	assert.Less(t, totalTime, 30*time.Second,
		"total migration time should be <30s, took %v", totalTime)

	// Individual migrations should each be fast
	for i, m := range migrations {
		assert.Less(t, timings[i], 10*time.Second,
			"migration %s should complete within 10s", m.name)
	}

	// Verify all tables exist after migration
	// Note: embedded migrations differ from full schema migrations —
	// they create the tables that the search/integration tests require.
	tables := []string{
		"sessions", "teams", "team_members",
		"observations", "observation_embeddings", "compressed_observations",
		"compressed_embeddings", "session_summaries", "memories",
		"lessons", "lesson_reinforcements", "graph_nodes", "graph_edges",
	}

	for _, table := range tables {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "table %s should exist after migration", table)
	}
}

// TestBenchMigrationIdempotency verifies that re-running migrations is safe
// and fast (a key requirement for startup resilience).
func TestBenchMigrationIdempotency(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	allMigrations := []string{
		migration001, migration002, migration003,
		migration004, migration005, migration006, migration007,
	}

	// First run: measure initial migration time
	start := time.Now()
	for _, m := range allMigrations {
		_, err := db.Pool.Exec(ctx, m)
		require.NoError(t, err)
	}
	firstRunTime := time.Since(start)
	t.Logf("First migration run: %v", firstRunTime)

	// Second run: measure idempotent re-run time
	start = time.Now()
	for _, m := range allMigrations {
		_, err := db.Pool.Exec(ctx, m)
		require.NoError(t, err, "idempotent re-run must not error")
	}
	secondRunTime := time.Since(start)
	t.Logf("Second (idempotent) run: %v", secondRunTime)

	// Idempotent re-run should be faster than the first run
	// (all CREATE IF NOT EXISTS should be no-ops)
	assert.Less(t, secondRunTime, firstRunTime+1*time.Second,
		"idempotent re-run should not take longer than initial run")

	// Total time should be well under 30s
	totalTime := firstRunTime + secondRunTime
	assert.Less(t, totalTime, 30*time.Second,
		"combined migration time should be <30s")
}

// TestBenchMigrationIndividualTiming measures each migration step timing individually.
func TestBenchMigrationIndividualTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Migration 001 is the largest (5 tables) — should still be fast
	start := time.Now()
	_, err := db.Pool.Exec(ctx, migration001)
	require.NoError(t, err)
	m001Time := time.Since(start)
	t.Logf("Migration 001 (initial schema, 5 tables): %v", m001Time)
	assert.Less(t, m001Time, 5*time.Second, "migration 001 should complete within 5s")

	// Migration 002: observations table
	start = time.Now()
	_, err = db.Pool.Exec(ctx, migration002)
	require.NoError(t, err)
	m002Time := time.Since(start)
	t.Logf("Migration 002 (observations): %v", m002Time)
	assert.Less(t, m002Time, 5*time.Second, "migration 002 should complete within 5s")

	// Migration 003: embeddings + vector extension
	start = time.Now()
	_, err = db.Pool.Exec(ctx, migration003)
	require.NoError(t, err)
	m003Time := time.Since(start)
	t.Logf("Migration 003 (embeddings + vector ext): %v", m003Time)
	assert.Less(t, m003Time, 10*time.Second, "migration 003 should complete within 10s")

	// Migration 007 is the most complex (graph + bm25 + functions)
	// Run all intermediate migrations first
	for _, m := range []string{migration004, migration005, migration006} {
		_, err = db.Pool.Exec(ctx, m)
		require.NoError(t, err)
	}

	start = time.Now()
	_, err = db.Pool.Exec(ctx, migration007)
	require.NoError(t, err)
	m007Time := time.Since(start)
	t.Logf("Migration 007 (graph + bm25 + functions): %v", m007Time)
	assert.Less(t, m007Time, 15*time.Second, "migration 007 should complete within 15s")
}

// TestBenchMigrationExtensionTiming measures time to enable PG extensions.
func TestBenchMigrationExtensionTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Measure pg_search extension creation
	start := time.Now()
	_, err := db.Pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pg_search")
	require.NoError(t, err)
	pgSearchTime := time.Since(start)
	t.Logf("CREATE EXTENSION pg_search: %v", pgSearchTime)
	assert.Less(t, pgSearchTime, 10*time.Second, "pg_search extension should create within 10s")

	// Measure vector extension creation
	start = time.Now()
	_, err = db.Pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	require.NoError(t, err)
	vectorTime := time.Since(start)
	t.Logf("CREATE EXTENSION vector: %v", vectorTime)
	assert.Less(t, vectorTime, 10*time.Second, "vector extension should create within 10s")

	// Both extensions combined should be well under 30s
	totalExtTime := pgSearchTime + vectorTime
	t.Logf("Total extension creation time: %v", totalExtTime)
	assert.Less(t, totalExtTime, 30*time.Second, "extension creation should be <30s")
}

// TestBenchBM25IndexCreationTiming measures the BM25 index creation time,
// which is typically the slowest single operation in migrations.
func TestBenchBM25IndexCreationTiming(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Run all prereq migrations
	prereqs := []string{migration001, migration002, migration003}
	for _, m := range prereqs {
		_, err := db.Pool.Exec(ctx, m)
		require.NoError(t, err)
	}

	// Seed some data to make the BM25 index meaningful
	seedBenchObservations(db.Pool, 50)

	// Create the BM25 index
	createBM25SQL := `
		CREATE INDEX IF NOT EXISTS idx_observations_bm25 ON observations
		USING bm25 (id, title, narrative, facts)
		WITH (key_field='id')
	`

	start := time.Now()
	_, err := db.Pool.Exec(ctx, createBM25SQL)
	require.NoError(t, err)
	bm25Time := time.Since(start)
	t.Logf("BM25 index creation (50 observations): %v", bm25Time)
	assert.Less(t, bm25Time, 15*time.Second, "BM25 index creation should be <15s")

	// Also test the function creation from migration 007
	funcSQL := `
		CREATE OR REPLACE FUNCTION bm25_search(query_text text, result_limit int)
		RETURNS TABLE(id text, bm25_score float8) AS $$
		BEGIN
		    RETURN QUERY
		    SELECT observations.id, paradedb.score(observations.id)::float8
		    FROM observations
		    WHERE observations @@@ paradedb.parse(query_text)
		    ORDER BY paradedb.score(observations.id) DESC
		    LIMIT result_limit;
		END;
		$$ LANGUAGE plpgsql STABLE;
	`

	start = time.Now()
	_, err = db.Pool.Exec(ctx, funcSQL)
	require.NoError(t, err)
	funcTime := time.Since(start)
	t.Logf("bm25_search function creation: %v", funcTime)
	assert.Less(t, funcTime, 5*time.Second, "function creation should be <5s")
}
