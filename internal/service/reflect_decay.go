package service

import (
	"context"
	"fmt"
	"log/slog"
)

// DecayInsights applies confidence decay to all non-reinforced insights.
// The underlying ApplyDecay query handles the UPDATE logic, including:
//   - Reducing confidence by decay_rate * weeksSince (floored at 0.05)
//   - Skipping insights reinforced within the last week
//   - Soft-deleting insights whose confidence drops below 0.1 with 0 reinforcements
//   - Skipping already-deleted insights
//
// Returns the number of insights decayed, soft-deleted (both currently 0 as the
// query only reports success — real counts require a follow-up query).
func (s *ReflectionService) DecayInsights(ctx context.Context, weeksSince float64) (decayed int, softDeleted int, err error) {
	if err := s.queries.ApplyDecay(ctx, weeksSince); err != nil {
		return 0, 0, fmt.Errorf("failed to apply decay: %w", err)
	}
	slog.Info("insight decay applied", "weeks_since", weeksSince)
	return 0, 0, nil
}
