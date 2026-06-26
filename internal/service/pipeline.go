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

// SetOwner sets the user and team IDs that own the consolidated memories and lessons.
func (s *ConsolidationService) SetOwner(userID, teamID string) {
	s.mode.OwnerUserID = userID
	s.mode.OwnerTeamID = teamID
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

	// Compute effective owner IDs (nil when empty, to satisfy FK constraints).
	var ownerUserID, ownerTeamID *string
	if s.mode.OwnerUserID != "" {
		ownerUserID = &s.mode.OwnerUserID
	}
	if s.mode.OwnerTeamID != "" {
		ownerTeamID = &s.mode.OwnerTeamID
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
				OwnerUserID: ownerUserID,
				OwnerTeamID: ownerTeamID,
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
				TeamID:     ownerTeamID,
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

// reflectLLM is the LLM interface used by ReflectionService.
// *LLMService satisfies this interface; mocks are used in tests.
type reflectLLM interface {
	Call(ctx context.Context, prompt string) (string, error)
}

// reflectionQuerier is the subset of database operations needed by ReflectionService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type reflectionQuerier interface {
	ListAllMemories(ctx context.Context, limit int32) ([]store.Memory, error)
	UpsertInsight(ctx context.Context, params store.UpsertInsightParams) error
	MarkMemoriesReflected(ctx context.Context, ids []string) error
	ApplyDecayWithCounts(ctx context.Context, weeksSince float64) (store.ApplyDecayWithCountsRow, error)
	ListInsights(ctx context.Context, arg store.ListInsightsParams) ([]store.ListInsightsRow, error)
	SearchInsights(ctx context.Context, arg store.SearchInsightsParams) ([]store.SearchInsightsRow, error)
}

// ReflectionService handles periodic reflection: clustering memories,
// detecting patterns, and synthesizing insights.
type ReflectionService struct {
	queries       reflectionQuerier
	timerInterval int // seconds
	llm           reflectLLM
}

// NewReflectionService creates a new ReflectionService.
func NewReflectionService(pool *pgxpool.Pool, timerIntervalSeconds int, llm reflectLLM) *ReflectionService {
	if timerIntervalSeconds <= 0 {
		timerIntervalSeconds = 3600
	}
	return &ReflectionService{
		queries:       store.New(pool),
		timerInterval: timerIntervalSeconds,
		llm:           llm,
	}
}

// newReflectionServiceWithQuerier creates a ReflectionService with a custom querier (for testing).
func newReflectionServiceWithQuerier(q reflectionQuerier, llm reflectLLM) *ReflectionService {
	return &ReflectionService{
		queries:       q,
		timerInterval: 3600,
		llm:           llm,
	}
}

// RunReflection fetches memories from the database, groups them by shared concepts,
// builds LLM prompts for qualifying clusters (3+ memories), parses the LLM response
// into insights, and persists them via UpsertInsight with content-based fingerprint IDs.
// Clusters with fewer than 3 memories are skipped. maxClusters caps the number of
// clusters processed via LLM. On LLM failure the cluster is skipped with a WARN log.
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
	clusters := GroupMemoriesByConcept(reflectionMemories)

	// Cap at maxClusters (clusters fed to the LLM, not including skipped small ones)
	clustersProcessed := 0
	insightCount := 0
	skippedClusters := 0
	var totalClusters int
	var failedClusters int

	for _, cluster := range clusters {
		// Enforce maxClusters cap
		if maxClusters > 0 && clustersProcessed >= maxClusters {
			break
		}

		// Skip clusters with fewer than 3 memories (not enough signal)
		if len(cluster.Memories) < 3 {
			concepts := uniqueConcepts(cluster.Memories)
			slog.Debug("skipping small cluster",
				"memory_count", len(cluster.Memories),
				"concepts", concepts,
			)
			skippedClusters++
			continue
		}

		totalClusters++
		clustersProcessed++

		// Convert cluster memories to a ReflectCluster for the prompt
		concepts := uniqueConcepts(cluster.Memories)
		facts := memoriesToFacts(cluster.Memories)
		reflectCluster := ReflectCluster{
			Concepts: concepts,
			Facts:    facts,
		}

		prompt := BuildReflectPrompt(reflectCluster)
		response, err := s.llm.Call(ctx, prompt)
		if err != nil {
			slog.Warn("LLM reflection failed for cluster, skipping",
				"error", err,
				"concepts", concepts,
			)
			failedClusters++
			continue
		}

		// Parse LLM response into structured insights
		insights := ParseReflectResponse(response)

		// Cap at 5 insights per cluster (per spec FR-014)
		if len(insights) > 5 {
			insights = insights[:5]
		}

		// Collect memory IDs for MarkMemoriesReflected
		memoryIDs := make([]string, len(cluster.Memories))
		for i, m := range cluster.Memories {
			memoryIDs[i] = m.ID
		}

		// Project pointer for UpsertInsight
		var projectPtr *string
		if project != "" {
			projectPtr = &project
		}

		// Persist each parsed insight
		persistedCount := 0
		for _, insight := range insights {
			id := InsightFingerprint(insight.Content)
			if err := s.queries.UpsertInsight(ctx, store.UpsertInsightParams{
				ID:                   id,
				Title:                insight.Title,
				Content:              insight.Content,
				Confidence:           insight.Confidence,
				SourceConceptCluster: concepts,
				SourceMemoryIds:      memoryIDs,
				SourceLessonIds:      []string{},
				Project:              projectPtr,
				Tags:                 []string{},
			}); err != nil {
				slog.Warn("failed to persist insight", "error", err)
				continue
			}
			insightCount++
			persistedCount++
		}

		// Track cluster failure if no insights were persisted
		if persistedCount == 0 {
			failedClusters++
		}

		// Mark memories in this cluster as reflected only if at least one insight was persisted
		if persistedCount > 0 {
			if err := s.queries.MarkMemoriesReflected(ctx, memoryIDs); err != nil {
				slog.Warn("failed to mark memories reflected", "error", err)
			}
		}
	}

	slog.Info("reflection complete",
		"memories", len(memories),
		"clusters", len(clusters),
		"insights", insightCount,
		"skipped_clusters", skippedClusters,
	)

	// If all clusters failed, return an error so the caller can distinguish
	// total failure from success. This is required by CLAUDE.md Rule 12.
	if totalClusters > 0 && failedClusters == totalClusters {
		return fmt.Errorf("all %d reflection clusters failed", totalClusters)
	}

	// Partial failure is logged but not returned as an error -- some clusters
	// may have succeeded, which is acceptable.
	if failedClusters > 0 {
		slog.Warn("partial reflection failure", "failed", failedClusters, "total", totalClusters)
	}

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

// ListInsights returns insights filtered by project and minimum confidence, ordered by confidence descending.
func (s *ReflectionService) ListInsights(ctx context.Context, project string, minConfidence float64, limit int32) ([]store.Insight, error) {
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}
	var mc *float64
	if minConfidence > 0 {
		mc = &minConfidence
	}

	rows, err := s.queries.ListInsights(ctx, store.ListInsightsParams{
		Limit:         limit,
		Project:       projectPtr,
		MinConfidence: mc,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list insights: %w", err)
	}

	insights := make([]store.Insight, len(rows))
	for i, r := range rows {
		insights[i] = store.Insight{
			ID:                 r.ID,
			Title:              r.Title,
			Content:            r.Content,
			Confidence:         r.Confidence,
			ReinforcementCount: r.ReinforcementCount,
			Project:            r.Project,
			Tags:               r.Tags,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
		}
	}
	return insights, nil
}

// SearchInsights performs a full-text search on insights, filtered by project and minimum confidence.
func (s *ReflectionService) SearchInsights(ctx context.Context, project string, query string, minConfidence float64, limit int32) ([]store.Insight, error) {
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}
	var mc *float64
	if minConfidence > 0 {
		mc = &minConfidence
	}
	var queryPtr *string
	if query != "" {
		queryPtr = &query
	}

	rows, err := s.queries.SearchInsights(ctx, store.SearchInsightsParams{
		Limit:         limit,
		Project:       projectPtr,
		MinConfidence: mc,
		Query:         queryPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search insights: %w", err)
	}

	insights := make([]store.Insight, len(rows))
	for i, r := range rows {
		insights[i] = store.Insight{
			ID:                 r.ID,
			Title:              r.Title,
			Content:            r.Content,
			Confidence:         r.Confidence,
			ReinforcementCount: r.ReinforcementCount,
			Project:            r.Project,
			Tags:               r.Tags,
			CreatedAt:          r.CreatedAt,
			UpdatedAt:          r.UpdatedAt,
		}
	}
	return insights, nil
}

// uniqueConcepts returns the deduplicated, lowercased list of concepts across all memories.
func uniqueConcepts(memories []MemoryForReflection) []string {
	seen := make(map[string]bool)
	var result []string
	for _, m := range memories {
		for _, c := range m.Concepts {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" && !seen[c] {
				seen[c] = true
				result = append(result, c)
			}
		}
	}
	return result
}

// memoriesToFacts converts each memory's content into a FactRef with base confidence 0.5.
func memoriesToFacts(memories []MemoryForReflection) []FactRef {
	facts := make([]FactRef, 0, len(memories))
	for _, m := range memories {
		facts = append(facts, FactRef{
			Fact:       m.Content,
			Confidence: 0.5,
		})
	}
	return facts
}

