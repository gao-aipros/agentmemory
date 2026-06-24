package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// summarizationQuerier is the subset of *store.Queries methods used by SummarizationService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type summarizationQuerier interface {
	ListObservationsBySession(ctx context.Context, params store.ListObservationsBySessionParams) ([]store.Observation, error)
	UpsertSessionSummary(ctx context.Context, params store.UpsertSessionSummaryParams) (store.SessionSummary, error)
}

// SummarizationService gathers session observations, chunks them if needed,
// calls the LLM for summarization, and stores the SessionSummary.
type SummarizationService struct {
	queries    summarizationQuerier
	llmService *LLMService
}

// newSummarizationServiceWithQuerier creates a SummarizationService with a custom querier (for testing).
func newSummarizationServiceWithQuerier(q summarizationQuerier, llm *LLMService) *SummarizationService {
	return &SummarizationService{
		queries:    q,
		llmService: llm,
	}
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
		if strings.TrimSpace(response) == "" {
			slog.Warn("LLM returned empty response for chunk", "chunk", i)
			continue
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

	// Store extracted memories via batch insert (avoids N+1 writes).
	if len(result.Memories) > 0 {
		visibility := "private"
		if s.mode.Visibility == VisibilityTeam {
			visibility = "team"
		} else if s.mode.Visibility == VisibilityPublic {
			visibility = "public"
		}
		memRows := make([]store.BatchInsertMemoriesParams, 0, len(result.Memories))
		for _, m := range result.Memories {
			memRows = append(memRows, store.BatchInsertMemoriesParams{
				ID:          uuid.New().String(),
				OwnerType:   "user",
				OwnerUserID: &s.mode.OwnerUserID,
				OwnerTeamID: &s.mode.OwnerTeamID,
				Visibility:  visibility,
				Content:     m.Content,
				Concepts:    m.Concepts,
				Source:      "consolidation",
				Confidence:  0.5,
			})
		}
		if len(memRows) > 0 {
			if _, err := s.queries.BatchInsertMemories(ctx, memRows); err != nil {
				slog.Warn("failed to batch insert memories", "error", err)
			}
		}
	}

	// Store extracted lessons via batch insert (avoids N+1 writes).
	if len(result.Lessons) > 0 {
		lessonRows := make([]store.BatchInsertLessonsParams, 0, len(result.Lessons))
		for _, l := range result.Lessons {
			lessonRows = append(lessonRows, store.BatchInsertLessonsParams{
				ID:         uuid.New().String(),
				TeamID:     &s.mode.OwnerTeamID,
				Visibility: "team",
				Content:    l.Content,
				Context:    &l.Context,
				Confidence: 0.5,
				Source:     "consolidation",
			})
		}
		if len(lessonRows) > 0 {
			if _, err := s.queries.BatchInsertLessons(ctx, lessonRows); err != nil {
				slog.Warn("failed to batch insert lessons", "error", err)
			}
		}
	}

	slog.Info("session consolidated",
		"session_id", sessionID,
		"memories", len(result.Memories),
		"lessons", len(result.Lessons),
	)
	return nil
}

// reflectionQuerier is the subset of database operations needed by ReflectionService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type reflectionQuerier interface {
	ListAllMemories(ctx context.Context, limit int32) ([]store.Memory, error)
	InsertInsight(ctx context.Context, params store.InsertInsightParams) error
}

// ReflectionService handles periodic reflection: clustering memories,
// detecting patterns, and synthesizing insights.
type ReflectionService struct {
	queries       reflectionQuerier
	timerInterval int // seconds
}

// NewReflectionService creates a new ReflectionService.
func NewReflectionService(pool *pgxpool.Pool, timerIntervalSeconds int) *ReflectionService {
	if timerIntervalSeconds <= 0 {
		timerIntervalSeconds = 3600
	}
	return &ReflectionService{
		queries:       store.New(pool),
		timerInterval: timerIntervalSeconds,
	}
}

// newReflectionServiceWithQuerier creates a ReflectionService with a custom querier (for testing).
func newReflectionServiceWithQuerier(q reflectionQuerier) *ReflectionService {
	return &ReflectionService{
		queries:       q,
		timerInterval: 3600,
	}
}

// RunReflection fetches memories from the database, groups them by shared concepts,
// detects patterns, synthesizes insights, and persists them.
func (s *ReflectionService) RunReflection(ctx context.Context, project string, maxClusters int) error {
	// Fetch memories (up to 1000)
	memories, err := s.queries.ListAllMemories(ctx, 1000)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	if len(memories) == 0 {
		slog.Info("no memories to reflect on")
		return nil
	}

	// Convert store.Memory → MemoryForReflection
	reflectionMemories := make([]MemoryForReflection, len(memories))
	for i, m := range memories {
		reflectionMemories[i] = MemoryForReflection{
			ID:       m.ID,
			Content:  m.Content,
			Concepts: m.Concepts,
		}
	}

	// Cluster memories by shared concepts
	// Note: project and maxClusters are accepted for future LLM-based reflection
	// but not yet used by the pure clustering functions.
	clusters := GroupMemoriesByConcept(reflectionMemories)

	// Detect patterns and synthesize insights for each cluster
	var allInsights []SynthesizedInsight
	for _, cluster := range clusters {
		patterns := DetectPatterns(cluster)
		insights := SynthesizeInsights(patterns)
		allInsights = append(allInsights, insights...)
	}

	// Persist each insight
	for _, insight := range allInsights {
		if err := s.queries.InsertInsight(ctx, store.InsertInsightParams{
				ID:         uuid.New().String(),
				Content:    insight.Content,
				Confidence: insight.Confidence,
			}); err != nil {
			slog.Warn("failed to persist insight", "error", err)
		}
	}

	slog.Info("reflection complete",
		"memories", len(memories),
		"clusters", len(clusters),
		"insights", len(allInsights),
	)
	return nil
}

// TriggerTimerCheck is called after consolidation to check if reflection
// should run based on the timer interval. In MVP, it runs reflection on every call.
func (s *ReflectionService) TriggerTimerCheck(ctx context.Context) {
	slog.Debug("reflection timer check triggered")
	if err := s.RunReflection(ctx, "", 10); err != nil {
		slog.Warn("reflection run failed", "error", err)
	}
}

