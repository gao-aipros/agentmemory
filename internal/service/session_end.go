package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"golang.org/x/sync/semaphore"
)

// sessionEndSessioner is the interface for ending sessions, extracted for testability.
type sessionEndSessioner interface {
	GetSession(ctx context.Context, sessionID string) (*store.Session, error)
	EndSession(ctx context.Context, sessionID string) (*store.Session, error)
}

// SessionEndHandler orchestrates the end-of-session pipeline:
// 1. Close session (update ended_at, status=ended)
// 2. Trigger compression (via Scheduler, async)
// 3. Graph extraction (fire-and-forget, never blocks pipeline)
// 4. After compression: trigger full summarization (via Scheduler, async)
type SessionEndHandler struct {
	sessionSvc   sessionEndSessioner
	scheduler    *Scheduler
	summarizer   *SummarizationService
	consolidator *ConsolidationService
	reflector    *ReflectionService
	profileSvc   *ProfileService
	graphExtract *GraphExtractionService
	wg           *sync.WaitGroup
	sem          *semaphore.Weighted
}

// NewSessionEndHandler creates a new SessionEndHandler.
// graphExtract is optional; pass nil to disable graph extraction.
func NewSessionEndHandler(
	sessionSvc *SessionService,
	scheduler *Scheduler,
	summarizer *SummarizationService,
	consolidator *ConsolidationService,
	reflector *ReflectionService,
	profileSvc *ProfileService,
	graphExtract *GraphExtractionService,
	wg *sync.WaitGroup,
	sem *semaphore.Weighted,
) *SessionEndHandler {
	return &SessionEndHandler{
		sessionSvc:   sessionSvc,
		scheduler:    scheduler,
		summarizer:   summarizer,
		consolidator: consolidator,
		reflector:    reflector,
		profileSvc:   profileSvc,
		graphExtract: graphExtract,
		wg:           wg,
		sem:          sem,
	}
}

// HandleSessionEnd closes the session and triggers the memory pipeline.
// Steps 2-4 run asynchronously. The pipeline goroutine is tracked by the
// WaitGroup and bounded by the semaphore.
func (h *SessionEndHandler) HandleSessionEnd(ctx context.Context, sessionID string) error {
	// Step 0: Check if session already ended (idempotency guard).
	session, err := h.sessionSvc.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == "ended" {
		slog.Debug("session already ended, skipping pipeline", "session_id", sessionID)
		return nil
	}

	// Step 1: Close the session (synchronous)
	session, err = h.sessionSvc.EndSession(ctx, sessionID)
	if err != nil {
		return err
	}

	slog.Info("session ended, starting memory pipeline",
		"session_id", sessionID,
	)

	// Steps 2-4: Run pipeline asynchronously with lifecycle management.
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("pipeline goroutine panicked", "session_id", sessionID, "panic", r)
			}
		}()
		if err := h.sem.Acquire(context.Background(), 1); err != nil {
			return // context cancelled, server shutting down
		}
		defer h.sem.Release(1)
		h.runPipeline(sessionID)
	}()

	_ = session // session data available for pipeline use
	return nil
}

// Wait blocks until all in-flight pipeline goroutines have completed.
// Call during server shutdown after the HTTP server has stopped accepting
// new requests.
func (h *SessionEndHandler) Wait() {
	h.wg.Wait()
}

// runPipeline executes the compress → graph-extract → summarize chain via the Scheduler.
// Consolidate and reflect are handled by the Scheduler's periodic tiers (US4).
func (h *SessionEndHandler) runPipeline(sessionID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("pipeline goroutine panicked", "session_id", sessionID, "panic", r)
		}
	}()
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Step 2: Compress observations via Scheduler
	if h.scheduler != nil {
		slog.Info("starting session compression", "session_id", sessionID)
		if err := h.scheduler.CompressSessionNow(bgCtx, sessionID); err != nil {
			slog.Warn("session compression failed",
				"session_id", sessionID,
				"error", err,
			)
			// Continue to summarization even if compression fails
		}
	}

	// Step 2.5: Graph extraction (fire-and-forget, never blocks pipeline)
	if h.graphExtract != nil {
		h.wg.Add(1)
			go func() {
				defer h.wg.Done()
				defer func() {
					if r := recover(); r != nil {
						slog.Error("graph extraction goroutine panicked", "session_id", sessionID, "panic", r)
					}
				}()
			bgCtx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel2()
			if _, _, err := h.graphExtract.ExtractFromSession(bgCtx2, sessionID); err != nil {
				slog.Warn("graph extraction failed", "session_id", sessionID, "error", err)
			}
		}()
	}
	// Step 3: Full summarization via Scheduler
	if h.scheduler != nil {
		slog.Info("starting session summarization", "session_id", sessionID)
		if err := h.scheduler.SummarizeSessionNow(bgCtx, sessionID, true); err != nil {
			slog.Warn("session summarization failed",
				"session_id", sessionID,
				"error", err,
			)
		}
	}
}
