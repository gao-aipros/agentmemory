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
	pool       *pgxpool.Pool // for batch inserts
}

// NewConsolidationService creates a new ConsolidationService.
func NewConsolidationService(pool *pgxpool.Pool, llm *LLMService, mode ConsolidationMode) *ConsolidationService {
	return &ConsolidationService{
		queries:    store.New(pool),
		llmService: llm,
		mode:       mode,
		pool:       pool,
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
		memories := make([]memoryRow, 0, len(result.Memories))
		for _, m := range result.Memories {
			memories = append(memories, memoryRow{
				id:         uuid.New().String(),
				content:    m.Content,
				concepts:   m.Concepts,
				source:     "consolidation",
				confidence: 0.5,
			})
		}
		if err := s.batchInsertMemories(ctx, memories, s.mode.OwnerUserID, s.mode.OwnerTeamID, visibility); err != nil {
			slog.Warn("failed to batch insert memories", "error", err)
		}
	}

	// Store extracted lessons via batch insert (avoids N+1 writes).
	if len(result.Lessons) > 0 {
		lessons := make([]lessonRow, 0, len(result.Lessons))
		for _, l := range result.Lessons {
			lessons = append(lessons, lessonRow{
				id:         uuid.New().String(),
				teamID:     s.mode.OwnerTeamID,
				visibility: "team",
				content:    l.Content,
				context:    l.Context,
				confidence: 0.5,
				source:     "consolidation",
			})
		}
		if err := s.batchInsertLessons(ctx, lessons); err != nil {
			slog.Warn("failed to batch insert lessons", "error", err)
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

// memoryRow and lessonRow are row types for batch inserts, avoiding N+1 writes.
type memoryRow struct {
	id         string
	content    string
	concepts   []string
	source     string
	confidence float64
}

type lessonRow struct {
	id         string
	teamID     string
	visibility string
	content    string
	context    string
	confidence float64
	source     string
}

// batchInsertMemories inserts multiple memories in a single multi-row INSERT.
func (s *ConsolidationService) batchInsertMemories(ctx context.Context, rows []memoryRow, ownerUserID, ownerTeamID, visibility string) error {
	if len(rows) == 0 {
		return nil
	}
	if s.pool == nil {
		return fmt.Errorf("consolidation service pool is nil")
	}

	values := make([]string, len(rows))
	args := make([]any, 0, len(rows)*9)

	for i, r := range rows {
		base := i * 9
		values[i] = fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9,
		)
		args = append(args,
			r.id, "user", ownerUserID, ownerTeamID,
			visibility, r.content, r.concepts, r.source, r.confidence,
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO memories (id, owner_type, owner_user_id, owner_team_id, visibility, content, concepts, source, confidence)
		 VALUES %s`,
		strings.Join(values, ", "),
	)

	_, err := s.pool.Exec(ctx, query, args...)
	return err
}

// batchInsertLessons inserts multiple lessons in a single multi-row INSERT.
func (s *ConsolidationService) batchInsertLessons(ctx context.Context, rows []lessonRow) error {
	if len(rows) == 0 {
		return nil
	}
	if s.pool == nil {
		return fmt.Errorf("consolidation service pool is nil")
	}

	values := make([]string, len(rows))
	args := make([]any, 0, len(rows)*8)

	for i, r := range rows {
		base := i * 8
		values[i] = fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,now())",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7,
		)
		args = append(args,
			r.id, r.teamID, r.visibility,
			r.content, r.context, r.confidence, r.source,
		)
	}

	query := fmt.Sprintf(
		`INSERT INTO lessons (id, team_id, visibility, content, context, confidence, source, created_at)
		 VALUES %s`,
		strings.Join(values, ", "),
	)

	_, err := s.pool.Exec(ctx, query, args...)
	return err
}
