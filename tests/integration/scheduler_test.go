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

// TestSessionEndPipelineWithScheduler tests the full session-end pipeline:
// HandleSessionEnd -> CompressSessionNow -> SummarizeSessionNow (isFull=true).
// Verifies compressed_observations and session_summary (with is_full=true) are created.
func TestSessionEndPipelineWithScheduler(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "scheduler-test@example.com", "hash", "Scheduler Test User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Record observations
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	obsSvc := service.NewObservationService(db.Pool)

	for i := 0; i < 3; i++ {
		_, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
			SessionID:   sessionID,
			OwnerUserID: userID,
			Type:        service.HookUserPromptSubmit,
			Title:       "Test observation " + formatTestInt(i),
			Narrative:   "Test narrative for observation " + formatTestInt(i),
			Importance:  ptrFloat64(0.5),
		})
		require.NoError(t, err)
	}

	// Set up scheduler and session end handler
	sessionSvc := service.NewSessionService(db.Pool)
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{})
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	mode.OwnerUserID = userID
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, &sync.WaitGroup{}, semaphore.NewWeighted(20))

	// End the session — triggers compress + summarize via scheduler
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	// Verify session is ended
	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)

	// Verify compressed_observations exist (created by CompressSessionNow)
	var compressedObs []store.CompressedObservation
	for i := 0; i < 8; i++ {
		time.Sleep(1 * time.Second)
		compressedObs, err = queries.ListCompressedBySession(ctx, sessionID)
		if err == nil && len(compressedObs) > 0 {
			break
		}
	}
	require.NotEmpty(t, compressedObs, "compressed observations should have been created by CompressSessionNow")
	assert.NotEmpty(t, compressedObs[0].CompressedText)
	assert.Contains(t, compressedObs[0].CompressedText, "Compressed")

	// Verify session_summary exists with is_full=true
	var sessionSummary *store.SessionSummary
	for i := 0; i < 8; i++ {
		time.Sleep(1 * time.Second)
		summary, err := queries.GetSessionSummary(ctx, sessionID)
		if err == nil {
			sessionSummary = &summary
			break
		}
	}
	require.NotNil(t, sessionSummary, "session summary should have been created by SummarizeSessionNow")
	assert.NotEmpty(t, sessionSummary.SummaryText)
	assert.True(t, sessionSummary.IsFull, "session-end summary should have is_full=true")
	assert.Contains(t, sessionSummary.SummaryText, "Session summary")
}

// TestSessionEndPipelineWithoutObservations tests that ending a session with no
// observations completes gracefully (compression skips, summarization skips).
func TestSessionEndPipelineWithoutObservations(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "scheduler-no-obs@example.com", "hash", "No Obs Scheduler User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Set up without observations
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	sessionSvc := service.NewSessionService(db.Pool)
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{})
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, &sync.WaitGroup{}, semaphore.NewWeighted(20))

	// End session — should handle empty observations gracefully
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	// Verify session is ended
	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)
}
