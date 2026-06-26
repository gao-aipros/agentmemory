package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// T017: Save memory with source='manual_save', run Reflection, assert saved
// memory is NOT in insight input set
// ---------------------------------------------------------------------------
func TestSavedMemoryExcludedFromReflection(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create a test user to own the memories
	userID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        "test@example.com",
		PasswordHash: "hash",
		Name:         "Test User",
		TotpSecret:   nil,
		TotpEnabled:  false,
	})
	require.NoError(t, err, "CreateUser must succeed")

	manualMemoryID := uuid.New().String()
	consolidationMemoryID := uuid.New().String()

	// Insert a memory with source = 'manual_save'
	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          manualMemoryID,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Manually saved memory about project preferences.",
		Concepts:    []string{"preferences", "project"},
		Source:      "manual_save",
		Confidence:  0.8,
	})
	require.NoError(t, err, "InsertMemory (manual_save) must succeed")

	// Insert a memory with source = 'consolidation'
	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          consolidationMemoryID,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Consolidated memory about database connection patterns.",
		Concepts:    []string{"database", "connections"},
		Source:      "consolidation",
		Confidence:  0.7,
	})
	require.NoError(t, err, "InsertMemory (consolidation) must succeed")

	// HasUnreflectedMemories should return true — consolidation memory exists
	// and is unreflected. The manual_save memory should be invisible to this query.
	hasUnreflected, err := queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.True(t, hasUnreflected,
		"HasUnreflectedMemories should be true when an unreflected consolidation memory exists")

	// ListAllMemories should include the consolidation memory but NOT the manual_save memory
	memories, err := queries.ListAllMemories(ctx, 10)
	require.NoError(t, err)

	// Collect returned memory IDs for assertion
	returnedIDs := make(map[string]bool)
	for _, m := range memories {
		returnedIDs[m.ID] = true
	}

	assert.Contains(t, returnedIDs, consolidationMemoryID,
		"ListAllMemories should include the consolidation-sourced memory")
	assert.NotContains(t, returnedIDs, manualMemoryID,
		"ListAllMemories should NOT include the manual_save memory")

	// Verify Source field on returned consolidation memory
	for _, m := range memories {
		if m.ID == consolidationMemoryID {
			assert.Equal(t, "consolidation", m.Source,
				"Returned memory should have source='consolidation'")
			break
		}
	}

	// Mark the consolidation memory as reflected
	_, err = db.Pool.Exec(ctx,
		"UPDATE memories SET reflected = true WHERE id = $1", consolidationMemoryID)
	require.NoError(t, err, "marking memory as reflected must succeed")

	// HasUnreflectedMemories should now return false — all consolidation memories
	// are reflected, and manual_save memories are invisible to the query
	hasUnreflected, err = queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.False(t, hasUnreflected,
		"HasUnreflectedMemories should be false after all consolidation memories are reflected")
}

// ---------------------------------------------------------------------------
// T018: Integration test — consolidation memories are still processed
// through the reflection pipeline
// ---------------------------------------------------------------------------
func TestConsolidationMemoriesProcessedInReflection(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create a test user to own the memory
	userID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        "consolidation-test@example.com",
		PasswordHash: "hash",
		Name:         "Consolidation Test User",
		TotpSecret:   nil,
		TotpEnabled:  false,
	})
	require.NoError(t, err, "CreateUser must succeed")

	memoryID := uuid.New().String()

	// Insert a memory with source = 'consolidation'
	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          memoryID,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Consolidated memory about API rate limiting strategies.",
		Concepts:    []string{"api", "rate-limiting"},
		Source:      "consolidation",
		Confidence:  0.6,
	})
	require.NoError(t, err, "InsertMemory (consolidation) must succeed")

	// HasUnreflectedMemories must return true — the consolidation memory is
	// unreflected and should be picked up by the pipeline
	hasUnreflected, err := queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.True(t, hasUnreflected,
		"HasUnreflectedMemories should return true when a consolidation memory exists")

	// ListAllMemories must include the consolidation memory
	memories, err := queries.ListAllMemories(ctx, 10)
	require.NoError(t, err)

	found := false
	for _, m := range memories {
		if m.ID == memoryID {
			found = true
			assert.Equal(t, "consolidation", m.Source,
				"Returned memory should have source='consolidation'")
			assert.False(t, m.Reflected,
				"Newly inserted memory should not be marked as reflected")
			break
		}
	}
	assert.True(t, found,
		"ListAllMemories must include the consolidation-sourced memory")

	// Verify at least one result was returned
	require.NotEmpty(t, memories,
		"ListAllMemories should return at least one memory")

	// Mark the memory as reflected
	_, err = db.Pool.Exec(ctx,
		"UPDATE memories SET reflected = true WHERE id = $1", memoryID)
	require.NoError(t, err, "marking memory as reflected must succeed")

	// After marking reflected, HasUnreflectedMemories should return false
	hasUnreflected, err = queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.False(t, hasUnreflected,
		"HasUnreflectedMemories should return false after memory is reflected")
}

// ---------------------------------------------------------------------------
// T019: End-to-end assertion — verify ListAllMemories and
// HasUnreflectedMemories both correctly filter source='consolidation'
// ---------------------------------------------------------------------------
func TestListAllMemoriesFiltersByConsolidationSource(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create a test user to own the memories
	userID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        "filter-test@example.com",
		PasswordHash: "hash",
		Name:         "Filter Test User",
		TotpSecret:   nil,
		TotpEnabled:  false,
	})
	require.NoError(t, err, "CreateUser must succeed")

	manualMemoryID := uuid.New().String()
	consolidationMemoryID1 := uuid.New().String()
	consolidationMemoryID2 := uuid.New().String()

	// Insert manual_save memory
	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          manualMemoryID,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "User's manual note about preferred color scheme.",
		Concepts:    []string{"ui", "theme"},
		Source:      "manual_save",
		Confidence:  0.9,
	})
	require.NoError(t, err, "InsertMemory (manual_save) must succeed")

	// Insert two consolidation memories
	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          consolidationMemoryID1,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Consolidated insight about connection pooling under load.",
		Concepts:    []string{"database", "connection-pool"},
		Source:      "consolidation",
		Confidence:  0.75,
	})
	require.NoError(t, err, "InsertMemory (consolidation 1) must succeed")

	_, err = queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          consolidationMemoryID2,
		OwnerType:   "user",
		OwnerUserID: &userID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Consolidated insight about caching strategies for API responses.",
		Concepts:    []string{"caching", "api"},
		Source:      "consolidation",
		Confidence:  0.65,
	})
	require.NoError(t, err, "InsertMemory (consolidation 2) must succeed")

	// ListAllMemories should return ONLY consolidation-sourced memories
	memories, err := queries.ListAllMemories(ctx, 10)
	require.NoError(t, err)

	// Verify manual_save is excluded
	for _, m := range memories {
		assert.NotEqual(t, "manual_save", m.Source,
			"ListAllMemories should never include manual_save memories")
		assert.NotEqual(t, manualMemoryID, m.ID,
			"ListAllMemories should never include manual_save memory by ID")
	}

	// Verify both consolidation memories are included
	memoryIDs := make(map[string]string)
	for _, m := range memories {
		memoryIDs[m.ID] = m.Source
	}

	assert.Equal(t, "consolidation", memoryIDs[consolidationMemoryID1],
		"ListAllMemories should include consolidation memory 1 with correct source")
	assert.Equal(t, "consolidation", memoryIDs[consolidationMemoryID2],
		"ListAllMemories should include consolidation memory 2 with correct source")
	assert.NotContains(t, memoryIDs, manualMemoryID,
		"ListAllMemories should NOT include the manual_save memory ID")

	// All returned memories should have source = 'consolidation'
	assert.Len(t, memories, 2,
		"ListAllMemories should return exactly 2 consolidation-sourced memories (not the manual_save one)")

	// HasUnreflectedMemories — should be true because consolidation memories are unreflected
	hasUnreflected, err := queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.True(t, hasUnreflected,
		"HasUnreflectedMemories should be true when unreflected consolidation memories exist")

	// Mark all consolidation memories as reflected
	for _, id := range []string{consolidationMemoryID1, consolidationMemoryID2} {
		_, err = db.Pool.Exec(ctx,
			"UPDATE memories SET reflected = true WHERE id = $1", id)
		require.NoError(t, err, "marking memory %s as reflected must succeed", id)
	}

	// HasUnreflectedMemories — should now be false
	hasUnreflected, err = queries.HasUnreflectedMemories(ctx)
	require.NoError(t, err)
	assert.False(t, hasUnreflected,
		"HasUnreflectedMemories should be false after all consolidation memories are reflected")

	// Verify manual_save memory is still present and unreflected (not accidentally modified)
	var manualReflected bool
	err = db.Pool.QueryRow(ctx,
		"SELECT reflected FROM memories WHERE id = $1", manualMemoryID,
	).Scan(&manualReflected)
	require.NoError(t, err, "querying manual_save memory must succeed")
	assert.False(t, manualReflected,
		"manual_save memory should still be unreflected (not modified by reflection pipeline)")
}
