package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPipelineObserveToCompressed tests the full pipeline: observe → compress → searchable.
// Uses testcontainers ParadeDB with a mock LLM provider.
func TestPipelineObserveToCompressed(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()

	// Run migrations
	runMigrations(t, db)

	// Create a test user
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "test@example.com", "hash", "Test User")
	require.NoError(t, err)

	// Create a test session
	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Set up services with mock LLM
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil) // No embed provider for test
	compressor := service.NewCompressionService(db.Pool, llmSvc, embedSvc)
	obsSvc := service.NewObservationService(db.Pool)

	// Step 1: Observe — record an observation
	input := service.RecordObservationInput{
		SessionID:   sessionID,
		OwnerType:   "user",
		OwnerUserID: userID,
		Type:        service.HookUserPromptSubmit,
		Title:       "User asked about database schema",
		Narrative:   "The user wanted to understand the PostgreSQL schema structure for the memory platform.",
		Facts:       "Database uses PostgreSQL with pgvector extension",
		Concepts:    []string{"postgresql", "schema", "pgvector"},
		Importance:  ptrFloat64(0.8),
	}

	obs, err := obsSvc.RecordObservation(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, obs)
	assert.Equal(t, service.HookUserPromptSubmit, obs.Type)
	assert.Equal(t, "User asked about database schema", obs.Title)

	// Step 2: Verify observation was stored
	queries := store.New(db.Pool)
	storedObs, err := queries.GetObservation(ctx, obs.ID)
	require.NoError(t, err)
	assert.Equal(t, obs.ID, storedObs.ID)
	assert.Equal(t, sessionID, storedObs.SessionID)

	// Step 3: Wait for async compression to complete
	time.Sleep(2 * time.Second)

	// Step 4: Verify compressed observation was created
	compressedObs, err := queries.ListCompressedBySession(ctx, sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, compressedObs, "compressed observation should have been created")
	if len(compressedObs) > 0 {
		assert.NotEmpty(t, compressedObs[0].CompressedText)
		assert.Contains(t, compressedObs[0].CompressedText, "Compressed")
	}

	// Step 5: List observations by session
	observations, err := queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: sessionID,
		Limit:     50,
	})
	require.NoError(t, err)
	assert.Len(t, observations, 1)
}

// TestPipelineObserveTypeValidation tests that invalid hook types are rejected.
func TestPipelineObserveTypeValidation(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "test2@example.com", "hash", "Test User 2")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	obsSvc := service.NewObservationService(db.Pool)

	// Try invalid type
	_, err = obsSvc.RecordObservation(ctx, service.RecordObservationInput{
		SessionID:   sessionID,
		OwnerUserID: userID,
		Type:        "invalid_type",
		Title:       "Test",
		Narrative:   "Test",
		Importance:  ptrFloat64(0.5),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid hook type")
}

// TestPipelineImportanceValidation tests that importance outside [0.0, 1.0] is rejected.
func TestPipelineImportanceValidation(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "test3@example.com", "hash", "Test User 3")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	obsSvc := service.NewObservationService(db.Pool)

	// Try invalid importance
	_, err = obsSvc.RecordObservation(ctx, service.RecordObservationInput{
		SessionID:   sessionID,
		OwnerUserID: userID,
		Type:        service.HookNotification,
		Title:       "Test",
		Narrative:   "Test",
		Importance:  ptrFloat64(1.5),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "importance must be between")
}

// TestListObservationsBySession_Limit verifies that ListObservationsBySession
// respects its LIMIT parameter and does not return unbounded results (#52).
func TestListObservationsBySession_Limit(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "limit-obs@example.com", "hash", "Limit Obs User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	queries := store.New(db.Pool)

	// Insert 5 observations
	for i := 0; i < 5; i++ {
		obsID := uuid.New().String()
		_, err := db.Pool.Exec(ctx, `INSERT INTO observations (id, session_id, owner_type, owner_user_id, visibility, type, title, narrative, facts, concepts, files, importance, timestamp)
			VALUES ($1, $2, 'user', $3, 'private', 'note', $4, 'narrative', '{}', '{}', '{}', 0.5, now())`,
			obsID, sessionID, userID, "observation-"+obsID[:8])
		require.NoError(t, err)
	}

	// Query with limit=3 — should return exactly 3
	observations, err := queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: sessionID,
		Limit:     3,
	})
	require.NoError(t, err)
	assert.Len(t, observations, 3)
}
