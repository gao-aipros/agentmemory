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

const (
	// listAllMemories is a raw SQL query to fetch all memories ordered by recency.
	listAllMemories = `SELECT id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence, created_at FROM memories ORDER BY created_at DESC LIMIT $1`
	// insertInsight is a raw SQL query to persist a synthesized insight.
	insertInsight = `INSERT INTO insights (id, content, confidence, source, created_at) VALUES ($1, $2, $3, 'reflect', now())`
)

// reflectionQuerier is the subset of database operations needed by ReflectionService.
// The concrete reflectionQuerierImpl satisfies this interface, enabling mock-based unit testing.
type reflectionQuerier interface {
	ListMemories(ctx context.Context, limit int32) ([]store.Memory, error)
	InsertInsight(ctx context.Context, id string, content string, confidence float64) error
}

// reflectionQuerierImpl is the production implementation using a pgxpool.Pool.
type reflectionQuerierImpl struct {
	pool *pgxpool.Pool
}

// ListMemories fetches all memories from the database, ordered by recency, up to limit.
func (r *reflectionQuerierImpl) ListMemories(ctx context.Context, limit int32) ([]store.Memory, error) {
	rows, err := r.pool.Query(ctx, listAllMemories, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []store.Memory
	for rows.Next() {
		var i store.Memory
		if err := rows.Scan(
			&i.ID,
			&i.OwnerType,
			&i.OwnerUserID,
			&i.OwnerTeamID,
			&i.Visibility,
			&i.Content,
			&i.Concepts,
			&i.Source,
			&i.Confidence,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

// InsertInsight persists a synthesized insight to the insights table.
func (r *reflectionQuerierImpl) InsertInsight(ctx context.Context, id string, content string, confidence float64) error {
	_, err := r.pool.Exec(ctx, insertInsight, id, content, confidence)
	return err
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
		timerIntervalSeconds = 3600 // default 1 hour
	}
	return &ReflectionService{
		queries:       &reflectionQuerierImpl{pool: pool},
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
	memories, err := s.queries.ListMemories(ctx, 1000)
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
		if err := s.queries.InsertInsight(ctx, uuid.New().String(), insight.Content, insight.Confidence); err != nil {
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
