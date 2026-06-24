package service

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPatternsQuerier implements patternsQuerier for testing.
type mockPatternsQuerier struct {
	concepts []store.GetConceptFrequenciesRow
	err      error
}

func (m *mockPatternsQuerier) GetConceptFrequencies(ctx context.Context) ([]store.GetConceptFrequenciesRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.concepts, nil
}

func TestPatterns_DetectPatterns_ReturnsConceptFrequencies(t *testing.T) {
	mock := &mockPatternsQuerier{
		concepts: []store.GetConceptFrequenciesRow{
			{Concept: "authentication", Freq: 10},
			{Concept: "database", Freq: 7},
			{Concept: "logging", Freq: 3},
		},
	}
	svc := newPatternsServiceWithQuerier(mock)

	summary, err := svc.DetectPatterns(context.Background(), "my-project")
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Equal(t, "my-project", summary.Project)
	assert.Len(t, summary.TopConcepts, 3)
	assert.Equal(t, "authentication", summary.TopConcepts[0].Concept)
	assert.Equal(t, 10, summary.TopConcepts[0].Count)
	// Other fields remain at zero/empty values
	assert.Empty(t, summary.ToolUsage)
	assert.Empty(t, summary.FilePatterns)
	assert.Equal(t, 0, summary.SessionCount)
}

func TestPatterns_DetectPatterns_EmptyResult(t *testing.T) {
	mock := &mockPatternsQuerier{
		concepts: []store.GetConceptFrequenciesRow{},
	}
	svc := newPatternsServiceWithQuerier(mock)

	summary, err := svc.DetectPatterns(context.Background(), "empty-project")
	require.NoError(t, err)
	require.NotNil(t, summary)
	assert.Empty(t, summary.TopConcepts)
	assert.Equal(t, "empty-project", summary.Project)
}

func TestPatterns_DetectPatterns_ErrorPropagation(t *testing.T) {
	mock := &mockPatternsQuerier{
		err: assert.AnError,
	}
	svc := newPatternsServiceWithQuerier(mock)

	_, err := svc.DetectPatterns(context.Background(), "fail-project")
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}
