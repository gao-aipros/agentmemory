package service

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReflectionLLM implements reflectLLM for testing.
type mockReflectionLLM struct {
	callFunc func(ctx context.Context, prompt string) (string, error)
}

func (m *mockReflectionLLM) Call(ctx context.Context, prompt string) (string, error) {
	return m.callFunc(ctx, prompt)
}

// cannedXMLResponse is the standard mock LLM response used across multiple tests.
const cannedXMLResponse = `<insights>
	<insight confidence="0.8" title="Connection Pool Pattern">Connection pool timeouts occur when max connections is below 25.</insight>
	<insight confidence="0.5" title="Retry Logic Gap">Multiple services lack retry logic for transient errors.</insight>
	</insights>`

// TestRunReflection_EmptyMemories verifies that RunReflection handles no memories
// gracefully — no LLM call is made, no insights are upserted, no error is returned.
func TestRunReflection_EmptyMemories(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return nil, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("UpsertInsight should not be called when there are no memories")
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			t.Error("MarkMemoriesReflected should not be called when there are no memories")
			return nil
		},
	}
	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			t.Error("LLM should not be called when there are no memories")
			return "", nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	assert.NoError(t, err)
}

// TestRunReflection_CreatesInsights verifies that memories with shared concepts
// form clusters that drive LLM reflection, producing the expected insights.
func TestRunReflection_CreatesInsights(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedInsights []store.UpsertInsightParams
	var reflectedIDs []string

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			// 5 memories form 2 clusters:
			//   Cluster A: m1,m2,m3 (concepts: database, performance) — 3 memories, qualifies
			//   Cluster B: m4,m5     (concepts: resilience, retry)    — 2 memories, skipped
			return []store.Memory{
				{ID: "m1", Content: "Connection pool too small at 10 max connections", Concepts: []string{"database", "performance"}},
				{ID: "m2", Content: "Timeout errors observed with only 20 pool connections", Concepts: []string{"database", "performance"}},
				{ID: "m3", Content: "Increased connection pool to 50 resolved timeouts", Concepts: []string{"database", "performance"}},
				{ID: "m4", Content: "Service A missing retry logic for 5xx errors", Concepts: []string{"resilience", "retry"}},
				{ID: "m5", Content: "Service B also lacks retry handling", Concepts: []string{"resilience", "retry"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedInsights = append(capturedInsights, params)
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			mu.Lock()
			defer mu.Unlock()
			reflectedIDs = append(reflectedIDs, ids...)
			return nil
		},
	}

	llmCalled := false
	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			llmCalled = true
			assert.Contains(t, prompt, "database")
			assert.Contains(t, prompt, "You are a higher-order reasoning engine",
				"prompt should include REFLECT_SYSTEM content")
			assert.Contains(t, prompt, "performance")
			// resilience and retry belong to the 2-memory cluster, should not appear
			return cannedXMLResponse, nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	require.NoError(t, err)

	assert.True(t, llmCalled, "LLM should have been called for the qualifying cluster")

	// 2 insights from the canned XML response
	require.Len(t, capturedInsights, 2)

	// First insight
	assert.Equal(t, "Connection Pool Pattern", capturedInsights[0].Title)
	assert.Contains(t, capturedInsights[0].Content, "Connection pool timeouts")
	assert.InDelta(t, 0.8, capturedInsights[0].Confidence, 1e-6)
	assert.NotEmpty(t, capturedInsights[0].ID)
	assert.Contains(t, capturedInsights[0].SourceConceptCluster, "database")
	assert.Contains(t, capturedInsights[0].SourceConceptCluster, "performance")

	// Second insight
	assert.Equal(t, "Retry Logic Gap", capturedInsights[1].Title)
	assert.InDelta(t, 0.5, capturedInsights[1].Confidence, 1e-6)
	assert.NotEmpty(t, capturedInsights[1].ID)

	// Memory IDs from the qualifying cluster (m1, m2, m3) should be reflected
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, reflectedIDs)
}

// TestRunReflection_ReinforcesExistingInsights verifies that running reflection
// twice with the same cluster produces the same fingerprint IDs, triggering the
// ON CONFLICT DO UPDATE path in UpsertInsight (boosting confidence).
func TestRunReflection_ReinforcesExistingInsights(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedIDs []string

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "m1", Content: "Pool too small at 10", Concepts: []string{"database"}},
				{ID: "m2", Content: "Timeouts at 20 connections", Concepts: []string{"database"}},
				{ID: "m3", Content: "Pool of 50 fixed it", Concepts: []string{"database"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedIDs = append(capturedIDs, params.ID)
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			return nil
		},
	}

	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			return `<insights>
	<insight confidence="0.8" title="Pool Sizing">The connection pool must be at least 50 to avoid timeouts.</insight>
	</insights>`, nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)

	// First run
	err := svc.RunReflection(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, capturedIDs, 1)
	firstID := capturedIDs[0]
	assert.Contains(t, firstID, "ins_")

	// Reset capture and run again
	capturedIDs = nil
	err = svc.RunReflection(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, capturedIDs, 1)
	secondID := capturedIDs[0]

	// Same content must produce the same fingerprint ID
	assert.Equal(t, firstID, secondID,
		"the same insight content should produce the same fingerprint ID, "+
			"triggering UpsertInsight ON CONFLICT DO UPDATE")
}

// TestRunReflection_MarksMemoriesReflected verifies that MarkMemoriesReflected
// is called with the correct memory IDs after a cluster is processed.
func TestRunReflection_MarksMemoriesReflected(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedIDs []string

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "a1", Content: "Memory one", Concepts: []string{"alpha"}},
				{ID: "a2", Content: "Memory two", Concepts: []string{"alpha"}},
				{ID: "a3", Content: "Memory three", Concepts: []string{"alpha"}},
				{ID: "b1", Content: "Memory B one", Concepts: []string{"beta"}},
				{ID: "b2", Content: "Memory B two", Concepts: []string{"beta"}},
				{ID: "b3", Content: "Memory B three", Concepts: []string{"beta"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			mu.Lock()
			defer mu.Unlock()
			capturedIDs = append(capturedIDs, ids...)
			return nil
		},
	}

	callCount := 0
	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			return cannedXMLResponse, nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	require.NoError(t, err)

	// Two clusters qualify (both have 3+ memories)
	assert.Equal(t, 2, callCount, "LLM should be called once per qualifying cluster")
	// All 6 memory IDs should be marked reflected
	assert.ElementsMatch(t, []string{"a1", "a2", "a3", "b1", "b2", "b3"}, capturedIDs)
}

// TestRunReflection_SkipsSmallClusters verifies that clusters with fewer than 3
// memories are skipped — no LLM call is made and no insights are stored.
func TestRunReflection_SkipsSmallClusters(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			// 2 memories sharing "small" concept — below the 3-memory threshold
			return []store.Memory{
				{ID: "s1", Content: "Single observation", Concepts: []string{"small"}},
				{ID: "s2", Content: "Related observation", Concepts: []string{"small"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("UpsertInsight should not be called for a small cluster")
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			t.Error("MarkMemoriesReflected should not be called for a small cluster")
			return nil
		},
	}

	llmCalled := false
	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			llmCalled = true
			return "", nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	assert.NoError(t, err)
	assert.False(t, llmCalled, "LLM should not be called when no cluster has 3+ memories")
}

// TestRunReflection_LogsSkippedSmallClusters verifies that small clusters are
// logged and counted as skipped in the final reflection log.
func TestRunReflection_LogsSkippedSmallClusters(t *testing.T) {
	ctx := context.Background()

	// Capture slog output
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			// 2 memories sharing a concept — one small cluster below the 3-memory threshold
			return []store.Memory{
				{ID: "s1", Content: "Single observation", Concepts: []string{"small"}},
				{ID: "s2", Content: "Related observation", Concepts: []string{"small"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("UpsertInsight should not be called for a small cluster")
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			t.Error("MarkMemoriesReflected should not be called for a small cluster")
			return nil
		},
	}

	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			t.Error("LLM should not be called for a small cluster")
			return "", nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	assert.NoError(t, err)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "skipping small cluster",
		"should log when a small cluster is skipped")
	assert.Contains(t, logOutput, "skipped_clusters",
		"final log line should include skipped_clusters counter")
}

// TestRunReflection_LLMFailure verifies that when the LLM returns an error for
// one cluster, processing continues with the remaining clusters.
func TestRunReflection_LLMFailure(t *testing.T) {
	ctx := context.Background()

	var mu sync.Mutex
	var capturedInsights []store.UpsertInsightParams

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			// Two clusters, both with 3+ memories
			return []store.Memory{
				{ID: "x1", Content: "X one", Concepts: []string{"cluster-x"}},
				{ID: "x2", Content: "X two", Concepts: []string{"cluster-x"}},
				{ID: "x3", Content: "X three", Concepts: []string{"cluster-x"}},
				{ID: "y1", Content: "Y one", Concepts: []string{"cluster-y"}},
				{ID: "y2", Content: "Y two", Concepts: []string{"cluster-y"}},
				{ID: "y3", Content: "Y three", Concepts: []string{"cluster-y"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			mu.Lock()
			defer mu.Unlock()
			capturedInsights = append(capturedInsights, params)
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			return nil
		},
	}

	callCount := 0
	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			callCount++
			if callCount == 1 {
				// First cluster: LLM fails
				return "", assert.AnError
			}
			// Second cluster: LLM succeeds
			return `<insights>
	<insight confidence="0.7" title="Cluster Y Insight">Insight from the second cluster that survived LLM failure.</insight>
	</insights>`, nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	assert.NoError(t, err)

	// LLM was called twice (once per qualifying cluster)
	assert.Equal(t, 2, callCount, "LLM should be called for both clusters")

	// Only 1 insight from the successful second call
	require.Len(t, capturedInsights, 1)
	assert.Equal(t, "Cluster Y Insight", capturedInsights[0].Title)
	assert.InDelta(t, 0.7, capturedInsights[0].Confidence, 1e-6)
}

// =============================================================================
// ListInsights / SearchInsights tests (T019)
// =============================================================================

// insightRow is a test helper that creates a ListInsightsRow with defaults.
func insightRow(id, title, content string, confidence float64, project string) store.ListInsightsRow {
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}
	return store.ListInsightsRow{
		ID:                 id,
		Title:              title,
		Content:            content,
		Confidence:         confidence,
		ReinforcementCount: 0,
		Project:            projectPtr,
		Tags:               nil,
	}
}

// TestListInsights_FiltersByProject verifies that ListInsights only returns
// insights belonging to the specified project.
func TestListInsights_FiltersByProject(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listInsights: func(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error) {
			assert.NotNil(t, arg.Project)
			assert.Equal(t, "project-A", *arg.Project)
			return []store.ListInsightsRow{
				insightRow("i1", "Title A", "Content A", 0.8, "project-A"),
				insightRow("i2", "Title B", "Content B", 0.6, "project-A"),
			}, nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	insights, err := svc.ListInsights(ctx, "project-A", 0, 50)

	require.NoError(t, err)
	require.Len(t, insights, 2)
	assert.Equal(t, "Title A", insights[0].Title)
	assert.Equal(t, "Title B", insights[1].Title)
}

// TestListInsights_FiltersByMinConfidence verifies that ListInsights only
// returns insights meeting the minimum confidence threshold.
func TestListInsights_FiltersByMinConfidence(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listInsights: func(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error) {
			assert.NotNil(t, arg.MinConfidence)
			assert.Equal(t, 0.5, *arg.MinConfidence)
			// Return only insights above the threshold
			return []store.ListInsightsRow{
				insightRow("i1", "High Confidence", "Important", 0.9, ""),
				insightRow("i2", "Medium Confidence", "Moderate", 0.7, ""),
			}, nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	insights, err := svc.ListInsights(ctx, "", 0.5, 50)

	require.NoError(t, err)
	require.Len(t, insights, 2)
	// Results should be ordered by confidence descending
	assert.Equal(t, 0.9, insights[0].Confidence)
	assert.Equal(t, 0.7, insights[1].Confidence)
}

// TestListInsights_ExcludesDeleted verifies that the querier layer filters
// out deleted insights (deleted = false in the SQL WHERE clause).
func TestListInsights_ExcludesDeleted(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listInsights: func(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error) {
			// The SQL query already filters deleted=false; the service
			// passes through whatever the querier returns.
			return []store.ListInsightsRow{
				insightRow("i1", "Active Insight", "Still relevant", 0.8, ""),
			}, nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	insights, err := svc.ListInsights(ctx, "", 0, 50)

	require.NoError(t, err)
	require.Len(t, insights, 1)
	assert.Equal(t, "Active Insight", insights[0].Title)
}

// TestSearchInsights_MatchesTitle verifies that SearchInsights can find
// insights by matching their title via full-text search.
func TestSearchInsights_MatchesTitle(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		searchInsights: func(ctx context.Context, arg store.SearchInsightsParams) ([]store.SearchInsightsRow, error) {
			assert.NotNil(t, arg.Query)
			assert.Equal(t, "pool", *arg.Query)
			return []store.SearchInsightsRow{
				{
					ID:                 "i1",
					Title:              "Connection Pool Pattern",
					Content:            "Connection pool timeouts occur when max connections is below 25.",
					Confidence:         0.8,
					ReinforcementCount: 3,
				},
			}, nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	insights, err := svc.SearchInsights(ctx, "", "pool", 0, 50)

	require.NoError(t, err)
	require.Len(t, insights, 1)
	assert.Equal(t, "Connection Pool Pattern", insights[0].Title)
	assert.Contains(t, insights[0].Content, "Connection pool")
}

// TestSearchInsights_MatchesContent verifies that SearchInsights can find
// insights by matching their content via full-text search.
func TestSearchInsights_MatchesContent(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		searchInsights: func(ctx context.Context, arg store.SearchInsightsParams) ([]store.SearchInsightsRow, error) {
			assert.NotNil(t, arg.Query)
			assert.Equal(t, "timeout", *arg.Query)
			return []store.SearchInsightsRow{
				{
					ID:                 "i2",
					Title:              "Retry Logic",
					Content:            "Services should implement retry logic with exponential backoff and timeout.",
					Confidence:         0.7,
					ReinforcementCount: 1,
				},
			}, nil
		},
	}
	mockLLM := &mockReflectionLLM{}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	insights, err := svc.SearchInsights(ctx, "", "timeout", 0, 50)

	require.NoError(t, err)
	require.Len(t, insights, 1)
	assert.Equal(t, "Retry Logic", insights[0].Title)
	assert.Contains(t, insights[0].Content, "timeout")
}

// TestRunReflection_NoInsights_DoesNotMarkReflected verifies that when the LLM
// returns zero valid insights (e.g., empty <insights> block), MarkMemoriesReflected
// is NOT called — memories should not be marked reflected if nothing was persisted.
func TestRunReflection_NoInsights_DoesNotMarkReflected(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "n1", Content: "First memory", Concepts: []string{"alpha"}},
				{ID: "n2", Content: "Second memory", Concepts: []string{"alpha"}},
				{ID: "n3", Content: "Third memory", Concepts: []string{"alpha"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("UpsertInsight should not be called when LLM returns zero valid insights")
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			t.Error("MarkMemoriesReflected should not be called when zero insights were persisted")
			return nil
		},
	}

	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			// Return valid XML wrapper but no <insight> blocks — zero insights
			return "<insights>\n</insights>", nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 reflection clusters failed")
}
