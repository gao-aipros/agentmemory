package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// =============================================================================
// SignalService — agent-to-agent messaging
// =============================================================================

// Signal represents an inter-agent message.
type Signal struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	ReplyTo   string    `json:"reply_to"`
	ThreadID  string    `json:"thread_id"`
	Type      string    `json:"type"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// SignalService manages inter-agent signals (messages).
// MVP uses an in-memory store.
type SignalService struct {
	pool    *pgxpool.Pool
	mu      sync.RWMutex
	signals []*Signal
}

// NewSignalService creates a new SignalService.
func NewSignalService(pool *pgxpool.Pool) *SignalService {
	return &SignalService{
		pool:    pool,
		signals: make([]*Signal, 0),
	}
}

// SendSignal creates and stores a new signal (message) from one agent to another.
func (s *SignalService) SendSignal(ctx context.Context, from, to, content, replyTo, msgType string) (*Signal, error) {
	if from == "" {
		return nil, fmt.Errorf("from agent ID is required")
	}
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	signal := &Signal{
		ID:        uuid.New().String(),
		From:      from,
		To:        to,
		Content:   content,
		ReplyTo:   replyTo,
		Type:      msgType,
		Read:      false,
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour), // default 24h TTL
	}

	// Auto-set ThreadID: if replying, inherit the parent's thread; otherwise, start a new thread.
	if replyTo != "" {
		for _, existing := range s.signals {
			if existing.ID == replyTo {
				signal.ThreadID = existing.ThreadID
				break
			}
		}
	}
	if signal.ThreadID == "" {
		// Also check if replyTo was a thread root.
		for _, existing := range s.signals {
			if existing.ThreadID == replyTo {
				signal.ThreadID = replyTo
				break
			}
		}
		if signal.ThreadID == "" {
			signal.ThreadID = signal.ID
		}
	}

	s.signals = append(s.signals, signal)

	return signal, nil
}

// ReadSignals returns messages for the given agent. If unreadOnly is true,
// only unread messages are returned. If threadID is non-empty, only messages
// from that thread are returned.
func (s *SignalService) ReadSignals(ctx context.Context, agentID string, limit int, threadID string, unreadOnly bool) ([]Signal, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}

	var result []Signal
	for _, sig := range s.signals {
		if len(result) >= limit {
			break
		}

		// Match recipient: either addressed directly or broadcast (empty To).
		if sig.To != "" && sig.To != agentID {
			continue
		}

		if unreadOnly && sig.Read {
			continue
		}

		if threadID != "" && sig.ThreadID != threadID {
			continue
		}

		result = append(result, *sig)
		// Mark as read.
		sig.Read = true
	}

	if result == nil {
		result = []Signal{}
	}

	return result, nil
}

// =============================================================================
// SentinelService — event-driven action gate watchers
// =============================================================================

// Sentinel watches for external conditions and unblocks gated actions when fired.
type Sentinel struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Config          string    `json:"config"`
	LinkedActionIDs string    `json:"linked_action_ids"`
	Result          string    `json:"result"`
	Triggered       bool      `json:"triggered"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// SentinelService manages sentinels — event-driven watchers that auto-unblock
// gated actions when triggered.
type SentinelService struct {
	pool      *pgxpool.Pool
	mu        sync.RWMutex
	sentinels map[string]*Sentinel
}

// NewSentinelService creates a new SentinelService.
func NewSentinelService(pool *pgxpool.Pool) *SentinelService {
	return &SentinelService{
		pool:      pool,
		sentinels: make(map[string]*Sentinel),
	}
}

// CreateSentinel creates a new sentinel with the given configuration.
func (s *SentinelService) CreateSentinel(ctx context.Context, name, sentinelType, config, linkedActionIDs string, expiresInMs int64) (*Sentinel, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if sentinelType == "" {
		return nil, fmt.Errorf("type is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(expiresInMs) * time.Millisecond)
	if expiresInMs <= 0 {
		expiresAt = now.Add(1 * time.Hour) // default 1h TTL
	}

	sentinel := &Sentinel{
		ID:              uuid.New().String(),
		Name:            name,
		Type:            sentinelType,
		Config:          config,
		LinkedActionIDs: linkedActionIDs,
		Result:          "",
		Triggered:       false,
		CreatedAt:       now,
		ExpiresAt:       expiresAt,
	}

	s.sentinels[sentinel.ID] = sentinel

	return sentinel, nil
}

// TriggerSentinel fires a sentinel, recording the result and marking it as
// triggered. Triggered sentinels unblock their gated actions.
func (s *SentinelService) TriggerSentinel(ctx context.Context, sentinelID, result string) error {
	if sentinelID == "" {
		return fmt.Errorf("sentinel ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sentinel, ok := s.sentinels[sentinelID]
	if !ok {
		return fmt.Errorf("sentinel %q not found", sentinelID)
	}

	if sentinel.Triggered {
		return fmt.Errorf("sentinel %q has already been triggered", sentinelID)
	}

	sentinel.Triggered = true
	sentinel.Result = result

	return nil
}

// =============================================================================
// CheckpointService — external gates for action progression
// =============================================================================

// Checkpoint represents an external gate (CI result, approval, deploy status)
// that blocks or unblocks dependent actions.
type Checkpoint struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	LinkedActionIDs string    `json:"linked_action_ids"`
	Status          string    `json:"status"` // "pending", "passed", "failed"
	CreatedAt       time.Time `json:"created_at"`
	ResolvedAt      time.Time `json:"resolved_at"`
}

// CheckpointService manages external checkpoints that gate action progress.
type CheckpointService struct {
	pool        *pgxpool.Pool
	mu          sync.RWMutex
	checkpoints map[string]*Checkpoint
}

// NewCheckpointService creates a new CheckpointService.
func NewCheckpointService(pool *pgxpool.Pool) *CheckpointService {
	return &CheckpointService{
		pool:        pool,
		checkpoints: make(map[string]*Checkpoint),
	}
}

// CreateCheckpoint creates a new external checkpoint that gates the specified
// action IDs.
func (s *CheckpointService) CreateCheckpoint(ctx context.Context, name, checkType, linkedActionIDs string) (*Checkpoint, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if checkType == "" {
		return nil, fmt.Errorf("type is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	cp := &Checkpoint{
		ID:              uuid.New().String(),
		Name:            name,
		Type:            checkType,
		LinkedActionIDs: linkedActionIDs,
		Status:          "pending",
		CreatedAt:       now,
	}

	s.checkpoints[cp.ID] = cp

	return cp, nil
}

// ResolveCheckpoint resolves a checkpoint as passed or failed, recording the
// resolution timestamp.
func (s *CheckpointService) ResolveCheckpoint(ctx context.Context, checkpointID, status string) error {
	if checkpointID == "" {
		return fmt.Errorf("checkpoint ID is required")
	}
	if status != "passed" && status != "failed" {
		return fmt.Errorf("status must be 'passed' or 'failed', got %q", status)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cp, ok := s.checkpoints[checkpointID]
	if !ok {
		return fmt.Errorf("checkpoint %q not found", checkpointID)
	}

	cp.Status = status
	cp.ResolvedAt = time.Now().UTC()

	return nil
}

// ListCheckpoints returns all checkpoints.
func (s *CheckpointService) ListCheckpoints(ctx context.Context) ([]Checkpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Checkpoint, 0, len(s.checkpoints))
	for _, cp := range s.checkpoints {
		result = append(result, *cp)
	}

	return result, nil
}
