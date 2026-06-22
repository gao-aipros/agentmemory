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

// TestSessionEndTriggersPipeline tests that ending a session triggers
// the summarize → consolidate pipeline and creates session_summaries,
// memories, and lessons.
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
	embedSvc := service.NewEmbeddingServiceWithEmbedder(db.Pool, nil)
	compressor := service.NewCompressionService(db.Pool, llmSvc, embedSvc)
	obsSvc := service.NewObservationService(db.Pool, compressor)

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

	// Wait for compression to complete
	time.Sleep(2 * time.Second)

	// Set up session end handler
	sessionSvc := service.NewSessionService(db.Pool)
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	mode.OwnerUserID = userID
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, summarizer, consolidator, reflector)

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
	assert.Contains(t, sessionSummary.SummaryText, "Session summary")

	// Verify memories were extracted — poll since pipeline runs async
	var memories []store.Memory
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		memories, err = queries.ListMemoriesByOwner(ctx, &userID)
		if err == nil && len(memories) > 0 {
			break
		}
	}
	assert.NotEmpty(t, memories, "memories should have been extracted from consolidation")
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
	sessionSvc := service.NewSessionService(db.Pool)
	summarizer := service.NewSummarizationService(db.Pool, llmSvc)
	mode := service.DefaultConsolidationMode("member_choice", false)
	consolidator := service.NewConsolidationService(db.Pool, llmSvc, mode)
	reflector := service.NewReflectionService(db.Pool, 3600)
	sessionEndH := service.NewSessionEndHandler(sessionSvc, summarizer, consolidator, reflector)

	// End session (should handle empty observations gracefully)
	err = sessionEndH.HandleSessionEnd(ctx, sessionID)
	require.NoError(t, err) // Should not error even with no observations

	queries := store.New(db.Pool)
	session, err := queries.GetSession(ctx, sessionID)
	require.NoError(t, err)
	assert.Equal(t, "ended", session.Status)
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
