package service

import (
	"context"
	"log/slog"
	"time"
)

// SessionEndHandler orchestrates the end-of-session pipeline:
// 1. Close session (update ended_at, status=ended)
// 2. Trigger summarize (async)
// 3. After summarize: trigger consolidate (async)
// 4. After consolidate: trigger reflect timer check
type SessionEndHandler struct {
	sessionSvc   *SessionService
	summarizer   *SummarizationService
	consolidator *ConsolidationService
	reflector    *ReflectionService
}

// NewSessionEndHandler creates a new SessionEndHandler.
func NewSessionEndHandler(
	sessionSvc *SessionService,
	summarizer *SummarizationService,
	consolidator *ConsolidationService,
	reflector *ReflectionService,
) *SessionEndHandler {
	return &SessionEndHandler{
		sessionSvc:   sessionSvc,
		summarizer:   summarizer,
		consolidator: consolidator,
		reflector:    reflector,
	}
}

// HandleSessionEnd closes the session and triggers the memory pipeline.
// Steps 2-4 run asynchronously.
func (h *SessionEndHandler) HandleSessionEnd(ctx context.Context, sessionID string) error {
	// Step 1: Close the session (synchronous)
	session, err := h.sessionSvc.EndSession(ctx, sessionID)
	if err != nil {
		return err
	}

	slog.Info("session ended, starting memory pipeline",
		"session_id", sessionID,
	)

	// Steps 2-4: Run pipeline asynchronously
	go h.runPipeline(sessionID)

	_ = session // session data available for pipeline use
	return nil
}

// runPipeline executes the summarize → consolidate → reflect chain.
func (h *SessionEndHandler) runPipeline(sessionID string) {
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
