package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
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

	// Overridable process functions (default to private methods, injected for testing).
	CompressionFunc   SchedulingFunc
	SummarizationFunc SchedulingFunc
	ConsolidationFunc SchedulingFunc
	ReflectionFunc    SchedulingFunc
}

// NewScheduler creates a new Scheduler with the given dependencies and intervals.
func NewScheduler(pool *pgxpool.Pool, llm *LLMService, embed *EmbeddingService, intervals SchedulerIntervals) *Scheduler {
	s := &Scheduler{
		pool:      pool,
		llm:       llm,
		embed:     embed,
		queries:   store.New(pool),
		intervals: intervals,
	}
	// Wire default process functions to the private implementations.
	s.CompressionFunc = s.processCompression
	s.SummarizationFunc = s.processSummarization
	s.ConsolidationFunc = s.processConsolidation
	s.ReflectionFunc = s.processReflection
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

// processCompression is a no-op stub for compression processing.
func (s *Scheduler) processCompression(ctx context.Context) error {
	return nil
}

// processSummarization is a no-op stub for summarization processing.
func (s *Scheduler) processSummarization(ctx context.Context) error {
	return nil
}

// processConsolidation is a no-op stub for consolidation processing.
func (s *Scheduler) processConsolidation(ctx context.Context) error {
	return nil
}

// processReflection is a no-op stub for reflection processing.
func (s *Scheduler) processReflection(ctx context.Context) error {
	return nil
}
