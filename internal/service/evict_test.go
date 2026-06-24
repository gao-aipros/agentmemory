package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEvictionQuerier implements evictionQuerier for testing.
type mockEvictionQuerier struct {
	candidates []store.ListEvictionCandidatesRow
	err        error
}

func (m *mockEvictionQuerier) ListEvictionCandidates(ctx context.Context, params store.ListEvictionCandidatesParams) ([]store.ListEvictionCandidatesRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(m.candidates) > int(params.Limit) {
		return m.candidates[:params.Limit], nil
	}
	return m.candidates, nil
}

func (m *mockEvictionQuerier) DeleteObservation(ctx context.Context, id string) error {
	return nil
}

func newEvictionServiceWithQuerier(q evictionQuerier) *EvictionService {
	return &EvictionService{
		queries: q,
	}
}

func TestEviction_FindCandidates_ReturnsCandidates(t *testing.T) {
	mock := &mockEvictionQuerier{
		candidates: []store.ListEvictionCandidatesRow{
			{ID: "obs-1", Importance: 0.1, AgeDays: 30},
			{ID: "obs-2", Importance: 0.15, AgeDays: 25},
		},
	}
	svc := newEvictionServiceWithQuerier(mock)

	candidates, err := svc.FindCandidates(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
	assert.Equal(t, "obs-1", candidates[0].ObservationID)
	assert.Equal(t, 0.1, candidates[0].Importance)
	assert.Equal(t, "30.0 days", candidates[0].Age)
}

func TestEviction_FindCandidates_EmptyResult(t *testing.T) {
	mock := &mockEvictionQuerier{
		candidates: []store.ListEvictionCandidatesRow{},
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
		candidates: []store.ListEvictionCandidatesRow{
			{ID: "obs-1", Importance: 0.1, AgeDays: 1},
			{ID: "obs-2", Importance: 0.2, AgeDays: 2},
			{ID: "obs-3", Importance: 0.3, AgeDays: 3},
		},
	}
	svc := newEvictionServiceWithQuerier(mock)

	candidates, err := svc.FindCandidates(context.Background(), 2)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
}
