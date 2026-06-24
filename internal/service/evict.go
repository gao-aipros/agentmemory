package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// evictionQuerier is the subset of *store.Queries methods used by EvictionService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type evictionQuerier interface {
	ListEvictionCandidates(ctx context.Context, params store.ListEvictionCandidatesParams) ([]store.ListEvictionCandidatesRow, error)
	DeleteObservation(ctx context.Context, id string) error
}

// EvictionService handles pruning low-importance, old observations
// when the database approaches capacity. Compressed observations and
// lessons are preserved.
type EvictionService struct {
	queries evictionQuerier
}

// NewEvictionService creates a new EvictionService.
func NewEvictionService(pool *pgxpool.Pool) *EvictionService {
	return &EvictionService{
		queries: store.New(pool),
	}
}

// newEvictionServiceWithQuerier creates an EvictionService with a custom querier (for testing).
func newEvictionServiceWithQuerier(q evictionQuerier) *EvictionService {
	return &EvictionService{
		queries: q,
	}
}

// EvictionCandidate identifies an observation that may be evicted.
type EvictionCandidate struct {
	ObservationID string
	Importance    float64
	Age           string // human-readable age, e.g. "30.5 days"
}

// FindCandidates returns observations that are candidates for eviction:
// low importance (below 0.2) and old. Compressed observations and lessons
// are preserved. Results sorted by importance ascending, age ascending.
func (s *EvictionService) FindCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error) {
	if limit <= 0 {
		limit = 50
	}
	slog.Debug("searching for eviction candidates", "limit", limit)
	rows, err := s.queries.ListEvictionCandidates(ctx, store.ListEvictionCandidatesParams{
		Importance: 0.2,
		Limit:      int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list eviction candidates: %w", err)
	}
	candidates := make([]EvictionCandidate, len(rows))
	for i, row := range rows {
		candidates[i] = EvictionCandidate{
			ObservationID: row.ID,
			Importance:    row.Importance,
			Age:           fmt.Sprintf("%.1f days", float64(row.AgeDays)),
		}
	}
	return candidates, nil
}

// EvictObservation deletes a single raw observation.
// Compressed observations and lessons are preserved.
func (s *EvictionService) EvictObservation(ctx context.Context, observationID string) error {
	slog.Info("evicting observation", "observation_id", observationID)
	return s.queries.DeleteObservation(ctx, observationID)
}
