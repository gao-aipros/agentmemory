package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SchedulingFunc is a function that processes a batch for one scheduler tier.
type SchedulingFunc func(ctx context.Context) error

// SchedulerIntervals holds the interval durations for each scheduler tier.
type SchedulerIntervals struct {
	Compression    time.Duration
	Summarization  time.Duration
	Consolidation  time.Duration
	Reflection     time.Duration
}

// Scheduler runs periodic batch processing at configurable intervals.
// Each tier (compression, summarization, consolidation, reflection) runs in
// its own goroutine at its configured interval. Setting an interval to 0
// disables that tier.
type Scheduler struct {
	pool      *pgxpool.Pool
	llm       *LLMService
	embed     *EmbeddingService
	queries   *store.Queries
	intervals SchedulerIntervals

	consolidationSvc *ConsolidationService
	reflectionSvc    *ReflectionService
	profileSvc       *ProfileService

	// Overridable process functions (default to private methods, injected for testing).
	CompressionFunc   SchedulingFunc
	SummarizationFunc SchedulingFunc
	ConsolidationFunc SchedulingFunc
	ReflectionFunc    SchedulingFunc
}

// NewScheduler creates a new Scheduler with the given dependencies and intervals.
func NewScheduler(pool *pgxpool.Pool, llm *LLMService, embed *EmbeddingService, intervals SchedulerIntervals, profileSvc *ProfileService) *Scheduler {
	s := &Scheduler{
		pool:             pool,
		llm:              llm,
		embed:            embed,
		queries:          store.New(pool),
		intervals:        intervals,
		consolidationSvc: NewConsolidationService(pool, llm, DefaultConsolidationMode("member_choice", false)),
		reflectionSvc:    NewReflectionService(pool, 3600, llm),
		profileSvc:       profileSvc,
	}
	// Wire default process functions to the private implementations.
	s.CompressionFunc = s.ProcessCompression
	s.SummarizationFunc = s.ProcessSummarization
	s.ConsolidationFunc = s.ProcessConsolidation
	s.ReflectionFunc = s.ProcessReflection
	return s
}

// Start launches a goroutine for each scheduler tier.
func (s *Scheduler) Start(ctx context.Context) {
	if s.intervals.Compression > 0 {
		go s.runTier(ctx, s.intervals.Compression, s.CompressionFunc, "Tier0(compress)")
	}
	if s.intervals.Summarization > 0 {
		go s.runTier(ctx, s.intervals.Summarization, s.SummarizationFunc, "Tier1(summarize)")
	}
	if s.intervals.Consolidation > 0 {
		go s.runTier(ctx, s.intervals.Consolidation, s.ConsolidationFunc, "Tier2(consolidate)")
	}
	if s.intervals.Reflection > 0 {
		go s.runTier(ctx, s.intervals.Reflection, s.ReflectionFunc, "Tier3(reflect)")
	}
}

// runTier runs fn on every tick of a time.Ticker, skipping when fn returns nil.
func (s *Scheduler) runTier(ctx context.Context, interval time.Duration, fn SchedulingFunc, name string) {
	if interval <= 0 {
		return
	}
	slog.Debug("scheduler tier starting", "name", name, "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("scheduler tier stopped", "name", name, "reason", ctx.Err())
			return
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				slog.Warn("scheduler tier error", "name", name, "error", err)
			}
		}
	}
}

// CompressSessionNow immediately compresses all uncompressed observations for a
// session, bypassing the scheduler interval. Used by session-end handlers.
// Uses FOR UPDATE SKIP LOCKED to avoid duplicating work if scheduler is also
// processing the same session concurrently.
func (s *Scheduler) CompressSessionNow(ctx context.Context, sessionID string) error {
	return s.compressSession(ctx, sessionID)
}

// SummarizeSessionNow immediately summarizes a session, bypassing the scheduler
// interval. isFull=true when triggered by session-end, false for mid-session.
func (s *Scheduler) SummarizeSessionNow(ctx context.Context, sessionID string, isFull bool) error {
	return s.summarizeSession(ctx, sessionID, isFull)
}

// processCompression is the Tier 0 scheduler function: scans for sessions with
// uncompressed observations and batch-compresses them.
func (s *Scheduler) ProcessCompression(ctx context.Context) error {
	sessions, err := s.queries.ListSessionsWithUncompressedObservations(ctx)
	if err != nil {
		return err
	}
	for _, sessionID := range sessions {
		if err := s.compressSession(ctx, sessionID); err != nil {
			slog.Warn("compression: failed for session",
				"session_id", sessionID,
				"tier", "Tier0(compress)",
				"error", err,
			)
		}
	}
	return nil
}

// compressSession claims and compresses uncompressed observations for a session.
func (s *Scheduler) compressSession(ctx context.Context, sessionID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Claim uncompressed observations atomically
	obs, err := s.queries.WithTx(tx).ClaimUncompressedObservations(ctx, sessionID)
	if err != nil {
		return err
	}
	if len(obs) == 0 {
		return nil // already claimed by another caller
	}

	// TODO (US1): batch LLM call, batch embedding, insert compressed rows, UPDATE compressed_at
	qtx := s.queries.WithTx(tx)

	// Convert to ObservationForCompression for prompt building
	compObs := make([]ObservationForCompression, len(obs))
	for i, o := range obs {
		facts := ""
		if o.Facts != nil {
			facts = *o.Facts
		}
		compObs[i] = ObservationForCompression{
			Title:     o.Title,
			Narrative: o.Narrative,
			Facts:     facts,
			Concepts:  o.Concepts,
		}
	}

	// Build batch prompt and call LLM
	prompt := BuildBatchCompressionPrompt(compObs)
	response, err := s.llm.Call(ctx, prompt)

	var results []CompressionResult
	if err == nil {
		// Parse response
		results, err = ParseBatchCompressionResponse(response, len(obs))
	}

	if err != nil {
		slog.Warn("LLM compression failed, falling back to synthetic compression",
			"session_id", sessionID,
			"error", err,
		)
		results = make([]CompressionResult, len(obs))
		for i, o := range obs {
			sr := BuildSyntheticCompression(o.Title, o.Narrative, o.Title, o.Type, "", o.Files)
			results[i] = CompressionResult{
				CompressedText: sr.CompressedText,
				Concepts:       sr.Concepts,
			}
		}
	}

	// Build batch insert params — use min(len(obs), len(results))
	n := len(obs)
	if len(results) < n {
		n = len(results)
	}
	if n == 0 {
		return fmt.Errorf("no results to insert after batch compression")
	}
	insertParams := make([]store.BatchInsertCompressedObservationsParams, 0, n)
	for i := 0; i < n; i++ {
		insertParams = append(insertParams, store.BatchInsertCompressedObservationsParams{
			ID:             uuid.New().String(),
			ObservationIds: []string{obs[i].ID},
			SessionID:      sessionID,
			CompressedText: results[i].CompressedText,
			Concepts:       results[i].Concepts,
			OwnerType:      obs[i].OwnerType,
			OwnerUserID:    obs[i].OwnerUserID,
			Visibility:     obs[i].Visibility,
		})
	}

	// Batch insert via copyfrom
	if _, err := qtx.BatchInsertCompressedObservations(ctx, insertParams); err != nil {
		return fmt.Errorf("failed to batch insert compressed observations: %w", err)
	}

	// Batch embed all compressed texts
	texts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		texts = append(texts, results[i].CompressedText)
	}
	embeddings, err := s.embed.BatchEmbedDocuments(ctx, texts)
	if err != nil {
		slog.Warn("batch embedding failed for compressed observations",
			"session_id", sessionID,
			"error", err,
		)
	} else {
		for i, emb := range embeddings {
			vec := pgvector.NewVector(emb)
			if err := qtx.InsertCompressedEmbedding(ctx, store.InsertCompressedEmbeddingParams{
				CompressedID: insertParams[i].ID,
				Embedding:    &vec,
				Model:        "default",
			}); err != nil {
				slog.Warn("failed to store compressed embedding",
					"compressed_id", insertParams[i].ID,
					"error", err,
				)
			}
		}
	}

	// Mark observations as compressed
	obsIDs := make([]string, len(obs))
	for i, o := range obs {
		obsIDs[i] = o.ID
	}
	if err := qtx.MarkObservationsCompressed(ctx, obsIDs); err != nil {
		return fmt.Errorf("failed to mark observations compressed: %w", err)
	}

	return tx.Commit(ctx)
}

// summarizeSession updates a session summary incrementally.
// isFull=true when triggered by session-end (all observations), false for mid-session scheduler runs.
func (s *Scheduler) summarizeSession(ctx context.Context, sessionID string, isFull bool) error {
	// Fetch existing summary if any (for incremental support)
	var existingSummaryText string
	var lastSummaryTime time.Time
	existingSummary, err := s.queries.GetSessionSummary(ctx, sessionID)
	if err == nil {
		existingSummaryText = existingSummary.SummaryText
		lastSummaryTime = existingSummary.CreatedAt.Time
	}

	// Fetch all observations for the session
	allObs, err := s.queries.ListObservationsBySession(ctx, store.ListObservationsBySessionParams{
		SessionID: sessionID,
		Limit:     10000,
	})
	if err != nil {
		return fmt.Errorf("failed to list observations: %w", err)
	}
	if len(allObs) == 0 {
		slog.Info("no observations to summarize", "session_id", sessionID)
		return nil
	}

	// Determine which observations to include
	var targetObs []store.Observation
	if existingSummaryText != "" && !isFull {
		// Incremental: only observations newer than the last summary
		for _, obs := range allObs {
			if obs.CreatedAt.Time.After(lastSummaryTime) {
				targetObs = append(targetObs, obs)
			}
		}
	} else {
		// Full: all observations
		targetObs = allObs
	}
	if len(targetObs) == 0 {
		slog.Debug("no new observations since last summary", "session_id", sessionID)
		return nil
	}

	// Convert to SummarizeObservation views
	views := make([]SummarizeObservation, len(targetObs))
	for i, obs := range targetObs {
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

	// Collect all concepts from all observations (not just new ones for full)
	allConcepts := make(map[string]bool)
	for _, obs := range allObs {
		for _, c := range obs.Concepts {
			allConcepts[c] = true
		}
	}
	conceptsList := make([]string, 0, len(allConcepts))
	for c := range allConcepts {
		conceptsList = append(conceptsList, c)
	}

	// Build prompt and call LLM
	var summaryText string
	if existingSummaryText != "" && !isFull {
		// Incremental summarization: use existing summary + new observations
		prompt := BuildIncrementalSummarizePrompt(existingSummaryText, views)
		response, err := s.llm.Call(ctx, prompt)
		if err != nil {
			return fmt.Errorf("LLM incremental summarization failed: %w", err)
		}
		summaryText = strings.TrimSpace(response)
		if summaryText == "" {
			slog.Warn("LLM returned empty incremental summary", "session_id", sessionID)
			return nil
		}
	} else {
		// Full summarization with chunking
		chunks := ChunkObservations(views, 3000)
		for i, chunk := range chunks {
			prompt := BuildSummarizePrompt(chunk)
			response, err := s.llm.Call(ctx, prompt)
			if err != nil {
				return fmt.Errorf("LLM summarization failed for chunk %d: %w", i, err)
			}
			if strings.TrimSpace(response) == "" {
				slog.Warn("LLM returned empty response for chunk", "chunk", i)
				continue
			}
			if summaryText != "" {
				summaryText += "\n"
			}
			summaryText += response
		}
	}
	if summaryText == "" {
		slog.Warn("no summary text generated", "session_id", sessionID)
		return nil
	}

	// Upsert with is_full flag
	summaryID := uuid.New().String()
	_, err = s.queries.UpsertSessionSummary(ctx, store.UpsertSessionSummaryParams{
		ID:          summaryID,
		SessionID:   sessionID,
		Visibility:  "private",
		SummaryText: summaryText,
		Concepts:    conceptsList,
		IsFull:      isFull,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert session summary: %w", err)
	}

	slog.Info("session summarized",
		"session_id", sessionID,
		"is_full", isFull,
	)
	return nil
}

// processSummarization scans for sessions needing summarization and summarizes them.
func (s *Scheduler) ProcessSummarization(ctx context.Context) error {
	sessions, err := s.queries.ListSessionsNeedingSummarization(ctx)
	if err != nil {
		return err
	}
	for _, sessionID := range sessions {
		if err := s.summarizeSession(ctx, sessionID, false); err != nil {
			slog.Warn("summarization: failed for session",
				"session_id", sessionID,
				"tier", "Tier1(summarize)",
				"error", err,
			)
		}
	}
	return nil
}

// processConsolidation is the Tier 2 scheduler function: scans for sessions with
// summaries that have not yet been consolidated and runs the consolidation service.
func (s *Scheduler) ProcessConsolidation(ctx context.Context) error {
	sessions, err := s.queries.ListUnconsolidatedSessions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list unconsolidated sessions: %w", err)
	}
	for _, sessionID := range sessions {
		// Get the session to extract ownership (user_id, team_id) for memory scoping.
		session, err := s.queries.GetSession(ctx, sessionID)
		if err != nil {
			slog.Warn("consolidation: failed to get session",
				"session_id", sessionID,
				"tier", "Tier2(consolidate)",
				"error", err,
			)
			continue
		}

		// Set ownership from the session so memories/lessons satisfy FK constraints.
		var teamID string
		if session.TeamID != nil {
			teamID = *session.TeamID
		}
		s.consolidationSvc.SetOwner(session.UserID, teamID)

		if err := s.consolidationSvc.ConsolidateSession(ctx, sessionID); err != nil {
			slog.Warn("consolidation: failed to consolidate session",
				"session_id", sessionID,
				"tier", "Tier2(consolidate)",
				"error", err,
			)
			continue
		}

		// After successful consolidation, update the project profile so that
		// concept frequencies and file references reflect the latest observations.
		if s.profileSvc != nil {
			// TODO: Extract actual project slug from session/memories when the schema
			// supports session-to-project mapping. Currently uses sessionID as the slug;
			// ListObservationsByProject ignores the parameter and returns all observations
			// until the schema supports project-scoped filtering.
			if err := s.profileSvc.UpdateProfile(ctx, sessionID); err != nil {
				slog.Warn("consolidation: profile update failed",
					"session_id", sessionID,
					"tier", "Tier2(consolidate)",
					"error", err,
				)
			}
		}
	}
	return nil
}

// processReflection is the Tier 3 scheduler function: decays insights on every
// tick, then checks for unreflected memories and runs reflection if needed.
func (s *Scheduler) ProcessReflection(ctx context.Context) error {
	// Decay runs on every tick, independently of whether there are new memories.
	weeksSince := float64(s.intervals.Reflection.Hours()) / (24 * 7)
	if weeksSince < 1.0 {
		weeksSince = 1.0
	}
	if decayed, deleted, err := s.reflectionSvc.DecayInsights(ctx, weeksSince); err != nil {
		slog.Warn("reflection: decay sweep failed", "error", err)
	} else {
		slog.Info("reflection: decay sweep complete", "decayed", decayed, "soft_deleted", deleted)
	}

	hasUnreflected, err := s.queries.HasUnreflectedMemories(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for unreflected memories: %w", err)
	}
	if !hasUnreflected {
		return nil
	}
	if err := s.reflectionSvc.RunReflection(ctx, "", 10); err != nil {
		slog.Warn("reflection: failed to run reflection",
			"tier", "Tier3(reflect)",
			"error", err,
		)
		return err
	}

	return nil
}
