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
// 2. Trigger summarize (async)
// 3. After summarize: trigger consolidate (async)
// 4. After consolidate: trigger reflect timer check
type SessionEndHandler struct {
	sessionSvc   sessionEndSessioner
	summarizer   *SummarizationService
	consolidator *ConsolidationService
	reflector    *ReflectionService
	wg           *sync.WaitGroup
	sem          *semaphore.Weighted
}

// NewSessionEndHandler creates a new SessionEndHandler.
func NewSessionEndHandler(
	sessionSvc *SessionService,
	summarizer *SummarizationService,
	consolidator *ConsolidationService,
	reflector *ReflectionService,
	wg *sync.WaitGroup,
	sem *semaphore.Weighted,
) *SessionEndHandler {
	return &SessionEndHandler{
		sessionSvc:   sessionSvc,
		summarizer:   summarizer,
		consolidator: consolidator,
		reflector:    reflector,
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

// runPipeline executes the summarize → consolidate → reflect chain.
func (h *SessionEndHandler) runPipeline(sessionID string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("pipeline run panicked", "session_id", sessionID, "panic", r)
		}
	}()
	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Step 2: Summarize
	if h.summarizer != nil {
		slog.Info("starting session summarization", "session_id", sessionID)
		if err := h.summarizer.SummarizeSession(bgCtx, sessionID); err != nil {
			slog.Warn("session summarization failed",
				"session_id", sessionID,
				"error", err,
			)
			return // Don't continue if summarization failed
		}
	}

	// Step 3: Consolidate
	if h.consolidator != nil {
		slog.Info("starting session consolidation", "session_id", sessionID)
		if err := h.consolidator.ConsolidateSession(bgCtx, sessionID); err != nil {
			slog.Warn("session consolidation failed",
				"session_id", sessionID,
				"error", err,
			)
		}
	}

	// Step 4: Trigger reflection timer check
	if h.reflector != nil {
		h.reflector.TriggerTimerCheck(bgCtx)
	}
}
