package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T009: Save a memory via memory_save, then search via memory_recall,
// verify result appears with source 'manual_save'.
//
// RED PHASE: This test WILL FAIL because SearchService.HybridSearch (called by
// RecallService.Recall) only queries the observations table. Manually saved
// memories are stored in the memories table but are not yet included in search
// indexes. When memory search is implemented, this test should pass.
// =============================================================================
func TestMemorySaveAppearsInRecall(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	// Create test user (required by FK constraint)
	require.NoError(t, SeedTestUser(db.Pool))

	queries := store.New(db.Pool)
	ownerUserID := "user-001"

	// Insert a memory mimicking what memory_save MCP handler does
	memory, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          "mem-red-001",
		OwnerType:   "user",
		OwnerUserID: &ownerUserID,
		Visibility:  "private",
		Content:     "This is a manually saved memory about PostgreSQL connection pool tuning",
		Concepts:    []string{"postgresql", "connection-pool"},
		Source:      "manual_save",
		Confidence:  0.9,
	})
	require.NoError(t, err)
	require.Equal(t, "manual_save", memory.Source, "memory must be saved with source='manual_save'")

	// Search via memory_recall (RecallService.Recall -> SearchService.HybridSearch)
	recallSvc := service.NewRecallService(db.Pool, nil)
	result, err := recallSvc.Recall(ctx, "PostgreSQL connection pool", 10, "compact", "user-001")
	require.NoError(t, err)
	require.NotNil(t, result)

	// THIS ASSERTION FAILS: the saved memory is not indexed by the observations-only search.
	// Once the implementation adds memory queries to HybridSearch, this will pass.
	assert.NotEmpty(t, result.Compact,
		"saved memory should appear in recall results (memory now indexed and searchable)")

	found := false
	for _, c := range result.Compact {
		if c.ID == "mem-red-001" {
			found = true
			break
		}
	}
	assert.True(t, found,
		"saved memory mem-red-001 should appear in recall results with source='manual_save'")
}

// =============================================================================
// T010: Save a memory, delete it, verify it's gone from search results.
//
// RED PHASE: The initial search (step 2) FAILS because the memory was never
// indexed for search. The test never reaches the delete-and-verify-gone steps.
// When memory indexing is implemented, step 2 passes, the memory is deleted,
// and step 4 confirms removal from search.
// =============================================================================
func TestMemorySaveThenDeleteIsRemovedFromSearch(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestUser(db.Pool))

	queries := store.New(db.Pool)
	ownerUserID := "user-001"

	// 1. Save a memory
	_, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          "mem-del-001",
		OwnerType:   "user",
		OwnerUserID: &ownerUserID,
		Visibility:  "private",
		Content:     "Memory about to be deleted and verified gone from search",
		Source:      "manual_save",
		Confidence:  0.9,
	})
	require.NoError(t, err)

	// 2. Search — verify present before deletion
	// THIS FAILS: memory not indexed by search
	searchSvc := service.NewSearchService(db.Pool, nil)
	results, err := searchSvc.HybridSearch(ctx, "Memory about to be deleted", 10, "user-001")
	require.NoError(t, err)
	assert.NotEmpty(t, results,
		"saved memory should be findable before deletion (memory now indexed and searchable)")

	// 3. Delete the memory
	require.NoError(t, queries.DeleteMemory(ctx, "mem-del-001"))

	// 4. Search — verify gone after deletion
	resultsAfter, err := searchSvc.HybridSearch(ctx, "Memory about to be deleted", 10, "user-001")
	require.NoError(t, err)
	assert.Empty(t, resultsAfter,
		"deleted memory should not appear in search results")
}

// =============================================================================
// T011: Mixed observation + memory results with source disambiguation.
//
// Verifies that when both observations and memories match a query, both appear
// in results with correct source attribution.
//
// RED PHASE: The observation (obs-001) appears because observations ARE indexed
// for search. The memory (mem-mixed-001) does NOT appear because memories are
// not yet indexed. The assertion assert.True(t, foundMem) FAILS.
// =============================================================================
func TestMixedObservationAndMemorySearchResults(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	// Seed observations (also creates user + sessions)
	require.NoError(t, SeedTestObservations(db.Pool))

	queries := store.New(db.Pool)
	ownerUserID := "user-001"

	// Insert a memory with similar content to the observation obs-001
	// ("PostgreSQL Connection Pool Setup")
	_, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          "mem-mixed-001",
		OwnerType:   "user",
		OwnerUserID: &ownerUserID,
		Visibility:  "private",
		Content:     "PostgreSQL Connection Pool Setup and tuning for production workloads",
		Source:      "manual_save",
		Confidence:  0.9,
	})
	require.NoError(t, err)

	// Search for content that matches both the observation and the memory
	searchSvc := service.NewSearchService(db.Pool, nil)
	results, err := searchSvc.HybridSearch(ctx, "PostgreSQL connection pool", 20, "user-001")
	require.NoError(t, err)

	// Observations should be found
	assert.NotEmpty(t, results, "search should return results")

	foundObs := false
	foundMem := false
	for _, r := range results {
		if r.ID == "obs-001" {
			foundObs = true
		}
		if r.ID == "mem-mixed-001" {
			foundMem = true
		}
	}
	assert.True(t, foundObs, "observation obs-001 should appear in search results")
	// THIS FAILS: memory not indexed by search
	assert.True(t, foundMem,
		"memory mem-mixed-001 should appear in search results alongside observations "+
			"(memory now indexed and searchable)")
}
