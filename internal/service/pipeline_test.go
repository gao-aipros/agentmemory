package service

import (
	"context"
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/llms"
)

// mockSummarizationQuerier implements summarizationQuerier for testing.
type mockSummarizationQuerier struct {
	listObservationsBySession func(ctx context.Context, params store.ListObservationsBySessionParams) ([]store.Observation, error)
	upsertSessionSummary      func(ctx context.Context, params store.UpsertSessionSummaryParams) (store.SessionSummary, error)
}

func (m *mockSummarizationQuerier) ListObservationsBySession(ctx context.Context, params store.ListObservationsBySessionParams) ([]store.Observation, error) {
	return m.listObservationsBySession(ctx, params)
}

func (m *mockSummarizationQuerier) UpsertSessionSummary(ctx context.Context, params store.UpsertSessionSummaryParams) (store.SessionSummary, error) {
	return m.upsertSessionSummary(ctx, params)
}

// mockModel implements llms.Model for testing.
type mockModel struct {
	callFunc func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error)
}

func (m *mockModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return m.callFunc(ctx, prompt, options...)
}

func (m *mockModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	return nil, nil
}

// mockReflectionQuerier implements reflectionQuerier for testing.
type mockReflectionQuerier struct {
	listMemories          func(ctx context.Context, limit int32) ([]store.Memory, error)
	upsertInsight         func(ctx context.Context, params store.UpsertInsightParams) error
	markMemoriesReflected func(ctx context.Context, ids []string) error
	applyDecay            func(ctx context.Context, weeksSince float64) error
	listInsights          func(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error)
	searchInsights        func(ctx context.Context, arg store.SearchInsightsParams) ([]store.SearchInsightsRow, error)
}

func (m *mockReflectionQuerier) ListAllMemories(ctx context.Context, limit int32) ([]store.Memory, error) {
	return m.listMemories(ctx, limit)
}

func (m *mockReflectionQuerier) UpsertInsight(ctx context.Context, params store.UpsertInsightParams) error {
	return m.upsertInsight(ctx, params)
}

func (m *mockReflectionQuerier) MarkMemoriesReflected(ctx context.Context, ids []string) error {
	return m.markMemoriesReflected(ctx, ids)
}

func (m *mockReflectionQuerier) ApplyDecay(ctx context.Context, weeksSince float64) error {
	if m.applyDecay == nil {
		return nil
	}
	return m.applyDecay(ctx, weeksSince)
}

func (m *mockReflectionQuerier) ListInsights(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error) {
	if m.listInsights == nil {
		return nil, nil
	}
	return m.listInsights(ctx, arg)
}

func (m *mockReflectionQuerier) SearchInsights(ctx context.Context, arg store.SearchInsightsParams) ([]store.SearchInsightsRow, error) {
	if m.searchInsights == nil {
		return nil, nil
	}
	return m.searchInsights(ctx, arg)
}

// TestReflectionService_RunReflection_NoMemories verifies that RunReflection
// handles an empty memories table gracefully.
func TestReflectionService_RunReflection_NoMemories(t *testing.T) {
	ctx := context.Background()

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return nil, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			t.Error("insertInsight should not be called when there are no memories")
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
	if err != nil {
		t.Fatalf("RunReflection should not error with empty memories, got: %v", err)
	}
}

// TestReflectionService_RunReflection_WithMemories verifies that RunReflection
// produces insights from memories with shared concepts via the LLM.
func TestReflectionService_RunReflection_WithMemories(t *testing.T) {
	ctx := context.Background()

	var capturedInsights []store.UpsertInsightParams

	mockQ := &mockReflectionQuerier{
		listMemories: func(ctx context.Context, limit int32) ([]store.Memory, error) {
			return []store.Memory{
				{ID: "1", Content: "First memory about auth", Concepts: []string{"auth", "login"}},
				{ID: "2", Content: "Second memory about auth", Concepts: []string{"auth", "login"}},
				{ID: "3", Content: "Third memory about auth", Concepts: []string{"auth", "security"}},
			}, nil
		},
		upsertInsight: func(ctx context.Context, params store.UpsertInsightParams) error {
			capturedInsights = append(capturedInsights, params)
			return nil
		},
		markMemoriesReflected: func(ctx context.Context, ids []string) error {
			return nil
		},
	}

	mockLLM := &mockReflectionLLM{
		callFunc: func(ctx context.Context, prompt string) (string, error) {
			return `<insights>
	<insight confidence="0.8" title="Auth Pattern">Authentication patterns show consistent login flows.</insight>
	</insights>`, nil
		},
	}

	svc := newReflectionServiceWithQuerier(mockQ, mockLLM)
	err := svc.RunReflection(ctx, "", 10)
	if err != nil {
		t.Fatalf("RunReflection should not error with memories, got: %v", err)
	}

	// Should produce exactly 1 insight (from the mock LLM response)
	if len(capturedInsights) != 1 {
		t.Fatalf("expected 1 insight from mock LLM, got %d", len(capturedInsights))
	}
	assert.Equal(t, "Auth Pattern", capturedInsights[0].Title)
	assert.InDelta(t, 0.8, capturedInsights[0].Confidence, 1e-6)
	assert.Contains(t, capturedInsights[0].Content, "Authentication patterns")
	assert.Contains(t, capturedInsights[0].SourceConceptCluster, "auth")
}

func TestSummarizeSession_EmptyLLMResponse(t *testing.T) {
	ctx := context.Background()

	// Create a long narrative that forces each observation into its own chunk
	// (~3300 tokens each, exceeding the 3000 token budget).
	longNarrative := strings.Repeat("A long narrative to fill the token budget. ", 300)

	var capturedSummary string

	mockQ := &mockSummarizationQuerier{
		listObservationsBySession: func(ctx context.Context, params store.ListObservationsBySessionParams) ([]store.Observation, error) {
			return []store.Observation{
				{Title: "obs1", Narrative: longNarrative},
				{Title: "obs2", Narrative: longNarrative},
			}, nil
		},
		upsertSessionSummary: func(ctx context.Context, params store.UpsertSessionSummaryParams) (store.SessionSummary, error) {
			capturedSummary = params.SummaryText
			return store.SessionSummary{}, nil
		},
	}

	// First LLM call returns "valid summary", second returns "" (empty).
	callCount := 0
	mockModel := &mockModel{
		callFunc: func(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
			callCount++
			if callCount == 1 {
				return "valid summary", nil
			}
			return "", nil
		},
	}

	llmSvc := NewLLMServiceWithModel(mockModel)
	svc := newSummarizationServiceWithQuerier(mockQ, llmSvc)

	err := svc.SummarizeSession(ctx, "test-session")
	if err != nil {
		t.Fatalf("SummarizeSession should not error when LLM returns empty for a chunk, got: %v", err)
	}

	// Before the fix: capturedSummary would be "valid summary\n" because the
	// empty second chunk is still appended (with preceding separator).
	// After the fix: capturedSummary is "valid summary" (empty chunk skipped).
	if capturedSummary != "valid summary" {
		t.Errorf("expected summary to be 'valid summary', got %q", capturedSummary)
	}
}
