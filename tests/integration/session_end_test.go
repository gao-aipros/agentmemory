package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
)

// TestSessionEndTriggersPipeline tests that ending a session triggers
// the compress → summarize pipeline via the Scheduler and creates
// compressed_observations and session_summaries with is_full=true.
func TestSessionEndTriggersPipeline(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "session-end@example.com", "hash", "Session End User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Record some observations first
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	obsSvc := service.NewObservationService(db.Pool)

	// Add a few observations
	for i, obsType := range []string{
		service.HookSessionStart,
		service.HookUserPromptSubmit,
		service.HookPostToolUse,
	} {
		_, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
			SessionID:   sessionID,
			OwnerUserID: userID,
			Type:        obsType,
			Title:       "Test observation " + formatTestInt(i),
			Narrative:   "Test narrative for observation " + formatTestInt(i),
			Importance:  ptrFloat64(0.5),
		})
		require.NoError(t, err)
	}

	// Set up session end handler with Scheduler
	sessionSvc := service.NewSessionService(db.Pool)
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{})
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	mode.OwnerUserID = userID
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600, llmSvc)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, &sync.WaitGroup{}, semaphore.NewWeighted(20))

	// End the session
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	// Verify session is ended
	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)

	// Wait for async pipeline to complete (up to 8 seconds with polling)
	var sessionSummary *store.SessionSummary
	for i := 0; i < 8; i++ {
		time.Sleep(1 * time.Second)
		summary, err := queries.GetSessionSummary(ctx, sessionID)
		if err == nil {
			sessionSummary = &summary
			break
		}
	}
	require.NotNil(t, sessionSummary, "session summary should have been created")
	assert.NotEmpty(t, sessionSummary.SummaryText)
	assert.True(t, sessionSummary.IsFull, "session-end summary should have is_full=true")
	assert.Contains(t, sessionSummary.SummaryText, "Session summary")

	// Verify compressed observations were created by CompressSessionNow
	var compressedObs []store.CompressedObservation
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		compressedObs, err = queries.ListCompressedBySession(ctx, sessionID)
		if err == nil && len(compressedObs) > 0 {
			break
		}
	}
	assert.NotEmpty(t, compressedObs, "compressed observations should have been created")
	if len(compressedObs) > 0 {
		assert.NotEmpty(t, compressedObs[0].CompressedText)
	}
}

// TestSessionEndRequiresObservations tests that ending a session with no
// observations completes gracefully.
func TestSessionEndNoObservations(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "no-obs@example.com", "hash", "No Obs User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Set up session end handler without observations
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{})
	sessionSvc := service.NewSessionService(db.Pool)
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600, llmSvc)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, &sync.WaitGroup{}, semaphore.NewWeighted(20))

	// End session (should handle empty observations gracefully)
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err) // Should not error even with no observations

	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)
}

// TestListSessionsByUser_Limit verifies that ListSessionsByUser respects
// its LIMIT parameter and does not return unbounded results (#51).
func TestListSessionsByUser_Limit(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "limit-sessions@example.com", "hash", "Limit Sessions User")
	require.NoError(t, err)

	queries := store.New(db.Pool)

	// Insert 5 sessions
	for i := 0; i < 5; i++ {
		_, err := queries.CreateSession(ctx, store.CreateSessionParams{
			ID:     uuid.New().String(),
			UserID: userID,
		})
		require.NoError(t, err)
	}

	// Query with limit=3 — should return exactly 3
	sessions, err := queries.ListSessionsByUser(ctx, store.ListSessionsByUserParams{
		UserID: userID,
		Limit:  3,
	})
	require.NoError(t, err)
	assert.Len(t, sessions, 3)
}

func formatTestInt(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
