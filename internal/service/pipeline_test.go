package service

import (
	"context"
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
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

// TestSummarizeSession_EmptyLLMResponse verifies that empty responses from the
// LLM are gracefully skipped rather than silently appended to the summary.
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
