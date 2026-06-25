package service

import (
	"context"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContextQuerier implements contextQuerier for testing gatherRecap.
type mockContextQuerier struct {
	listSummariesByUserID func(ctx context.Context, params store.ListSummariesByUserIDParams) ([]store.SessionSummary, error)
	listObservationsByUserID func(ctx context.Context, params store.ListObservationsByUserIDParams) ([]store.Observation, error)
	getUserTeam            func(ctx context.Context, userID string) (store.Team, error)
	listLessonsByTeam      func(ctx context.Context, params store.ListLessonsByTeamParams) ([]store.Lesson, error)
	graphTraversal         func(ctx context.Context, params store.GraphTraversalParams) ([]store.GraphTraversalRow, error)
	getObservationsByIDs   func(ctx context.Context, ids []string) ([]store.Observation, error)
}

func (m *mockContextQuerier) ListSummariesByUserID(ctx context.Context, params store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
	return m.listSummariesByUserID(ctx, params)
}
func (m *mockContextQuerier) ListObservationsByUserID(ctx context.Context, params store.ListObservationsByUserIDParams) ([]store.Observation, error) {
	return m.listObservationsByUserID(ctx, params)
}
func (m *mockContextQuerier) GetUserTeam(ctx context.Context, userID string) (store.Team, error) {
	return m.getUserTeam(ctx, userID)
}
func (m *mockContextQuerier) ListLessonsByTeam(ctx context.Context, params store.ListLessonsByTeamParams) ([]store.Lesson, error) {
	return m.listLessonsByTeam(ctx, params)
}
func (m *mockContextQuerier) GraphTraversal(ctx context.Context, params store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
	return m.graphTraversal(ctx, params)
}
func (m *mockContextQuerier) GetObservationsByIDs(ctx context.Context, ids []string) ([]store.Observation, error) {
	return m.getObservationsByIDs(ctx, ids)
}

// TestGatherRecap_ShortSessionID_NoPanic verifies that gatherRecap does not panic
// when a SessionSummary has a SessionID shorter than 8 characters.
// Previously sum.SessionID[:8] would panic on short strings.
func TestGatherRecap_ShortSessionID_NoPanic(t *testing.T) {
	mock := &mockContextQuerier{
		listSummariesByUserID: func(_ context.Context, _ store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
			return []store.SessionSummary{
				{SessionID: "short", SummaryText: "session with short id"},
			}, nil
		},
		listObservationsByUserID: func(_ context.Context, _ store.ListObservationsByUserIDParams) ([]store.Observation, error) {
			return nil, nil
		},
		getUserTeam: func(_ context.Context, _ string) (store.Team, error) {
			return store.Team{}, nil
		},
		listLessonsByTeam: func(_ context.Context, _ store.ListLessonsByTeamParams) ([]store.Lesson, error) {
			return nil, nil
		},
		graphTraversal: func(_ context.Context, _ store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
			return nil, nil
		},
		getObservationsByIDs: func(_ context.Context, _ []string) ([]store.Observation, error) {
			return nil, nil
		},
	}

	svc := &ContextService{
		queries: mock,
	}

	result, err := svc.gatherRecap(context.Background(), "user-1")
	require.NoError(t, err)
	// With a short SessionID, the full ID should be used
	assert.Contains(t, result, "short")
}

// TestGatherRecap_NormalSessionID verifies that gatherRecap truncates long SessionIDs correctly.
func TestGatherRecap_NormalSessionID(t *testing.T) {
	mock := &mockContextQuerier{
		listSummariesByUserID: func(_ context.Context, _ store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
			return []store.SessionSummary{
				{SessionID: "abcdefghijklmnop", SummaryText: "session with long id"},
			}, nil
		},
		listObservationsByUserID: func(_ context.Context, _ store.ListObservationsByUserIDParams) ([]store.Observation, error) {
			return nil, nil
		},
		getUserTeam: func(_ context.Context, _ string) (store.Team, error) {
			return store.Team{}, nil
		},
		listLessonsByTeam: func(_ context.Context, _ store.ListLessonsByTeamParams) ([]store.Lesson, error) {
			return nil, nil
		},
		graphTraversal: func(_ context.Context, _ store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
			return nil, nil
		},
		getObservationsByIDs: func(_ context.Context, _ []string) ([]store.Observation, error) {
			return nil, nil
		},
	}

	svc := &ContextService{
		queries: mock,
	}

	result, err := svc.gatherRecap(context.Background(), "user-1")
	require.NoError(t, err)
	// Should contain the truncated 8-char prefix
	assert.Contains(t, result, "abcdefgh")
	assert.NotContains(t, result, "abcdefghijklmnop")
}

// TestGatherRecap_EmptySessionID verifies that gatherRecap handles an empty SessionID.
func TestGatherRecap_EmptySessionID(t *testing.T) {
	mock := &mockContextQuerier{
		listSummariesByUserID: func(_ context.Context, _ store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
			return []store.SessionSummary{
				{SessionID: "", SummaryText: "session with empty id"},
			}, nil
		},
		listObservationsByUserID: func(_ context.Context, _ store.ListObservationsByUserIDParams) ([]store.Observation, error) {
			return nil, nil
		},
		getUserTeam: func(_ context.Context, _ string) (store.Team, error) {
			return store.Team{}, nil
		},
		listLessonsByTeam: func(_ context.Context, _ store.ListLessonsByTeamParams) ([]store.Lesson, error) {
			return nil, nil
		},
		graphTraversal: func(_ context.Context, _ store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
			return nil, nil
		},
		getObservationsByIDs: func(_ context.Context, _ []string) ([]store.Observation, error) {
			return nil, nil
		},
	}

	svc := &ContextService{
		queries: mock,
	}

	result, err := svc.gatherRecap(context.Background(), "user-1")
	require.NoError(t, err)
	// An empty SessionID should not cause a panic; the full (empty) ID is used
	assert.Contains(t, result, "session with empty id")
}

// BenchmarkContextInjectionWithLLMExtractedEdges benchmarks context assembly
// time with and without LLM-extracted edges present in the graph.
// SC-003: Latency with LLM edges must be within 10% of baseline (no LLM edges).
func BenchmarkContextInjectionWithLLMExtractedEdges(b *testing.B) {
	b.Run("baseline/no-llm-edges", func(b *testing.B) {
		mock := &mockContextQuerier{
			listSummariesByUserID: func(_ context.Context, _ store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
				return []store.SessionSummary{
					{SessionID: "abcdefghijklmnop", SummaryText: "Session summary for benchmark testing."},
				}, nil
			},
			listObservationsByUserID: func(_ context.Context, _ store.ListObservationsByUserIDParams) ([]store.Observation, error) {
				return []store.Observation{
					{ID: "obs-1", Narrative: "Worked on authentication module refactoring."},
					{ID: "obs-2", Narrative: "Fixed bug in JWT token validation."},
					{ID: "obs-3", Narrative: "Reviewed database migration for user profiles."},
					{ID: "obs-4", Narrative: "Updated API documentation for rate limiting."},
					{ID: "obs-5", Narrative: "Refactored testing infrastructure."},
				}, nil
			},
			getUserTeam: func(_ context.Context, _ string) (store.Team, error) {
				return store.Team{}, nil
			},
			listLessonsByTeam: func(_ context.Context, _ store.ListLessonsByTeamParams) ([]store.Lesson, error) {
				return nil, nil
			},
			graphTraversal: func(_ context.Context, _ store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
				// Baseline: only structural edges (no LLM-discovered connections).
				// Only 2 observations are reachable via graph traversal.
				return []store.GraphTraversalRow{
					{ID: "obs-1", GraphScore: 0.5},
					{ID: "obs-2", GraphScore: 0.3},
				}, nil
			},
			getObservationsByIDs: func(_ context.Context, ids []string) ([]store.Observation, error) {
				return []store.Observation{
					{ID: "obs-1", Narrative: "Worked on authentication module refactoring."},
					{ID: "obs-2", Narrative: "Fixed bug in JWT token validation."},
				}, nil
			},
		}

		svc := &ContextService{queries: mock}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := svc.AssembleContext(context.Background(), "user-1")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("with-llm-extracted-edges", func(b *testing.B) {
		mock := &mockContextQuerier{
			listSummariesByUserID: func(_ context.Context, _ store.ListSummariesByUserIDParams) ([]store.SessionSummary, error) {
				return []store.SessionSummary{
					{SessionID: "abcdefghijklmnop", SummaryText: "Session summary for benchmark testing."},
				}, nil
			},
			listObservationsByUserID: func(_ context.Context, _ store.ListObservationsByUserIDParams) ([]store.Observation, error) {
				return []store.Observation{
					{ID: "obs-1", Narrative: "Worked on authentication module refactoring."},
					{ID: "obs-2", Narrative: "Fixed bug in JWT token validation."},
					{ID: "obs-3", Narrative: "Reviewed database migration for user profiles."},
					{ID: "obs-4", Narrative: "Updated API documentation for rate limiting."},
					{ID: "obs-5", Narrative: "Refactored testing infrastructure."},
				}, nil
			},
			getUserTeam: func(_ context.Context, _ string) (store.Team, error) {
				return store.Team{}, nil
			},
			listLessonsByTeam: func(_ context.Context, _ store.ListLessonsByTeamParams) ([]store.Lesson, error) {
				return nil, nil
			},
			graphTraversal: func(_ context.Context, _ store.GraphTraversalParams) ([]store.GraphTraversalRow, error) {
				// With LLM-extracted edges: more observations are reachable
				// because LLM-discovered connections create additional traversal paths.
				return []store.GraphTraversalRow{
					{ID: "obs-1", GraphScore: 0.9},
					{ID: "obs-2", GraphScore: 0.85},
					{ID: "obs-3", GraphScore: 0.7},
					{ID: "obs-4", GraphScore: 0.6},
					{ID: "obs-5", GraphScore: 0.5},
				}, nil
			},
			getObservationsByIDs: func(_ context.Context, ids []string) ([]store.Observation, error) {
				return []store.Observation{
					{ID: "obs-1", Narrative: "Worked on authentication module refactoring."},
					{ID: "obs-2", Narrative: "Fixed bug in JWT token validation."},
					{ID: "obs-3", Narrative: "Reviewed database migration for user profiles."},
					{ID: "obs-4", Narrative: "Updated API documentation for rate limiting."},
					{ID: "obs-5", Narrative: "Refactored testing infrastructure."},
				}, nil
			},
		}

		svc := &ContextService{queries: mock}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := svc.AssembleContext(context.Background(), "user-1")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
