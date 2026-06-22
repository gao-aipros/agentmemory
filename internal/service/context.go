package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AssembledContext holds the raw content from all 5 context source buckets
// before budget application and formatting.
type AssembledContext struct {
	Graph         string
	Lessons       string
	Observations  string
	Recap         string
	WorkingMemory string
}

// ContextService assembles context from 5 source buckets for injection
// into the agent's prompt at session start, pre-tool-use, and pre-compact hooks.
type ContextService struct {
	queries   *store.Queries
	searchSvc *SearchService
	slotSvc   *SlotService
}

// NewContextService creates a new ContextService.
func NewContextService(pool *pgxpool.Pool, embedSvc *EmbeddingService, slotSvc *SlotService) *ContextService {
	return &ContextService{
		queries:   store.New(pool),
		searchSvc: NewSearchService(pool, embedSvc),
		slotSvc:   slotSvc,
	}
}

// AssembleContext gathers all 5 context source buckets for a user.
// Buckets:
// 1. Relevant observations — from recent sessions via search
// 2. Session recap — from session_summaries, if recent session exists
// 3. Relevant lessons — from lessons table
// 4. Graph neighbors — from graph traversal
// 5. Working memory — from memory slots
func (s *ContextService) AssembleContext(ctx context.Context, userID string) (*AssembledContext, error) {
	assembled := &AssembledContext{}

	// 1. Gather relevant observations from recent sessions
	observations, err := s.gatherObservations(ctx, userID)
	if err != nil {
		observations = ""
	}
	assembled.Observations = observations

	// 2. Gather session recap from recent session summaries
	recap, err := s.gatherRecap(ctx, userID)
	if err != nil {
		recap = ""
	}
	assembled.Recap = recap

	// 3. Gather relevant lessons
	lessons, err := s.gatherLessons(ctx, userID)
	if err != nil {
		lessons = ""
	}
	assembled.Lessons = lessons

	// 4. Gather graph neighbors
	graph, err := s.gatherGraphNeighbors(ctx, userID)
	if err != nil {
		graph = ""
	}
	assembled.Graph = graph

	// 5. Gather working memory from slots
	workingMemory, err := s.gatherWorkingMemory(ctx)
	if err != nil {
		workingMemory = ""
	}
	assembled.WorkingMemory = workingMemory

	return assembled, nil
}

// gatherObservations finds recent relevant observations for the user.
// Uses a single JOIN query to avoid N+1 per-session fetches.
func (s *ContextService) gatherObservations(ctx context.Context, userID string) (string, error) {
	observations, err := s.queries.ListObservationsByUserID(ctx, store.ListObservationsByUserIDParams{
		UserID: userID,
		Limit:  20,
	})
	if err != nil || len(observations) == 0 {
		return "", err
	}

	var parts []string
	for i, o := range observations {
		if i >= 20 {
			break
		}
		ts := ""
		if o.Timestamp.Valid {
			ts = o.Timestamp.Time.Format("2006-01-02")
		}
		parts = append(parts, fmt.Sprintf("[%s] %s: %s",
			o.ID, ts, truncate(o.Narrative, 150)))
	}

	return strings.Join(parts, "\n"), nil
}

// gatherRecap finds recent session summaries.
// Uses a single JOIN query to avoid N+1 per-session fetches.
func (s *ContextService) gatherRecap(ctx context.Context, userID string) (string, error) {
	summaries, err := s.queries.ListSummariesByUserID(ctx, store.ListSummariesByUserIDParams{
		UserID: userID,
		Limit:  3,
	})
	if err != nil || len(summaries) == 0 {
		return "", err
	}

	var parts []string
	for _, sum := range summaries {
		parts = append(parts, fmt.Sprintf("[session %s] %s",
			sum.SessionID[:8], truncate(sum.SummaryText, 200)))
	}

	return strings.Join(parts, "\n"), nil
}

// gatherLessons finds lessons relevant to the user's team.
func (s *ContextService) gatherLessons(ctx context.Context, userID string) (string, error) {
	// Get the user's team for scoping
	team, err := s.queries.GetUserTeam(ctx, userID)
	if err != nil {
		// No team — user only sees their own lessons (none at team scope)
		return "", nil
	}

	lessons, err := s.queries.ListLessonsByTeam(ctx, &team.ID)
	if err != nil {
		return "", err
	}

	var parts []string
	now := time.Now()
	for _, l := range lessons {
		created := "unknown"
		if l.CreatedAt.Valid {
			created = l.CreatedAt.Time.Format("2006-01-02")
			// Decay check: skip low-confidence lessons older than 30 days
			age := now.Sub(l.CreatedAt.Time)
			if age > 30*24*time.Hour && l.Confidence < 0.3 {
				continue
			}
		}
		parts = append(parts, fmt.Sprintf("[%s] %s (confidence: %.2f, last: %s): %s",
			l.ID, created, l.Confidence, created, truncate(l.Content, 200)))
	}

	return strings.Join(parts, "\n"), nil
}

// gatherGraphNeighbors traverses the graph from the user's recent observations.
// Scoped to userID for cross-tenant isolation.
// Uses batch fetch to avoid N+1 for observation details.
func (s *ContextService) gatherGraphNeighbors(ctx context.Context, userID string) (string, error) {
	seedIds, err := s.getRecentObservationIDsForUser(ctx, userID, 10)
	if err != nil || len(seedIds) == 0 {
		return "", err
	}

	traversed, err := s.queries.GraphTraversal(ctx, store.GraphTraversalParams{
		Column1:     seedIds,
		OwnerUserID: &userID,
	})
	if err != nil {
		return "", err
	}

	// Batch-fetch all observation details
	traversedIDs := make([]string, len(traversed))
	for i, gt := range traversed {
		traversedIDs[i] = gt.ID
	}

	obsByID := make(map[string]store.Observation)
	if batchObs, err := s.queries.GetObservationsByIDs(ctx, traversedIDs); err == nil {
		for _, o := range batchObs {
			obsByID[o.ID] = o
		}
	}

	var parts []string
	for _, gt := range traversed {
		obs, ok := obsByID[gt.ID]
		if !ok {
			continue
		}
		ts := ""
		if obs.Timestamp.Valid {
			ts = obs.Timestamp.Time.Format("2006-01-02")
		}
		parts = append(parts, fmt.Sprintf("[%s] %s: %s (graph_score: %.2f)",
			obs.ID, ts, truncate(obs.Narrative, 120), gt.GraphScore))
		if len(parts) >= 10 {
			break
		}
	}

	return strings.Join(parts, "\n"), nil
}

// gatherWorkingMemory retrieves working memory from memory slots.
func (s *ContextService) gatherWorkingMemory(ctx context.Context) (string, error) {
	if s.slotSvc == nil {
		return "", nil
	}
	content, err := s.slotSvc.GetSlot(ctx, "working_memory")
	if err != nil {
		return "", nil
	}
	return truncate(content, 300), nil
}

// getRecentObservationIDsForUser returns IDs of the most recent observations for a specific user.
// Uses ListObservationsByUserID which joins sessions to filter by user_id.
func (s *ContextService) getRecentObservationIDsForUser(ctx context.Context, userID string, limit int) ([]string, error) {
	recentObs, err := s.queries.ListObservationsByUserID(ctx, store.ListObservationsByUserIDParams{
		UserID: userID,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(recentObs))
	for i, obs := range recentObs {
		ids[i] = obs.ID
	}
	return ids, nil
}
