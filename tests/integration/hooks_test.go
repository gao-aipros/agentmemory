package integration

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllThirteenHookTypes tests that all 13 hook event types can be
// recorded as observations and stored correctly in the database.
func TestAllThirteenHookTypes(t *testing.T) {
	t.Parallel()

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "hooks@example.com", "hash", "Hook Test User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	obsSvc := service.NewObservationService(db.Pool, nil)

	// Record all 13 hook types
	allTypes := service.ValidHookTypes
	assert.Len(t, allTypes, 13, "should be exactly 13 valid hook types")

	recordedIDs := make([]string, 0, len(allTypes))
	for _, hookType := range allTypes {
		obs, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
			SessionID:   sessionID,
			OwnerUserID: userID,
			Type:        hookType,
			Title:       "Hook: " + hookType,
			Narrative:   "Test narrative for hook type: " + hookType,
			Importance:  0.5,
		})
		require.NoError(t, err, "should be able to record hook type: %s", hookType)
		require.NotNil(t, obs)
		assert.Equal(t, hookType, obs.Type)
		recordedIDs = append(recordedIDs, obs.ID)
	}
	assert.Len(t, recordedIDs, 13, "should have recorded 13 observations")

	// Verify they can be retrieved
	queries := store.New(db.Pool)
	observations, err := queries.ListObservationsBySession(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, observations, 13, "should have 13 observations in the database")

	// Verify each type is present
	typeSet := make(map[string]bool)
	for _, obs := range observations {
		typeSet[obs.Type] = true
	}
	for _, expectedType := range allTypes {
		assert.True(t, typeSet[expectedType], "expected hook type %q to be in results", expectedType)
	}
}

// TestHookTypeSpecificFields tests that each hook type can carry its own
// specific metadata in facts and concepts.
func TestHookTypeSpecificFields(t *testing.T) {
	t.Parallel()

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "hookfields@example.com", "hash", "Hook Fields User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	obsSvc := service.NewObservationService(db.Pool, nil)

	// Pre-tool-use hook with tool metadata
	preToolObs, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
		SessionID:   sessionID,
		OwnerUserID: userID,
		Type:        service.HookPreToolUse,
		Title:       "About to use Read tool",
		Narrative:   "Agent is about to read a file",
		Facts:       `{"tool": "Read", "path": "/path/to/file.go"}`,
		Concepts:    []string{"tool_use", "read", "filesystem"},
		Importance:  0.6,
	})
	require.NoError(t, err)

	// Post-commit hook with git metadata
	postCommitObs, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
		SessionID:   sessionID,
		OwnerUserID: userID,
		Type:        service.HookPostCommit,
		Title:       "Commit made",
		Narrative:   "Agent committed changes to the repository",
		Facts:       `{"sha": "abc123", "branch": "main", "message": "Add migration"}`,
		Concepts:    []string{"git", "commit", "version_control"},
		Files:       []string{"/path/to/migration.sql"},
		Importance:  0.7,
	})
	require.NoError(t, err)

	// Verify facts and concepts are stored correctly
	queries := store.New(db.Pool)

	preTool, err := queries.GetObservation(ctx, preToolObs.ID)
	require.NoError(t, err)
	assert.Equal(t, service.HookPreToolUse, preTool.Type)
	assert.NotNil(t, preTool.Facts)
	assert.Contains(t, *preTool.Facts, "Read")
	assert.Contains(t, preTool.Concepts, "tool_use")

	postCommit, err := queries.GetObservation(ctx, postCommitObs.ID)
	require.NoError(t, err)
	assert.Equal(t, service.HookPostCommit, postCommit.Type)
	assert.NotNil(t, postCommit.Facts)
	assert.Contains(t, *postCommit.Facts, "abc123")
	assert.Contains(t, postCommit.Files, "/path/to/migration.sql")
}
