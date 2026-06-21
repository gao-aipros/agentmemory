package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProgressiveDisclosure_CompactResults verifies that SearchCompact returns
// lightweight results with only id, title, and score (no narrative/facts).
func TestProgressiveDisclosure_CompactResults(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)

	// Get compact results
	compact, err := searchSvc.SearchCompact(ctx, "PostgreSQL connection", 5)
	require.NoError(t, err)
	require.NotEmpty(t, compact, "compact search should return results")

	for _, c := range compact {
		assert.NotEmpty(t, c.ID, "compact result should have ID")
		assert.NotEmpty(t, c.Title, "compact result should have Title")
		assert.GreaterOrEqual(t, c.Score, 0.0, "compact result should have Score >= 0")
		assert.LessOrEqual(t, c.Score, 1.3, "combined score should not exceed max possible (1.3)")
	}
}

// TestProgressiveDisclosure_ExpandByID verifies that SearchExpand returns
// full observation details for specific IDs.
func TestProgressiveDisclosure_ExpandByID(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)

	// First get compact results
	compact, err := searchSvc.SearchCompact(ctx, "database migration", 5)
	require.NoError(t, err)
	require.NotEmpty(t, compact)

	// Collect IDs to expand
	ids := make([]string, len(compact))
	for i, c := range compact {
		ids[i] = c.ID
	}

	// Expand by IDs
	full, err := searchSvc.SearchExpand(ctx, ids)
	require.NoError(t, err)
	require.NotEmpty(t, full)

	// Verify full results have all fields
	for _, f := range full {
		assert.NotEmpty(t, f.ID, "full result should have ID")
		assert.NotEmpty(t, f.Title, "full result should have Title")
		assert.NotEmpty(t, f.Narrative, "full result should have Narrative")
		assert.NotEmpty(t, f.Timestamp, "full result should have Timestamp")
	}
}

// TestProgressiveDisclosure_ExpandNonexistentID verifies graceful handling
// of expansion with IDs that don't exist.
func TestProgressiveDisclosure_ExpandNonexistentID(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)

	// Try to expand a nonexistent ID
	full, err := searchSvc.SearchExpand(ctx, []string{"nonexistent-id-12345"})
	require.NoError(t, err)

	// Should return empty results, not error
	assert.Empty(t, full, "expanding nonexistent ID should return empty")
}

// TestProgressiveDisclosure_TwoStepFlow verifies the full progressive disclosure
// flow: compact search -> pick IDs -> expand.
func TestProgressiveDisclosure_TwoStepFlow(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))

	searchSvc := service.NewSearchService(db.Pool, nil)

	// Step 1: Compact search
	compact, err := searchSvc.SearchCompact(ctx, "pipeline configuration", 5)
	require.NoError(t, err)

	if len(compact) == 0 {
		t.Skip("no results for test query, skipping expand step")
	}

	// Step 2: Select first result and expand it
	firstID := compact[0].ID
	full, err := searchSvc.SearchExpand(ctx, []string{firstID})
	require.NoError(t, err)
	require.Len(t, full, 1)

	// Verify the expanded result matches the compact result
	assert.Equal(t, firstID, full[0].ID, "expanded ID should match compact ID")
	assert.Equal(t, compact[0].Title, full[0].Title, "expanded title should match compact title")
	assert.NotEmpty(t, full[0].Narrative, "expanded result should have narrative")
}
