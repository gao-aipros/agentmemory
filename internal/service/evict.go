package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// evictionQuerier handles eviction-related database queries.
// The concrete *evictionQuerierImpl satisfies this, enabling mock-based testing.
type evictionQuerier interface {
	findEvictionCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error)
	deleteObservation(ctx context.Context, id string) error
}

// evictionQuerierImpl is the production implementation using raw SQL.
type evictionQuerierImpl struct {
	pool *pgxpool.Pool
}

func (q *evictionQuerierImpl) findEvictionCandidates(ctx context.Context, limit int) ([]EvictionCandidate, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT id, importance,
		        EXTRACT(EPOCH FROM (now() - created_at)) / 86400 AS age_days
		 FROM observations
		 WHERE importance < $1
		 ORDER BY importance ASC, created_at ASC
		 LIMIT $2`,
		0.2, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query eviction candidates: %w", err)
	}
	defer rows.Close()

	var candidates []EvictionCandidate
	for rows.Next() {
		var c EvictionCandidate
		var ageDays float64
		if err := rows.Scan(&c.ObservationID, &c.Importance, &ageDays); err != nil {
			return nil, fmt.Errorf("failed to scan eviction candidate: %w", err)
		}
		c.Age = fmt.Sprintf("%.1f days", ageDays)
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating eviction candidates: %w", err)
	}

	return candidates, nil
}

func (q *evictionQuerierImpl) deleteObservation(ctx context.Context, id string) error {
	_, err := q.pool.Exec(ctx, "DELETE FROM observations WHERE id = $1", id)
	return err
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
		queries: &evictionQuerierImpl{pool: pool},
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
	return s.queries.findEvictionCandidates(ctx, limit)
}

// EvictObservation deletes a single raw observation.
// Compressed observations and lessons are preserved.
func (s *EvictionService) EvictObservation(ctx context.Context, observationID string) error {
	slog.Info("evicting observation", "observation_id", observationID)
	return s.queries.deleteObservation(ctx, observationID)
}
