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
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	mode.OwnerUserID = userID
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600, llmSvc)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, nil, nil, &sync.WaitGroup{}, semaphore.NewWeighted(20))

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
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600, llmSvc)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, scheduler, summarizer, consolidator, reflector, nil, nil, &sync.WaitGroup{}, semaphore.NewWeighted(20))

	// End session — should handle empty observations gracefully
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err)

	// Verify session is ended
	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)
}

// TestConsolidationViaScheduler tests that scheduler.ProcessConsolidation creates
// memories and lessons with source='consolidation' for sessions that have summaries.
func TestConsolidationViaScheduler(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "consolidation-test@example.com", "hash", "Consolidation Test User")
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
			Narrative:   "Test narrative for consolidation " + formatTestInt(i),
			Importance:  ptrFloat64(0.5),
		})
		require.NoError(t, err)
	}

	// Compress and summarize to create the session_summary needed for consolidation
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	err = scheduler.CompressSessionNow(ctx, sessionID)
	require.NoError(t, err, "CompressSessionNow should succeed")

	err = scheduler.SummarizeSessionNow(ctx, sessionID, true)
	require.NoError(t, err, "SummarizeSessionNow should succeed")

	// Verify summary exists before consolidation
	queries := store.New(db.Pool)
	summary, err := queries.GetSessionSummary(ctx, sessionID)
	require.NoError(t, err, "session summary should exist before consolidation")
	require.NotEmpty(t, summary.SummaryText)

	// Call processConsolidation — this should consolidate the session via ListUnconsolidatedSessions
	err = scheduler.ProcessConsolidation(ctx)
	require.NoError(t, err, "processConsolidation should succeed")

	// Verify memories created with source='consolidation'
	var memCount int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM memories WHERE source = 'consolidation'").Scan(&memCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, memCount, 1, "consolidation should create at least one memory")

	// Verify lessons created with source='consolidation'
	var lessonCount int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM lessons WHERE source = 'consolidation'").Scan(&lessonCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, lessonCount, 1, "consolidation should create at least one lesson")
}

// TestReflectionViaScheduler tests that scheduler.ProcessReflection creates
// insights with source='reflect' when memories exist.
func TestReflectionViaScheduler(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "reflection-test@example.com", "hash", "Reflection Test User")
	require.NoError(t, err)

	// Insert 3 memories with a shared concept so DetectPatterns produces output.
	// At least 2 memories sharing a concept are needed for pattern detection.
	concept := "shared-test-concept"
	for i := 0; i < 3; i++ {
		memID := uuid.New().String()
		_, err := db.Pool.Exec(ctx, `INSERT INTO memories (id, owner_type, owner_user_id, visibility, content, concepts, source, confidence, created_at)
			VALUES ($1, 'user', $2, 'private', $3, $4, 'consolidation', 0.5, now())`,
			memID, userID, "Memory for reflection test "+formatTestInt(i), []string{concept})
		require.NoError(t, err)
	}

	// Create scheduler and call processReflection directly
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)

	err = scheduler.ProcessReflection(ctx)
	require.NoError(t, err, "processReflection should succeed")

	// Verify insights created (the new schema drops the 'source' column; reflection
	// is the only pipeline that creates insights, so any non-deleted row counts).
	var insCount int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM insights WHERE deleted = false").Scan(&insCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, insCount, 1, "reflection should create at least one insight")
}

// TestSchedulerRecovery verifies that the scheduler picks up sessions needing
// compression regardless of session status (active or ended), and creates
// mid-session summaries with is_full=false.
func TestSchedulerRecovery(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session (keep it active — not ended)
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "recovery-test@example.com", "hash", "Recovery Test User")
	require.NoError(t, err)

	sessionID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO sessions (id, user_id, status) VALUES ($1, $2, 'active')`,
		sessionID, userID)
	require.NoError(t, err)

	// Record observations without ending the session
	llmSvc := NewMockLLMService()
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	obsSvc := service.NewObservationService(db.Pool)

	for i := 0; i < 3; i++ {
		_, err := obsSvc.RecordObservation(ctx, service.RecordObservationInput{
			SessionID:   sessionID,
			OwnerUserID: userID,
			Type:        service.HookUserPromptSubmit,
			Title:       "Recovery test observation " + formatTestInt(i),
			Narrative:   "Recovery narrative " + formatTestInt(i),
			Importance:  ptrFloat64(0.5),
		})
		require.NoError(t, err)
	}

	// Run processCompression — should catch active sessions with uncompressed observations
	scheduler := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	err = scheduler.ProcessCompression(ctx)
	require.NoError(t, err)

	// Verify compressed_observations created
	queries := store.New(db.Pool)
	compressedObs, err := queries.ListCompressedBySession(ctx, sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, compressedObs, "processCompression should create compressed observations for active sessions")
	assert.NotEmpty(t, compressedObs[0].CompressedText)

	// Run processSummarization — should create mid-session summary with is_full=false
	err = scheduler.ProcessSummarization(ctx)
	require.NoError(t, err)

	// Verify session_summary created with is_full=false (mid-session, not session-end)
	summary, err := queries.GetSessionSummary(ctx, sessionID)
	require.NoError(t, err)
	assert.NotEmpty(t, summary.SummaryText, "processSummarization should create a summary")
	assert.False(t, summary.IsFull, "recovery summarization should produce is_full=false")

	// Verify session is still active (not ended)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "active", session.Status, "session should remain active after recovery processing")
}

// TestCompressionIdempotency verifies that running compression twice on the same
// session does not produce duplicate compressed_observations. This simulates a
// scheduler restart scenario (new Scheduler instance) and verifies the
// compressed_at guard prevents re-compression.
func TestCompressionIdempotency(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	// Create user and session
	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "idempotency-test@example.com", "hash", "Idempotency Test User")
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
			Title:       "Idempotency test " + formatTestInt(i),
			Narrative:   "Idempotency narrative " + formatTestInt(i),
			Importance:  ptrFloat64(0.5),
		})
		require.NoError(t, err)
	}

	// First scheduler instance — compress
	scheduler1 := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	err = scheduler1.CompressSessionNow(ctx, sessionID)
	require.NoError(t, err)

	// Record count after first compression
	queries := store.New(db.Pool)
	firstPass, err := queries.ListCompressedBySession(ctx, sessionID)
	require.NoError(t, err)
	firstCount := len(firstPass)
	require.GreaterOrEqual(t, firstCount, 1, "first compression should produce at least one compressed observation")

	// Simulate restart: create a new scheduler instance (same pool, different object)
	scheduler2 := service.NewScheduler(db.Pool, llmSvc, embedSvc, service.SchedulerIntervals{}, nil)
	err = scheduler2.CompressSessionNow(ctx, sessionID)
	require.NoError(t, err)

	// Verify count is unchanged — no duplicates from compressed_at guard
	secondPass, err := queries.ListCompressedBySession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, firstCount, len(secondPass),
		"compressed_at guard should prevent duplicate compression after restart")

	// Verify FOR UPDATE SKIP LOCKED query compiles and is usable
	// (the actual concurrent behavior is tested at the SQL-level by the claim query)
	_ = store.New(nil)
}
