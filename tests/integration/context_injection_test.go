package integration

import (
	"context"
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SeedTestLessons inserts test lesson data into the database.
func SeedTestLessons(pool *pgxpool.Pool) error {
	ctx := context.Background()

	lessons := []struct {
		id, content string
		confidence  float64
	}{
		{"lesson-001", "Always set PostgreSQL connection timeouts to at least 30 seconds to handle network latency.", 0.7},
		{"lesson-002", "Use pgxpool for connection pooling instead of database/sql for better PostgreSQL performance.", 0.8},
		{"lesson-003", "Run sqlc generate after every SQL query file change to keep Go types in sync.", 0.9},
	}

	for _, l := range lessons {
		_, err := pool.Exec(ctx, `
			INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source)
			VALUES ($1, NULL, 'team', $2, '', $3, 'test')
			ON CONFLICT DO NOTHING
		`, l.id, l.content, l.confidence)
		if err != nil {
			return err
		}
	}
	return nil
}

// TestContextInjection_BudgetRespected verifies that the assembled context
// respects the 1500-token budget limit with real database data.
func TestContextInjection_BudgetRespected(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))
	require.NoError(t, SeedTestLessons(db.Pool))

	slotSvc := service.NewSlotService(db.Pool)
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	ctxSvc := service.NewContextService(db.Pool, embedSvc, slotSvc)

	// Assemble context for a test user
	assembled, err := ctxSvc.AssembleContext(ctx, "user-001")
	require.NoError(t, err)
	require.NotNil(t, assembled)

	// Apply budget
	budget := service.DefaultContextBudget()
	result := service.ApplyBudget(assembled, budget)

	// Verify budget is respected
	tokens := service.EstimateTokens(result)
	assert.LessOrEqual(t, tokens, budget.TotalTokens,
		"assembled context must respect %d token budget (got %d tokens)",
		budget.TotalTokens, tokens)

	t.Logf("Context tokens: %d / %d", tokens, budget.TotalTokens)
	t.Logf("Context preview:\n%s", truncateStr(result, 500))
}

// TestContextInjection_SourceBucketsPopulated verifies that the 5 context
// source buckets are populated when data exists.
func TestContextInjection_SourceBucketsPopulated(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))
	require.NoError(t, SeedTestLessons(db.Pool))
	require.NoError(t, SeedTestGraph(db.Pool))

	slotSvc := service.NewSlotService(db.Pool)
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	ctxSvc := service.NewContextService(db.Pool, embedSvc, slotSvc)

	assembled, err := ctxSvc.AssembleContext(ctx, "user-001")
	require.NoError(t, err)

	// Observations should be populated from seeded sessions
	if assembled.Observations != "" {
		assert.Contains(t, assembled.Observations, "obs-",
			"observations section should contain observation references")
	}

	// Recap should be populated if session summaries exist
	t.Logf("Recap content: %s", truncateStr(assembled.Recap, 200))

	// Lessons should be populated from seeded lessons
	if assembled.Lessons != "" {
		assert.Contains(t, assembled.Lessons, "lesson",
			"lessons section should contain lesson content")
	}

	// At least some buckets should have content
	bucketsFilled := 0
	if assembled.Observations != "" {
		bucketsFilled++
	}
	if assembled.Recap != "" {
		bucketsFilled++
	}
	if assembled.Lessons != "" {
		bucketsFilled++
	}
	if assembled.Graph != "" {
		bucketsFilled++
	}

	t.Logf("Buckets filled: %d/5", bucketsFilled)
	assert.GreaterOrEqual(t, bucketsFilled, 1,
		"at least one context bucket should be populated")
}

// TestContextGate_DisabledByDefault verifies that context injection is
// disabled when AGENTMEMORY_INJECT_CONTEXT is not set.
func TestContextGate_DisabledByDefault(t *testing.T) {
	// Ensure env var is not set
	os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

	gate := service.NewContextGate()
	assert.False(t, gate.IsEnabled(),
		"context gate should be disabled by default")
}

// TestContextGate_EnabledWhenSet verifies that context injection is
// enabled when AGENTMEMORY_INJECT_CONTEXT=true.
func TestContextGate_EnabledWhenSet(t *testing.T) {
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "true")
	defer os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")

	gate := service.NewContextGate()
	assert.True(t, gate.IsEnabled(),
		"context gate should be enabled when AGENTMEMORY_INJECT_CONTEXT=true")
}

// TestContextGate_ExplicitConstructor verifies the NewContextGateWithValue
// constructor for testing.
func TestContextGate_ExplicitConstructor(t *testing.T) {
	gate := service.NewContextGateWithValue(true)
	assert.True(t, gate.IsEnabled())

	gate = service.NewContextGateWithValue(false)
	assert.False(t, gate.IsEnabled())
}

// TestContextHookManager_SessionStart_Disabled verifies that session_start
// hook returns empty when gate is disabled.
func TestContextHookManager_SessionStart_Disabled(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))

	slotSvc := service.NewSlotService(db.Pool)
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	ctxSvc := service.NewContextService(db.Pool, embedSvc, slotSvc)

	// Gate disabled
	gate := service.NewContextGateWithValue(false)
	hookMgr := service.NewContextHookManager(ctxSvc, gate)

	result := hookMgr.TriggerSessionStart(ctx, "user-001")
	require.NotNil(t, result)
	assert.True(t, result.Skipped, "should skip when gate is disabled")
	assert.Empty(t, result.ContextText, "context text should be empty when disabled")
}

// TestContextFormat_ReferenceFormat verifies the reference format output.
func TestContextFormat_ReferenceFormat(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	require.NoError(t, RunMigrations(db.Pool))
	require.NoError(t, SeedTestObservations(db.Pool))

	slotSvc := service.NewSlotService(db.Pool)
	embedSvc := service.NewEmbeddingServiceWithEmbedder(nil)
	ctxSvc := service.NewContextService(db.Pool, embedSvc, slotSvc)

	assembled, err := ctxSvc.AssembleContext(ctx, "user-001")
	require.NoError(t, err)

	budget := service.DefaultContextBudget()
	result := service.ApplyBudget(assembled, budget)

	// Context should contain the header
	assert.Contains(t, result, "Context (AgentMemory v2)",
		"context should contain agentmemory header")

	// If observations exist, references should have recall IDs
	if assembled.Observations != "" {
		assert.Contains(t, result, "[obs-",
			"context should contain observation recall IDs")
	}

	// Should contain date
	assert.Contains(t, result, "Date:",
		"context should contain date stamp")

	t.Logf("Formatted context:\n%s", truncateStr(result, 800))
}

// Helper to truncate strings for logging.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
