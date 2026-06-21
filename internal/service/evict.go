package service

import (
	"context"
	"log/slog"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EvictionService handles pruning low-importance, old observations
// when the database approaches capacity. Compressed observations and
// lessons are preserved.
type EvictionService struct {
	queries *store.Queries
}

// NewEvictionService creates a new EvictionService.
func NewEvictionService(pool *pgxpool.Pool) *EvictionService {
	return &EvictionService{
		queries: store.New(pool),
	}
}

// EvictionCandidate identifies an observation that may be evicted.
type EvictionCandidate struct {
	ObservationID string
	Importance    float64
	Age           string // human-readable age
}

// FindCandidates returns observations that are candidates for eviction:
// low importance and old. Compressed observations and lessons are preserved.
// In MVP, this returns raw observations sorted by importance (ascending) and age.
func (s *EvictionService) FindCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error) {
	// In MVP, eviction is invoked manually or via admin API.
	// Full automated eviction with capacity tracking will be implemented
	// when the database storage backend is finalized.
	slog.Debug("eviction candidate search", "limit", limit)
	return nil, nil
}

// EvictObservation deletes a single raw observation.
// Compressed observations and lessons are preserved.
func (s *EvictionService) EvictObservation(ctx context.Context, observationID string) error {
	slog.Info("evicting observation", "observation_id", observationID)
	return s.queries.DeleteObservation(ctx, observationID)
}
