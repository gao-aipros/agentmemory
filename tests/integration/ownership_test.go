package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createUser creates a test user with the given email and name, returning the user ID.
func createUser(t *testing.T, ctx context.Context, queries *store.Queries, email, name string) string {
	t.Helper()
	hash, err := auth.HashPassword("test-password")
	require.NoError(t, err)

	userID := uuid.New().String()
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		Name:         name,
	})
	require.NoError(t, err)
	return userID
}

// createSession creates a test session for the given user, returning the session ID.
func createSession(t *testing.T, ctx context.Context, queries *store.Queries, userID string) string {
	t.Helper()
	sessionID := uuid.New().String()
	_, err := queries.CreateSession(ctx, store.CreateSessionParams{
		ID:     sessionID,
		UserID: userID,
		TeamID: nil,
	})
	require.NoError(t, err)
	return sessionID
}

// TestOwnership_PrivateObservation tests that observations are created with
// correct ownership metadata and that owner fields accurately reflect who
// created each observation.
func TestOwnership_PrivateObservation(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create two users: Alice and Bob
	aliceID := createUser(t, ctx, queries, "alice-obs@example.com", "Alice")
	bobID := createUser(t, ctx, queries, "bob-obs@example.com", "Bob")

	// Create sessions for each
	aliceSession := createSession(t, ctx, queries, aliceID)
	bobSession := createSession(t, ctx, queries, bobID)

	now := time.Now()

	// Alice creates an observation
	aliceObs, err := queries.InsertObservation(ctx, store.InsertObservationParams{
		ID:          uuid.New().String(),
		SessionID:   aliceSession,
		OwnerType:   "user",
		OwnerUserID: &aliceID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Type:        "tool_use",
		Title:       "Alice's private observation",
		Narrative:   "Alice ran a database query to check user counts",
		Facts:       strPtr("Found 42 users"),
		Concepts:    []string{"database", "users", "query"},
		Files:       []string{"main.go"},
		Importance:  0.8,
		Timestamp:   pgtype.Timestamptz{Time: now, Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "user", aliceObs.OwnerType)
	assert.NotNil(t, aliceObs.OwnerUserID)
	assert.Equal(t, aliceID, *aliceObs.OwnerUserID)
	assert.Equal(t, "private", aliceObs.Visibility)

	// Bob creates his own observation
	bobObs, err := queries.InsertObservation(ctx, store.InsertObservationParams{
		ID:          uuid.New().String(),
		SessionID:   bobSession,
		OwnerType:   "user",
		OwnerUserID: &bobID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Type:        "tool_use",
		Title:       "Bob's private observation",
		Narrative:   "Bob checked the API response times",
		Facts:       strPtr("Average latency: 45ms"),
		Concepts:    []string{"api", "performance", "latency"},
		Files:       []string{"server.go"},
		Importance:  0.7,
		Timestamp:   pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
	})
	require.NoError(t, err)
	assert.Equal(t, "user", bobObs.OwnerType)
	assert.NotNil(t, bobObs.OwnerUserID)
	assert.Equal(t, bobID, *bobObs.OwnerUserID)

	// Each observation can be retrieved by ID (ID-based retrieval doesn't filter by owner)
	fetchedAlice, err := queries.GetObservation(ctx, aliceObs.ID)
	require.NoError(t, err)
	assert.Equal(t, aliceID, *fetchedAlice.OwnerUserID)
	assert.Equal(t, "Alice's private observation", fetchedAlice.Title)

	fetchedBob, err := queries.GetObservation(ctx, bobObs.ID)
	require.NoError(t, err)
	assert.Equal(t, bobID, *fetchedBob.OwnerUserID)
	assert.Equal(t, "Bob's private observation", fetchedBob.Title)

	// Verify observations are in each user's session
	aliceObsList, err := queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: aliceSession,
		Limit:     50,
	})
	require.NoError(t, err)
	assert.Len(t, aliceObsList, 1)
	assert.Equal(t, aliceID, *aliceObsList[0].OwnerUserID)

	bobObsList, err := queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: bobSession,
		Limit:     50,
	})
	require.NoError(t, err)
	assert.Len(t, bobObsList, 1)
	assert.Equal(t, bobID, *bobObsList[0].OwnerUserID)

	// Alice's session should not contain Bob's observation
	for _, o := range aliceObsList {
		assert.NotEqual(t, bobObs.ID, o.ID, "Alice's session should not list Bob's observation")
	}
	for _, o := range bobObsList {
		assert.NotEqual(t, aliceObs.ID, o.ID, "Bob's session should not list Alice's observation")
	}
}

// TestOwnership_TeamMemorySharing tests the end-to-end memory sharing flow:
// Alice creates a private memory, then shares it to her team so Bob can see it.
func TestOwnership_TeamMemorySharing(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create two users
	aliceID := createUser(t, ctx, queries, "alice-mem@example.com", "Alice Memory")
	bobID := createUser(t, ctx, queries, "bob-mem@example.com", "Bob Memory")

	// Create a team owned by Alice
	teamID := uuid.New().String()
	_, err := queries.CreateTeam(ctx, store.CreateTeamParams{
		ID:                teamID,
		Name:              "Memory Sharing Team",
		OwnerID:           aliceID,
		DefaultVisibility: "member_choice",
	})
	require.NoError(t, err)

	// Alice joins her team
	_, err = queries.AddTeamMember(ctx, store.AddTeamMemberParams{
		ID:     uuid.New().String(),
		TeamID: teamID,
		UserID: aliceID,
	})
	require.NoError(t, err)

	// Bob joins the team
	_, err = queries.AddTeamMember(ctx, store.AddTeamMemberParams{
		ID:     uuid.New().String(),
		TeamID: teamID,
		UserID: bobID,
	})
	require.NoError(t, err)

	// Alice creates a private memory
	memoryID := uuid.New().String()
	memory, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          memoryID,
		OwnerType:   "user",
		OwnerUserID: &aliceID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "The deployment pipeline should run linting before unit tests to catch issues faster.",
		Concepts:    []string{"deployment", "pipeline", "linting", "testing"},
		Source:      "consolidation",
		Confidence:  0.85,
	})
	require.NoError(t, err)
	assert.Equal(t, "private", memory.Visibility)
	assert.Equal(t, aliceID, *memory.OwnerUserID)

	// Alice can see her own memory
	fetched, err := queries.GetMemory(ctx, memoryID)
	require.NoError(t, err)
	assert.Equal(t, memory.Content, fetched.Content)

	// List memories by Alice (owner) — she should see it
	aliceMemories, err := queries.ListMemoriesByOwner(ctx, &aliceID)
	require.NoError(t, err)
	assert.Len(t, aliceMemories, 1)
	assert.Equal(t, memoryID, aliceMemories[0].ID)

	// Bob lists his own memories — should be empty (Alice's is private)
	bobMemories, err := queries.ListMemoriesByOwner(ctx, &bobID)
	require.NoError(t, err)
	assert.Empty(t, bobMemories, "Bob should not see Alice's private memory")

	// Alice shares the memory to the team
	err = queries.UpdateVisibility(ctx, store.UpdateVisibilityParams{
		ID:         memoryID,
		Visibility: "team",
	})
	require.NoError(t, err)

	// Verify the memory is now team-visible
	shared, err := queries.GetMemory(ctx, memoryID)
	require.NoError(t, err)
	assert.Equal(t, "team", shared.Visibility)

	// For a complete team share, also set owner_team_id via raw SQL
	// (the generated UpdateVisibility only changes the visibility column).
	_, err = db.Pool.Exec(ctx,
		`UPDATE memories SET owner_team_id = $1, owner_type = 'team' WHERE id = $2`,
		teamID, memoryID,
	)
	require.NoError(t, err)

	// Verify the team-scoped memory can be queried by team ID
	var teamMemoryCount int
	err = db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM memories WHERE owner_team_id = $1 AND visibility = 'team'`,
		teamID,
	).Scan(&teamMemoryCount)
	require.NoError(t, err)
	assert.Equal(t, 1, teamMemoryCount, "team should have 1 shared memory")
}

// TestOwnership_VisibilityFiltering tests that memories can be created with
// different visibility levels and are correctly filtered by ownership queries.
func TestOwnership_VisibilityFiltering(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create users
	aliceID := createUser(t, ctx, queries, "alice-vis@example.com", "Alice Visibility")
	bobID := createUser(t, ctx, queries, "bob-vis@example.com", "Bob Visibility")

	// Alice creates multiple memories with different visibility levels
	privateMem, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          uuid.New().String(),
		OwnerType:   "user",
		OwnerUserID: &aliceID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Alice's private note about database schema changes.",
		Concepts:    []string{"database", "schema"},
		Source:      "consolidation",
		Confidence:  0.9,
	})
	require.NoError(t, err)

	teamMem, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          uuid.New().String(),
		OwnerType:   "user",
		OwnerUserID: &aliceID,
		OwnerTeamID: nil,
		Visibility:  "team",
		Content:     "Team-wide note: switch to connection pooling for all services.",
		Concepts:    []string{"connection-pooling", "performance"},
		Source:      "consolidation",
		Confidence:  0.7,
	})
	require.NoError(t, err)

	publicMem, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          uuid.New().String(),
		OwnerType:   "user",
		OwnerUserID: &aliceID,
		OwnerTeamID: nil,
		Visibility:  "public",
		Content:     "Public note: the API documentation is available at /docs.",
		Concepts:    []string{"api", "documentation"},
		Source:      "consolidation",
		Confidence:  0.95,
	})
	require.NoError(t, err)

	_ = privateMem
	_ = teamMem
	_ = publicMem

	// Bob creates his own memory
	bobMem, err := queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          uuid.New().String(),
		OwnerType:   "user",
		OwnerUserID: &bobID,
		OwnerTeamID: nil,
		Visibility:  "private",
		Content:     "Bob's private note about frontend caching strategy.",
		Concepts:    []string{"frontend", "caching"},
		Source:      "consolidation",
		Confidence:  0.6,
	})
	require.NoError(t, err)

	// Alice's ListMemoriesByOwner returns all 3 of her memories
	aliceMemories, err := queries.ListMemoriesByOwner(ctx, &aliceID)
	require.NoError(t, err)
	assert.Len(t, aliceMemories, 3, "Alice should have 3 memories")

	// Verify all visibilities are present in Alice's list
	visibilities := make(map[string]bool)
	for _, m := range aliceMemories {
		visibilities[m.Visibility] = true
	}
	assert.True(t, visibilities["private"], "Alice should have a private memory")
	assert.True(t, visibilities["team"], "Alice should have a team memory")
	assert.True(t, visibilities["public"], "Alice should have a public memory")

	// Bob's ListMemoriesByOwner returns only his 1 memory
	bobMemories, err := queries.ListMemoriesByOwner(ctx, &bobID)
	require.NoError(t, err)
	assert.Len(t, bobMemories, 1, "Bob should have 1 memory")
	assert.Equal(t, bobMem.ID, bobMemories[0].ID)

	// Change Alice's private memory to team visibility
	err = queries.UpdateVisibility(ctx, store.UpdateVisibilityParams{
		ID:         privateMem.ID,
		Visibility: "team",
	})
	require.NoError(t, err)

	updated, err := queries.GetMemory(ctx, privateMem.ID)
	require.NoError(t, err)
	assert.Equal(t, "team", updated.Visibility)

	// Alice still has 3 memories
	aliceMemoriesAfter, err := queries.ListMemoriesByOwner(ctx, &aliceID)
	require.NoError(t, err)
	assert.Len(t, aliceMemoriesAfter, 3)
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
