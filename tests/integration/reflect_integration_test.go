package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReflectIntegration_UpsertAndReinforce verifies that upserting an insight
// with an existing fingerprint ID:
//   - boosts confidence by 10% of remaining distance (LEAST(1.0, c + (1-c)*0.10))
//   - increments reinforcement_count
//   - sets deleted=false on conflict
func TestReflectIntegration_UpsertAndReinforce(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)
	insightID := uuid.New().String()

	// Insert new insight with confidence=0.5
	err := queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insightID,
		Title:                "Connection Pool Pattern",
		Content:              "Connection pool timeouts occur when max connections is below 25.",
		Confidence:           0.5,
		SourceConceptCluster: []string{"database", "performance"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Verify initial state
	var confidence float64
	var reinfCount int32
	var deleted bool
	err = db.Pool.QueryRow(ctx,
		"SELECT confidence, reinforcement_count, deleted FROM insights WHERE id = $1", insightID,
	).Scan(&confidence, &reinfCount, &deleted)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, confidence, 1e-6, "initial confidence should be preserved")
	assert.Equal(t, int32(0), reinfCount, "initial reinforcement_count should be 0")
	assert.False(t, deleted, "initial deleted should be false")

	// Upsert with the same fingerprint ID — simulated re-discovery of the same insight
	err = queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insightID,
		Title:                "Connection Pool Pattern",
		Content:              "Connection pool timeouts occur when max connections is below 25.",
		Confidence:           0.3, // initial confidence from new cluster — ignored on CONFLICT
		SourceConceptCluster: []string{"database", "performance"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Verify reinforcement: confidence boosted by 10% of remaining distance
	// Original confidence = 0.5, boost = (1.0 - 0.5) * 0.10 = 0.05, new = 0.55
	err = db.Pool.QueryRow(ctx,
		"SELECT confidence, reinforcement_count, deleted FROM insights WHERE id = $1", insightID,
	).Scan(&confidence, &reinfCount, &deleted)
	require.NoError(t, err)
	assert.InDelta(t, 0.55, confidence, 1e-6,
		"confidence should be boosted by 10%% of remaining distance from existing value")
	assert.Equal(t, int32(1), reinfCount, "reinforcement_count should be incremented by 1")
	assert.False(t, deleted, "upsert ON CONFLICT should set deleted=false")
}

// TestReflectIntegration_ListAndSearchInsights verifies that ListInsights and
// SearchInsights correctly filter by project, min_confidence, and full-text keyword.
func TestReflectIntegration_ListAndSearchInsights(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Insert 3 insights with different confidences and projects
	projectA := "proj-alpha"
	projectB := "proj-beta"

	// Insight A: high confidence, project A, mentions "connections"
	err := queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   uuid.New().String(),
		Title:                "Connection Pool Limits",
		Content:              "Connection pools should have a max of 25 connections for optimal performance.",
		Confidence:           0.9,
		SourceConceptCluster: []string{"database"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              &projectA,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Insight B: medium confidence, project B, mentions "indexes"
	err = queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   uuid.New().String(),
		Title:                "Database Indexing Strategy",
		Content:              "Indexes on frequently queried columns improve read performance significantly.",
		Confidence:           0.7,
		SourceConceptCluster: []string{"database"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              &projectB,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Insight C: low confidence, project A, mentions "retry"
	err = queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   uuid.New().String(),
		Title:                "Network Retry Logic",
		Content:              "Retry with exponential backoff for transient network failures.",
		Confidence:           0.3,
		SourceConceptCluster: []string{"networking"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              &projectA,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Test ListInsights filters by project
	alphaInsights, err := queries.ListInsights(ctx, store.ListInsightsParams{
		Limit:         10,
		Project:       &projectA,
		MinConfidence: nil,
	})
	require.NoError(t, err)
	require.Len(t, alphaInsights, 2, "ListInsights should return 2 insights for projectA")
	assert.Equal(t, "Connection Pool Limits", alphaInsights[0].Title,
		"results should be ordered by confidence DESC")
	assert.Equal(t, "Network Retry Logic", alphaInsights[1].Title)

	// Test ListInsights filters by min_confidence (>= 0.5)
	minConf := 0.5
	highConfidence, err := queries.ListInsights(ctx, store.ListInsightsParams{
		Limit:         10,
		Project:       nil,
		MinConfidence: &minConf,
	})
	require.NoError(t, err)
	require.Len(t, highConfidence, 2, "ListInsights should return 2 insights with confidence >= 0.5")
	assert.Equal(t, "Connection Pool Limits", highConfidence[0].Title)
	assert.Equal(t, "Database Indexing Strategy", highConfidence[1].Title)
	assert.GreaterOrEqual(t, highConfidence[0].Confidence, 0.5)
	assert.GreaterOrEqual(t, highConfidence[1].Confidence, 0.5)

	// Test SearchInsights finds by keyword
	queryStr := "connections"
	searchResults, err := queries.SearchInsights(ctx, store.SearchInsightsParams{
		Limit:         10,
		Project:       nil,
		MinConfidence: nil,
		Query:         &queryStr,
	})
	require.NoError(t, err)
	require.Len(t, searchResults, 1, "SearchInsights should find 1 insight matching 'connections'")
	assert.Contains(t, searchResults[0].Title, "Connection Pool")

	// Test SearchInsights respects NULL query guard — returns all non-deleted insights
	allResults, err := queries.SearchInsights(ctx, store.SearchInsightsParams{
		Limit:         10,
		Project:       nil,
		MinConfidence: nil,
		Query:         nil,
	})
	require.NoError(t, err)
	require.Len(t, allResults, 3, "NULL query should return all 3 insights (query guard in SQL)")
}

// TestReflectIntegration_DecayAndSoftDelete verifies the ApplyDecayWithCounts
// SQL business logic: confidence decay computation (GREATEST(0.05, c - rate*weeks))
// and soft-delete threshold (confidence <= 0.1 AND reinforcement_count = 0).
func TestReflectIntegration_DecayAndSoftDelete(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)
	insight1ID := uuid.New().String()
	insight2ID := uuid.New().String()

	// Insert insight1 with confidence=0.2, reinforcement_count=0
	// decay_rate defaults to 0.05 in the schema
	err := queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insight1ID,
		Title:                "Low Confidence Insight",
		Content:              "This insight has low confidence and will be decayed.",
		Confidence:           0.2,
		SourceConceptCluster: []string{"test"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Apply decay with weeksSince=3: decay = 0.05 * 3 = 0.15, result = 0.2 - 0.15 = 0.05
	result, err := queries.ApplyDecayWithCounts(ctx, 3.0)
	require.NoError(t, err)

	// insight1 meets soft-delete condition: pre-update 0.2 - 0.05*3 = 0.05 <= 0.1 AND reinforcement_count=0
	assert.Equal(t, int32(1), result.SoftDeletedCount, "insight1 should be soft-deleted")

	// Verify insight1 confidence floored at 0.05
	var confidence float64
	var deleted bool
	err = db.Pool.QueryRow(ctx,
		"SELECT confidence, deleted FROM insights WHERE id = $1", insight1ID,
	).Scan(&confidence, &deleted)
	require.NoError(t, err)
	assert.InDelta(t, 0.05, confidence, 1e-6,
		"confidence should be floored at GREATEST(0.05, 0.2 - 0.15) = 0.05")
	assert.True(t, deleted,
		"insight with confidence <= 0.1 and 0 reinforcements should be soft-deleted")

	// Insert insight2 with confidence=0.15, reinforcement_count=0
	err = queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insight2ID,
		Title:                "Another Low Confidence Insight",
		Content:              "This insight also has low confidence.",
		Confidence:           0.15,
		SourceConceptCluster: []string{"test"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Apply decay with weeksSince=2: decay = 0.05 * 2 = 0.10, result = 0.15 - 0.10 = 0.05
	// insight1 is already deleted (deleted=true), so WHERE deleted=false skips it
	result2, err := queries.ApplyDecayWithCounts(ctx, 2.0)
	require.NoError(t, err)
	assert.Equal(t, int32(1), result2.SoftDeletedCount, "insight2 should be soft-deleted")

	// Verify insight2 is soft-deleted
	err = db.Pool.QueryRow(ctx,
		"SELECT deleted FROM insights WHERE id = $1", insight2ID,
	).Scan(&deleted)
	require.NoError(t, err)
	assert.True(t, deleted, "insight2 should be soft-deleted after decay")

	// Verify insight1 is still soft-deleted (not revived by second call)
	err = db.Pool.QueryRow(ctx,
		"SELECT deleted FROM insights WHERE id = $1", insight1ID,
	).Scan(&deleted)
	require.NoError(t, err)
	assert.True(t, deleted, "insight1 should remain soft-deleted after second decay call")
}

// TestReflectIntegration_ReviveSoftDeleted verifies that upserting a soft-deleted
// insight with the same fingerprint ID restores it by setting deleted=false
// via the ON CONFLICT DO UPDATE SET deleted = false clause.
func TestReflectIntegration_ReviveSoftDeleted(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)
	insightID := uuid.New().String()

	// Insert a new insight
	err := queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insightID,
		Title:                "Temporary Insight",
		Content:              "This insight will be soft-deleted and revived.",
		Confidence:           0.8,
		SourceConceptCluster: []string{"test"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Soft-delete the insight directly (simulating decay soft-delete)
	_, err = db.Pool.Exec(ctx, "UPDATE insights SET deleted = true WHERE id = $1", insightID)
	require.NoError(t, err)

	// Verify soft-deleted
	var deleted bool
	err = db.Pool.QueryRow(ctx,
		"SELECT deleted FROM insights WHERE id = $1", insightID,
	).Scan(&deleted)
	require.NoError(t, err)
	assert.True(t, deleted, "insight should be soft-deleted before upsert")

	// Upsert with the same fingerprint ID — ON CONFLICT DO UPDATE SET deleted = false
	err = queries.UpsertInsight(ctx, store.UpsertInsightParams{
		ID:                   insightID,
		Title:                "Temporary Insight (Revised)",
		Content:              "This insight was revived.",
		Confidence:           0.6,
		SourceConceptCluster: []string{"test"},
		SourceMemoryIds:      []string{},
		SourceLessonIds:      []string{},
		Project:              nil,
		Tags:                 []string{},
	})
	require.NoError(t, err)

	// Verify revived (deleted = false)
	err = db.Pool.QueryRow(ctx,
		"SELECT deleted FROM insights WHERE id = $1", insightID,
	).Scan(&deleted)
	require.NoError(t, err)
	assert.False(t, deleted, "upsert ON CONFLICT should revive soft-deleted insight by setting deleted=false")
}
