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

// SlotManager is a minimal interface for reading memory slots.
type SlotManager struct {
	queries *store.Queries
}

// NewSlotManager creates a new SlotManager.
func NewSlotManager(pool *pgxpool.Pool) *SlotManager {
	return &SlotManager{
		queries: store.New(pool),
	}
}

// GetSlot returns the content of a named slot, or empty string if not found.
func (m *SlotManager) GetSlot(ctx context.Context, label string) (string, error) {
	// Slots are not yet persisted in the database schema.
	// This is a placeholder that returns empty for now.
	// Future: read from a memory_slots table.
	return "", nil
}

// ContextService assembles context from 5 source buckets for injection
// into the agent's prompt at session start, pre-tool-use, and pre-compact hooks.
type ContextService struct {
	queries   *store.Queries
	searchSvc *SearchService
	slotMgr   *SlotManager
}

// NewContextService creates a new ContextService.
func NewContextService(pool *pgxpool.Pool, embedSvc *EmbeddingService, slotMgr *SlotManager) *ContextService {
	return &ContextService{
		queries:   store.New(pool),
		searchSvc: NewSearchService(pool, embedSvc),
		slotMgr:   slotMgr,
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
	graph, err := s.gatherGraphNeighbors(ctx)
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
func (s *ContextService) gatherObservations(ctx context.Context, userID string) (string, error) {
	sessions, err := s.queries.ListSessionsByUser(ctx, userID)
	if err != nil || len(sessions) == 0 {
		return "", err
	}

	// Limit to 5 most recent sessions
	if len(sessions) > 5 {
		sessions = sessions[:5]
	}

	var parts []string
	for _, sess := range sessions {
		obs, err := s.queries.ListObservationsBySession(ctx, sess.ID)
		if err != nil {
			continue
		}
		for _, o := range obs {
			if len(parts) >= 20 {
				break
			}
			ts := ""
			if o.Timestamp.Valid {
				ts = o.Timestamp.Time.Format("2006-01-02")
			}
			parts = append(parts, fmt.Sprintf("[%s] %s: %s",
				o.ID, ts, truncate(o.Narrative, 150)))
		}
		if len(parts) >= 20 {
			break
		}
	}

	return strings.Join(parts, "\n"), nil
}

// gatherRecap finds recent session summaries.
func (s *ContextService) gatherRecap(ctx context.Context, userID string) (string, error) {
	sessions, err := s.queries.ListSessionsByUser(ctx, userID)
	if err != nil || len(sessions) == 0 {
		return "", err
	}

	// Limit to 3 most recent sessions
	if len(sessions) > 3 {
		sessions = sessions[:3]
	}

	var parts []string
	for _, sess := range sessions {
		summaries, err := s.queries.ListSummariesBySession(ctx, sess.ID)
		if err != nil {
			continue
		}
		for _, sum := range summaries {
			parts = append(parts, fmt.Sprintf("[session %s] %s",
				sess.ID[:8], truncate(sum.SummaryText, 200)))
		}
	}

	return strings.Join(parts, "\n"), nil
}

// gatherLessons finds relevant lessons.
func (s *ContextService) gatherLessons(ctx context.Context, userID string) (string, error) {
	lessons, err := s.queries.ListAllLessons(ctx, 10)
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

// gatherGraphNeighbors traverses the graph from recent observations.
func (s *ContextService) gatherGraphNeighbors(ctx context.Context) (string, error) {
	seedIds, err := s.getRecentObservationIDs(ctx, 10)
	if err != nil || len(seedIds) == 0 {
		return "", err
	}

	traversed, err := s.queries.GraphTraversal(ctx, seedIds)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, gt := range traversed {
		obs, err := s.queries.GetObservation(ctx, gt.ID)
		if err != nil {
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
	if s.slotMgr == nil {
		return "", nil
	}
	content, err := s.slotMgr.GetSlot(ctx, "working_memory")
	if err != nil {
		return "", nil
	}
	return truncate(content, 300), nil
}

// getRecentObservationIDs returns IDs of the most recent observations.
func (s *ContextService) getRecentObservationIDs(ctx context.Context, limit int) ([]string, error) {
	recentObs, err := s.queries.ListRecentObservations(ctx, int32(limit))
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(recentObs))
	for i, obs := range recentObs {
		ids[i] = obs.ID
	}
	return ids, nil
}
