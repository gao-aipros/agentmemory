package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SummarizationService gathers session observations, chunks them if needed,
// calls the LLM for summarization, and stores the SessionSummary.
type SummarizationService struct {
	queries    *store.Queries
	llmService *LLMService
}

// NewSummarizationService creates a new SummarizationService.
func NewSummarizationService(pool *pgxpool.Pool, llm *LLMService) *SummarizationService {
	return &SummarizationService{
		queries:    store.New(pool),
		llmService: llm,
	}
}

// SummarizeSession gathers all observations for a session, calls the LLM
// for summarization, and upserts the result as a SessionSummary.
func (s *SummarizationService) SummarizeSession(ctx context.Context, sessionID string) error {
	// Gather observations
	observations, err := s.queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: sessionID,
		Limit:     10000,
	})
	if err != nil {
		return fmt.Errorf("failed to list observations: %w", err)
	}

	if len(observations) == 0 {
		slog.Info("no observations to summarize", "session_id", sessionID)
		return nil
	}

	// Convert to lightweight summarize views
	views := make([]SummarizeObservation, len(observations))
	for i, obs := range observations {
		facts := ""
		if obs.Facts != nil {
			facts = *obs.Facts
		}
		views[i] = SummarizeObservation{
			Title:     obs.Title,
			Narrative: obs.Narrative,
			Facts:     facts,
			Concepts:  obs.Concepts,
		}
	}

	// Chunk if needed (token budget: ~3000 tokens for prompt)
	chunks := ChunkObservations(views, 3000)

	// Summarize each chunk and combine
	allConcepts := make(map[string]bool)
	var combinedSummary string

	for i, chunk := range chunks {
		prompt := BuildSummarizePrompt(chunk)
		response, err := s.llmService.Call(ctx, prompt)
		if err != nil {
			return fmt.Errorf("LLM summarization failed for chunk %d: %w", i, err)
		}
		if i > 0 {
			combinedSummary += "\n"
		}
		combinedSummary += response
	}

	// Collect all concepts across all observations
	for _, obs := range observations {
		for _, c := range obs.Concepts {
			allConcepts[c] = true
		}
	}

	conceptsList := make([]string, 0, len(allConcepts))
	for c := range allConcepts {
		conceptsList = append(conceptsList, c)
	}

	// Upsert session summary
	summaryID := uuid.New().String()
	_, err = s.queries.UpsertSessionSummary(ctx, store.UpsertSessionSummaryParams{
		ID:          summaryID,
		SessionID:   sessionID,
		Visibility:  "private",
		SummaryText: combinedSummary,
		Concepts:    conceptsList,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert session summary: %w", err)
	}

	slog.Info("session summarized", "session_id", sessionID)
	return nil
}

// ConsolidationService extracts memories and lessons from a session summary.
type ConsolidationService struct {
	queries    *store.Queries
	llmService *LLMService
	mode       ConsolidationMode
}

// NewConsolidationService creates a new ConsolidationService.
func NewConsolidationService(pool *pgxpool.Pool, llm *LLMService, mode ConsolidationMode) *ConsolidationService {
	return &ConsolidationService{
		queries:    store.New(pool),
		llmService: llm,
		mode:       mode,
	}
}

// ConsolidateSession extracts memories and lessons from the session summary.
func (s *ConsolidationService) ConsolidateSession(ctx context.Context, sessionID string) error {
	// Get the session summary
	summary, err := s.queries.GetSessionSummary(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session summary: %w", err)
	}

	// Get observation count
	observations, err := s.queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: sessionID,
		Limit:     10000,
	})
	if err != nil {
		return fmt.Errorf("failed to list observations: %w", err)
	}

	// Build consolidation prompt
	input := &ConsolidationInput{
		SessionID:    sessionID,
		SummaryText:  summary.SummaryText,
		Concepts:     summary.Concepts,
		Observations: len(observations),
	}

	prompt := BuildConsolidationPrompt(input)
	response, err := s.llmService.Call(ctx, prompt)
	if err != nil {
		return fmt.Errorf("LLM consolidation failed: %w", err)
	}

	result, err := ParseConsolidationResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse consolidation response: %w", err)
	}

	// Store extracted memories
	for _, m := range result.Memories {
		memID := uuid.New().String()
		visibility := "private"
		if s.mode.Visibility == VisibilityTeam {
			visibility = "team"
		} else if s.mode.Visibility == VisibilityPublic {
			visibility = "public"
		}

		_, err := s.queries.InsertMemory(ctx, store.InsertMemoryParams{
			ID:          memID,
			OwnerType:   "user",
			OwnerUserID: nilString(s.mode.OwnerUserID),
			OwnerTeamID: nilString(s.mode.OwnerTeamID),
			Visibility:  visibility,
			Content:     m.Content,
			Concepts:    m.Concepts,
			Source:      "consolidation",
			Confidence:  0.5,
		})
		if err != nil {
			slog.Warn("failed to insert memory", "error", err)
		}
	}

	// Store extracted lessons
	for _, l := range result.Lessons {
		lessonID := uuid.New().String()
		_, err := s.queries.InsertLesson(ctx, store.InsertLessonParams{
			ID:         lessonID,
			TeamID:     nilString(s.mode.OwnerTeamID),
			Visibility: "team",
			Content:    l.Content,
			Context:    nilString(l.Context),
			Confidence: 0.5,
			Source:     "consolidation",
		})
		if err != nil {
			slog.Warn("failed to insert lesson", "error", err)
		}
	}

	slog.Info("session consolidated",
		"session_id", sessionID,
		"memories", len(result.Memories),
		"lessons", len(result.Lessons),
	)
	return nil
}

// ReflectionService handles periodic reflection: clustering memories,
// detecting patterns, and synthesizing insights.
type ReflectionService struct {
	queries *store.Queries
	// timerInterval is the interval between reflection runs
	timerInterval int // seconds
}

// NewReflectionService creates a new ReflectionService.
func NewReflectionService(pool *pgxpool.Pool, timerIntervalSeconds int) *ReflectionService {
	if timerIntervalSeconds <= 0 {
		timerIntervalSeconds = 3600 // default 1 hour
	}
	return &ReflectionService{
		queries:       store.New(pool),
		timerInterval: timerIntervalSeconds,
	}
}

// TriggerTimerCheck is called after consolidation to check if reflection
// should run based on the timer interval.
func (s *ReflectionService) TriggerTimerCheck() {
	slog.Debug("reflection timer check triggered")
	// In MVP, reflection runs on-demand or via timer.
	// Full periodic scheduling will be implemented in a future phase.
}
