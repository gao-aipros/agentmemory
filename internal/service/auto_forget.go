package service

import (
	"context"
	"fmt"
	"log"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AutoForgetService detects contradictions between observations and
// automatically evicts the lower-confidence entry.
type AutoForgetService struct {
	queries *store.Queries
}

// NewAutoForgetService creates a new AutoForgetService.
func NewAutoForgetService(pool *pgxpool.Pool) *AutoForgetService {
	return &AutoForgetService{
		queries: store.New(pool),
	}
}

// DetectAndResolve searches for observations similar to newObs (same concepts
// or files) and, if a contradiction is detected, evicts the observation with
// the lower importance/confidence score.
func (s *AutoForgetService) DetectAndResolve(ctx context.Context, newObs *store.Observation) error {
	if newObs == nil {
		return fmt.Errorf("newObs cannot be nil")
	}

	// Search for existing observations with overlapping concepts or files.
	existingObs, err := s.findSimilarObservations(ctx, newObs)
	if err != nil {
		return fmt.Errorf("finding similar observations: %w", err)
	}

	for _, existing := range existingObs {
		if s.CheckContradiction(&existing, newObs) {
			log.Printf("[INFO] auto-forget: contradiction detected between %s and %s (same concepts/files, conflicting narratives)",
				existing.ID, newObs.ID)

			if existing.Importance <= newObs.Importance {
				log.Printf("[INFO] auto-forget: evicting existing observation %s (importance %.2f <= %.2f)",
					existing.ID, existing.Importance, newObs.Importance)
				if err := s.evictObservation(ctx, existing.ID); err != nil {
					return fmt.Errorf("evicting observation %s: %w", existing.ID, err)
				}
			} else {
				log.Printf("[INFO] auto-forget: keeping existing observation %s (importance %.2f > %.2f), discarding new",
					existing.ID, existing.Importance, newObs.Importance)
			}
		}
	}

	return nil
}

// CheckContradiction returns true if two observations appear to contradict
// each other (share concepts or files but have different narratives).
// This is a basic heuristic: same concepts/files but different facts/narratives
// is treated as a contradiction.
func (s *AutoForgetService) CheckContradiction(existing *store.Observation, new *store.Observation) bool {
	if existing == nil || new == nil {
		return false
	}

	// Must share at least one concept or file to be candidates for contradiction.
	hasOverlap := hasAnyOverlap(existing.Concepts, new.Concepts) ||
		hasAnyOverlap(existing.Files, new.Files)

	if !hasOverlap {
		return false
	}

	// If narratives are identical (trimmed), it is not a contradiction — it is
	// a duplicate. Duplicates are not auto-forgotten here; the eviction service
	// handles exact duplicates.
	if existing.Narrative == new.Narrative {
		return false
	}

	// Contradiction check: different narratives on the same topic suggest conflict.
	return true
}

// findSimilarObservations returns observations that share concepts or files
// with the given observation.
func (s *AutoForgetService) findSimilarObservations(ctx context.Context, obs *store.Observation) ([]store.Observation, error) {
	// Collect concept-based matches.
	seen := make(map[string]bool)
	var results []store.Observation

	// Search by each concept's observations (via search).
	// For MVP we query recent observations and filter in-memory.
	recentObs, err := s.queries.ListRecentObservations(ctx, 100)
	if err != nil {
		return nil, err
	}

	for _, o := range recentObs {
		if o.ID == obs.ID {
			continue
		}
		if seen[o.ID] {
			continue
		}
		if hasAnyOverlap(obs.Concepts, o.Concepts) || hasAnyOverlap(obs.Files, o.Files) {
			results = append(results, o)
			seen[o.ID] = true
		}
	}

	return results, nil
}

// evictObservation deletes an observation by ID.
func (s *AutoForgetService) evictObservation(ctx context.Context, id string) error {
	// Construct a synthetic observation with just the ID for the delete call.
	return s.queries.DeleteObservation(ctx, id)
}

// hasAnyOverlap returns true if the two string slices share at least one element.
func hasAnyOverlap(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if set[s] {
			return true
		}
	}
	return false
}

// generateID generates a new UUID string for service-internal use.
func generateID() string {
	return uuid.New().String()
}
