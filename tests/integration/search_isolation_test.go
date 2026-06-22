package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSearchIsolation_CrossTenantDataLeak verifies that search functions
// do NOT leak observations across users. This is a critical security test:
// Alice's searches must only return Alice's observations, and Bob's searches
// must only return Bob's observations.
//
// This test will FAIL until bm25_search and hybrid_search are updated
// to filter by owner_user_id.
func TestSearchIsolation_CrossTenantDataLeak(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Run migrations to create tables and search functions
	require.NoError(t, RunMigrations(db.Pool), "migrations must succeed")

	queries := store.New(db.Pool)

	// ── Create two users ──────────────────────────────────────────────
	aliceID := uuid.New().String()
	bobID := uuid.New().String()

	for _, u := range []struct{ id, email, name string }{
		{aliceID, "alice-search@example.com", "Alice Search"},
		{bobID, "bob-search@example.com", "Bob Search"},
	} {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
			u.id, u.email, "$2a$12$test", u.name,
		)
		require.NoError(t, err)
	}

	// ── Create sessions for each user ─────────────────────────────────
	aliceSession := uuid.New().String()
	bobSession := uuid.New().String()

	for _, s := range []struct{ id, userID string }{
		{aliceSession, aliceID},
		{bobSession, bobID},
	} {
		_, err := db.Pool.Exec(ctx,
			`INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'ended') ON CONFLICT DO NOTHING`,
			s.id, s.userID,
		)
		require.NoError(t, err)
	}

	now := time.Now()

	// ── Alice creates observations A1 and A2 ──────────────────────────
	aliceObs := []struct {
		ID, Title, Narrative string
	}{
		{
			uuid.New().String(),
			"Alice's PostgreSQL Connection Pool Setup",
			"Alice configured the PostgreSQL connection pool with max 25 connections and min 5 connections for optimal performance in her microservice.",
		},
		{
			uuid.New().String(),
			"Alice's API Handler Refactoring",
			"Alice refactored the API handlers to use the new middleware chain with proper error handling and request validation.",
		},
	}

	for _, o := range aliceObs {
		_, err := queries.InsertObservation(ctx, store.InsertObservationParams{
			ID:          o.ID,
			SessionID:   aliceSession,
			OwnerType:   "user",
			OwnerUserID: &aliceID,
			OwnerTeamID: nil,
			Visibility:  "private",
			Type:        "tool_use",
			Title:       o.Title,
			Narrative:   o.Narrative,
			Facts:       strPtr(""),
			Concepts:    []string{"postgresql", "api"},
			Files:       []string{"main.go"},
			Importance:  0.8,
			Timestamp:   pgtype.Timestamptz{Time: now, Valid: true},
		})
		require.NoError(t, err)
	}

	// ── Bob creates observations B1 and B2 ────────────────────────────
	bobObs := []struct {
		ID, Title, Narrative string
	}{
		{
			uuid.New().String(),
			"Bob's Frontend Caching Strategy",
			"Bob implemented a Redis caching layer for the frontend to reduce database load and improve page load times by 3x.",
		},
		{
			uuid.New().String(),
			"Bob's CI/CD Pipeline Configuration",
			"Bob set up the CI/CD pipeline with GitHub Actions including linting, testing, and deployment stages for the frontend repo.",
		},
	}

	for _, o := range bobObs {
		_, err := queries.InsertObservation(ctx, store.InsertObservationParams{
			ID:          o.ID,
			SessionID:   bobSession,
			OwnerType:   "user",
			OwnerUserID: &bobID,
			OwnerTeamID: nil,
			Visibility:  "private",
			Type:        "tool_use",
			Title:       o.Title,
			Narrative:   o.Narrative,
			Facts:       strPtr(""),
			Concepts:    []string{"frontend", "caching", "ci/cd"},
			Files:       []string{"server.go"},
			Importance:  0.7,
			Timestamp:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
		})
		require.NoError(t, err)
	}

	// ── Verify total observation count (all 4 exist in DB) ─────────────
	var totalCount int
	err := db.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM observations`).Scan(&totalCount)
	require.NoError(t, err)
	assert.Equal(t, 4, totalCount, "total observations in DB should be 4 (2 Alice + 2 Bob)")

	// ── TEST 1: Alice searches → must only see A1, A2 ─────────────────
	// Create a search service for Alice with no embedding provider (BM25-only)
	aliceSearchSvc := service.NewSearchService(db.Pool, nil)

	// Search for "PostgreSQL" — should only hit Alice's A1
	aliceResults, err := aliceSearchSvc.HybridSearch(ctx, "PostgreSQL connection pool", 10, aliceID)
	require.NoError(t, err)

	aliceResultIDs := make(map[string]bool)
	for _, r := range aliceResults {
		aliceResultIDs[r.ID] = true
		t.Logf("Alice's search result: id=%s title=%s score=%.4f", r.ID, r.Title, r.CombinedScore)
	}

	// Alice must see her observation about PostgreSQL
	assert.True(t, aliceResultIDs[aliceObs[0].ID],
		"Alice should see her own PostgreSQL observation (A1)")

	// CRITICAL: Alice must NOT see Bob's observations
	assert.False(t, aliceResultIDs[bobObs[0].ID],
		"SECURITY BUG: Alice sees Bob's Frontend Caching observation (B1)")
	assert.False(t, aliceResultIDs[bobObs[1].ID],
		"SECURITY BUG: Alice sees Bob's CI/CD Pipeline observation (B2)")

	// Alice's search should not contain any observation from Bob
	for _, r := range aliceResults {
		if r.ID == bobObs[0].ID || r.ID == bobObs[1].ID {
			t.Errorf("SECURITY LEAK: Alice's search result contains Bob's observation: %s (%s)", r.ID, r.Title)
		}
	}

	// ── TEST 2: Bob searches → must only see B1, B2 ─────────────────
	bobSearchSvc := service.NewSearchService(db.Pool, nil)
	bobResults, err := bobSearchSvc.HybridSearch(ctx, "caching frontend pipeline", 10, bobID)
	require.NoError(t, err)

	bobResultIDs := make(map[string]bool)
	for _, r := range bobResults {
		bobResultIDs[r.ID] = true
		t.Logf("Bob's search result: id=%s title=%s score=%.4f", r.ID, r.Title, r.CombinedScore)
	}

	// Bob must see his own observations
	assert.True(t, bobResultIDs[bobObs[0].ID],
		"Bob should see his own Frontend Caching observation (B1)")
	assert.True(t, bobResultIDs[bobObs[1].ID],
		"Bob should see his own CI/CD Pipeline observation (B2)")

	// CRITICAL: Bob must NOT see Alice's observations
	assert.False(t, bobResultIDs[aliceObs[0].ID],
		"SECURITY BUG: Bob sees Alice's PostgreSQL observation (A1)")
	assert.False(t, bobResultIDs[aliceObs[1].ID],
		"SECURITY BUG: Bob sees Alice's API Handler observation (A2)")

	// Bob's search should not contain any observation from Alice
	for _, r := range bobResults {
		if r.ID == aliceObs[0].ID || r.ID == aliceObs[1].ID {
			t.Errorf("SECURITY LEAK: Bob's search result contains Alice's observation: %s (%s)", r.ID, r.Title)
		}
	}

	// ── TEST 3: Verify owner_user_id filtering via direct SQL ─────────
	// This tests the bm25_search function directly with Alice's userID.
	// Bob's observations should NOT appear in bm25_search results scoped to Alice.
	var bobObsInAliceSearch int
	err = db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bm25_search($1, $2, $3) s
		 WHERE s.id = ANY($4)`,
		"caching frontend", 10, aliceID, []string{bobObs[0].ID, bobObs[1].ID},
	).Scan(&bobObsInAliceSearch)
	require.NoError(t, err)
	assert.Equal(t, 0, bobObsInAliceSearch,
		"SECURITY LEAK: bm25_search with Alice's userID returned Bob's observations")
	t.Logf("bm25_search(Alice) found %d of Bob's observations (should be 0)", bobObsInAliceSearch)

	// Verify that bm25_search returns Bob's observations when scoped to Bob.
	var bobObsInBobSearch int
	err = db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM bm25_search($1, $2, $3) s
		 WHERE s.id = ANY($4)`,
		"caching frontend", 10, bobID, []string{bobObs[0].ID, bobObs[1].ID},
	).Scan(&bobObsInBobSearch)
	require.NoError(t, err)
	assert.Equal(t, 2, bobObsInBobSearch,
		"bm25_search with Bob's userID should return Bob's 2 observations")
	t.Logf("bm25_search(Bob) found %d of Bob's observations (should be 2)", bobObsInBobSearch)
}
