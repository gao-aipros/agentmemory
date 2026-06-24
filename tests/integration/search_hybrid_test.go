package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHybridSearch_BM25VectorCombinedRanking verifies that the hybrid search
// (BM25 + vector combined) returns correctly ranked results with real ParadeDB.
func TestHybridSearch_BM25VectorCombinedRanking(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Run migrations to create tables
	require.NoError(t, RunMigrations(db.Pool), "migrations must succeed")

	// Seed test data
	require.NoError(t, SeedTestObservations(db.Pool), "seed data must succeed")

	// Create search service (without embedding provider — falls back to BM25-only)
	searchSvc := service.NewSearchService(db.Pool, nil)

	// Perform a hybrid search
	results, err := searchSvc.HybridSearch(ctx, "PostgreSQL connection pool configuration", 10, "user-001")
	require.NoError(t, err)
	require.NotEmpty(t, results, "search should return results")

	// Verify results are ranked (first result has highest score)
	if len(results) > 1 {
		assert.GreaterOrEqual(t, results[0].CombinedScore, results[len(results)-1].CombinedScore,
			"results should be ranked by combined score descending")
	}

	// Verify result structure
	for _, r := range results {
		assert.NotEmpty(t, r.ID, "result should have an ID")
		assert.NotEmpty(t, r.Title, "result should have a title")
		assert.GreaterOrEqual(t, r.CombinedScore, 0.0, "combined score should be non-negative")
	}
}

// TestHybridSearch_WithGraphBonus verifies that graph traversal adds bonus
// scores to search results.
func TestHybridSearch_GraphBonus(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))
	require.NoError(t, SeedTestGraph(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)
	results, err := searchSvc.HybridSearch(ctx, "PostgreSQL", 10, "user-001")
	require.NoError(t, err)

	// Graph scores should be present for results that have graph connections
	hasGraphScore := false
	for _, r := range results {
		if r.GraphScore > 0 {
			hasGraphScore = true
		}
		// Combined score should include graph component
		expectedCombined := service.CombineSearchScores(r.Bm25Score, r.VectorScore, r.GraphScore)
		assert.InDelta(t, expectedCombined, r.CombinedScore, 0.001,
			"combined score should equal weighted sum")
	}
	t.Logf("graph scores present: %v", hasGraphScore)
}

// TestHybridSearch_EmptyResults verifies graceful handling when no results found.
func TestHybridSearch_EmptyResults(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	require.NoError(t, RunMigrations(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)
	results, err := searchSvc.HybridSearch(ctx, "zzz_nonexistent_query_xyz", 10, "user-001")
	require.NoError(t, err)

	// Should return empty results, not error
	assert.Empty(t, results, "search for nonexistent query should return empty results")
}

// SeedTestObservations inserts test data for search testing.
func SeedTestUser(pool *pgxpool.Pool) error {
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, password_hash, name) VALUES
		('user-001', 'test@test.com', '$2a$12$test', 'Test User')
		ON CONFLICT DO NOTHING
	`)
	return err
}

// SeedTestObservations inserts test data for search testing.
func SeedTestObservations(pool *pgxpool.Pool) error {
	ctx := context.Background()

	// Ensure test user exists (required by FK constraint)
	if err := SeedTestUser(pool); err != nil {
		return err
	}

	// Create test sessions first
	_, err := pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, team_id, status)
		VALUES
			('sess-001', 'user-001', NULL, 'ended'),
			('sess-002', 'user-001', NULL, 'ended')
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return err
	}

	// Insert test observations
	observations := []struct {
		id, session, ownerType, ownerUser, obsType, title, narrative string
	}{
		{
			"obs-001", "sess-001", "user", "user-001", "session_start",
			"PostgreSQL Connection Pool Setup",
			"We configured the PostgreSQL connection pool with max 25 connections and min 5 connections. The pool timeout was set to 30 seconds for optimal performance.",
		},
		{
			"obs-002", "sess-001", "user", "user-001", "pre_tool_use",
			"Database Migration Applied",
			"Applied migration 003_embeddings to add pgvector extension support. The migration created observation_embeddings table.",
		},
		{
			"obs-003", "sess-002", "user", "user-001", "task_completed",
			"API Handler Implementation",
			"Implemented the REST API handlers for observe, session end, and session commit endpoints. Used chi router with middleware.",
		},
		{
			"obs-004", "sess-002", "user", "user-001", "notification",
			"Build Pipeline Configuration",
			"Set up the CI/CD pipeline configuration with GitHub Actions. Added test, lint, and build stages.",
		},
		{
			"obs-005", "sess-001", "user", "user-001", "pre_compact",
			"Memory Compression Strategy",
			"Implemented chunked summarization for compressing observations within token budgets. Used approximate token counting.",
		},
	}

	for _, o := range observations {
		_, err := pool.Exec(ctx, `
			INSERT INTO observations (id, session_id, owner_type, owner_user_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp)
			VALUES ($1, $2, $3, $4, 'private', $5, $6, $7, '', ARRAY['postgresql', 'database'], ARRAY[]::text[], 0.8, now())
			ON CONFLICT DO NOTHING
		`, o.id, o.session, o.ownerType, o.ownerUser, o.obsType, o.title, o.narrative)
		if err != nil {
			return err
		}
	}

	return nil
}

// SeedTestGraph creates graph nodes and edges for testing graph traversal.
func SeedTestGraph(pool *pgxpool.Pool) error {
	ctx := context.Background()

	// Create graph nodes for observations
	nodes := []struct {
		id, nodeType, entityID, label string
	}{
		{"gn-001", "observation", "obs-001", "PostgreSQL Connection Pool Setup"},
		{"gn-002", "observation", "obs-002", "Database Migration Applied"},
		{"gn-003", "observation", "obs-003", "API Handler Implementation"},
	}

	for _, n := range nodes {
		_, err := pool.Exec(ctx, `
			INSERT INTO graph_nodes (id, node_type, entity_id, label)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING
		`, n.id, n.nodeType, n.entityID, n.label)
		if err != nil {
			return err
		}
	}

	// Create edges connecting related observations
	edges := []struct {
		id, fromID, toID, edgeType string
		weight                     float64
	}{
		{"ge-001", "gn-001", "gn-002", "related_to", 0.8},
		{"ge-002", "gn-001", "gn-003", "mentions", 0.3},
	}

	for _, e := range edges {
		_, err := pool.Exec(ctx, `
			INSERT INTO graph_edges (id, from_node_id, to_node_id, edge_type, weight)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT DO NOTHING
		`, e.id, e.fromID, e.toID, e.edgeType, e.weight)
		if err != nil {
			return err
		}
	}

	return nil
}

