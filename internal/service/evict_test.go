package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEvictionQuerier implements evictionQuerier for testing.
type mockEvictionQuerier struct {
	candidates []EvictionCandidate
	err        error
}

func (m *mockEvictionQuerier) findEvictionCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.candidates) > limit {
		return m.candidates[:limit], nil
	}
	return m.candidates, nil
}

func (m *mockEvictionQuerier) deleteObservation(ctx context.Context, id string) error {
	return nil
}

func newEvictionServiceWithQuerier(q evictionQuerier) *EvictionService {
	return &EvictionService{
		queries: q,
	}
}

func TestEviction_FindCandidates_ReturnsCandidates(t *testing.T) {
	mock := &mockEvictionQuerier{
		candidates: []EvictionCandidate{
			{ObservationID: "obs-1", Importance: 0.1, Age: "30.5 days"},
			{ObservationID: "obs-2", Importance: 0.15, Age: "25.0 days"},
		},
	}
	svc := newEvictionServiceWithQuerier(mock)

	candidates, err := svc.FindCandidates(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
	assert.Equal(t, "obs-1", candidates[0].ObservationID)
	assert.Equal(t, 0.1, candidates[0].Importance)
	assert.Equal(t, "30.5 days", candidates[0].Age)
}

func TestEviction_FindCandidates_EmptyResult(t *testing.T) {
	mock := &mockEvictionQuerier{
		candidates: []EvictionCandidate{},
	}
	svc := newEvictionServiceWithQuerier(mock)

	candidates, err := svc.FindCandidates(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, candidates)
}

func TestEviction_FindCandidates_ErrorPropagation(t *testing.T) {
	mock := &mockEvictionQuerier{
		err: fmt.Errorf("database connection lost"),
	}
	svc := newEvictionServiceWithQuerier(mock)

	_, err := svc.FindCandidates(context.Background(), 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection lost")
}

func TestEviction_FindCandidates_RespectsLimit(t *testing.T) {
	mock := &mockEvictionQuerier{
		candidates: []EvictionCandidate{
			{ObservationID: "obs-1", Importance: 0.1, Age: "1 day"},
			{ObservationID: "obs-2", Importance: 0.2, Age: "2 days"},
			{ObservationID: "obs-3", Importance: 0.3, Age: "3 days"},
		},
	}
	svc := newEvictionServiceWithQuerier(mock)

	candidates, err := svc.FindCandidates(context.Background(), 2)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
}
