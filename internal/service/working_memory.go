package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Slot represents a working-memory slot — a named, size-limited text container
// that persists across sessions.
type Slot struct {
	Label       string    `json:"label"`
	Content     string    `json:"content"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`   // "global" or "project"
	Project     string    `json:"project"` // project name when scope is "project"
	SizeLimit   int       `json:"size_limit"`
	Pinned      bool       `json:"pinned"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"` // nil = never expires (pinned slots)
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// slotKey returns a composite key that includes scope and project to prevent
// collisions between slots with the same label across different scopes.
// Key format: "{scope}:{project}:{label}"
func slotKey(scope, project, label string) string {
	return scope + ":" + project + ":" + label
}

// SlotService manages working-memory slots. MVP uses an in-memory map.
// Future versions will persist slots to the database.
type SlotService struct {
	pool       *pgxpool.Pool
	mu         sync.RWMutex
	slots      map[string]*Slot // keyed by slotKey(scope, project, label)
	DefaultTTL time.Duration    // TTL for new non-pinned slots (default 7 days)
	MaxSlots   int              // max in-memory slots (0 = unlimited)
}

// NewSlotService creates a new SlotService.
func NewSlotService(pool *pgxpool.Pool) *SlotService {
	return &SlotService{
		pool:       pool,
		slots:      make(map[string]*Slot),
		DefaultTTL: 7 * 24 * time.Hour, // 7 days
		MaxSlots:   0,                   // unlimited
	}
}

// CreateSlot creates a new slot. Returns an error if a slot with the same
// label+scope+project already exists.
func (s *SlotService) CreateSlot(ctx context.Context, label, content, description, scope, project string, pinned bool, sizeLimit int) (*Slot, error) {
	if label == "" {
		return nil, fmt.Errorf("label is required")
	}

	// Apply defaults.
	if scope == "" {
		scope = "global"
	}
	if sizeLimit <= 0 {
		sizeLimit = 2000
	}
	if sizeLimit > 20000 {
		return nil, fmt.Errorf("sizeLimit cannot exceed 20000, got %d", sizeLimit)
	}
	if len(content) > sizeLimit {
		return nil, fmt.Errorf("content length %d exceeds sizeLimit %d", len(content), sizeLimit)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate label+scope+project.
	key := slotKey(scope, project, label)
	if _, exists := s.slots[key]; exists {
		return nil, fmt.Errorf("slot with label %q already exists in scope %q", label, scope)
	}

	// Evict expired slots first if at capacity.
	if s.MaxSlots > 0 && len(s.slots) >= s.MaxSlots {
		s.cleanupExpiredLocked()
		if len(s.slots) >= s.MaxSlots {
			return nil, fmt.Errorf("max slots (%d) reached and no expired slots to evict", s.MaxSlots)
		}
	}

	now := time.Now().UTC()
	slot := &Slot{
		Label:       label,
		Content:     content,
		Description: description,
		Scope:       scope,
		Project:     project,
		SizeLimit:   sizeLimit,
		Pinned:      pinned,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Set expiry for non-pinned slots.
	if !pinned && s.DefaultTTL > 0 {
		expires := now.Add(s.DefaultTTL)
		slot.ExpiresAt = &expires
	}

	s.slots[key] = slot

	return slot, nil
}

// GetSlot returns the content of a named slot, or an empty string if not found.
func (s *SlotService) GetSlot(ctx context.Context, label, scope, project string) (string, error) {
	if label == "" {
		return "", fmt.Errorf("label is required")
	}
	if scope == "" {
		scope = "global"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	key := slotKey(scope, project, label)
	slot, ok := s.slots[key]
	if !ok {
		return "", fmt.Errorf("slot %q not found", label)
	}

	return slot.Content, nil
}

// ListSlots returns all slots, optionally filtered by project.
// When project is empty, all slots are returned.
func (s *SlotService) ListSlots(ctx context.Context, project string) ([]Slot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Slot
	for _, slot := range s.slots {
		if project != "" && slot.Scope == "project" && slot.Project != project {
			continue
		}
		result = append(result, *slot)
	}

	if result == nil {
		result = []Slot{}
	}

	return result, nil
}

// ReplaceSlot replaces the entire content of a slot. Returns an error if
// the new content exceeds the slot's sizeLimit or if the slot does not exist.
func (s *SlotService) ReplaceSlot(ctx context.Context, label, content, scope, project string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if scope == "" {
		scope = "global"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := slotKey(scope, project, label)
	slot, ok := s.slots[key]
	if !ok {
		return fmt.Errorf("slot %q not found", label)
	}

	if len(content) > slot.SizeLimit {
		return fmt.Errorf("content length %d exceeds slot sizeLimit %d", len(content), slot.SizeLimit)
	}

	slot.Content = content
	slot.UpdatedAt = time.Now().UTC()

	return nil
}

// DeleteSlot removes a slot by label, scope, and project. Returns an error if the slot does not exist.
func (s *SlotService) DeleteSlot(ctx context.Context, label, scope, project string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if scope == "" {
		scope = "global"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := slotKey(scope, project, label)
	if _, ok := s.slots[key]; !ok {
		return fmt.Errorf("slot %q not found", label)
	}

	delete(s.slots, key)

	return nil
}

// AppendSlot appends text to an existing slot's content. Returns an error if
// the append would exceed the slot's sizeLimit or if the slot does not exist.
func (s *SlotService) AppendSlot(ctx context.Context, label, text, scope, project string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if scope == "" {
		scope = "global"
	}
	if text == "" {
		return nil // nothing to append
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	key := slotKey(scope, project, label)
	slot, ok := s.slots[key]
	if !ok {
		return fmt.Errorf("slot %q not found", label)
	}

	newContent := slot.Content + text
	if len(newContent) > slot.SizeLimit {
		return fmt.Errorf("append would exceed sizeLimit %d (current %d + append %d = %d)",
			slot.SizeLimit, len(slot.Content), len(text), len(newContent))
	}

	slot.Content = newContent
	slot.UpdatedAt = time.Now().UTC()

	return nil
}

// CleanupExpired removes all expired, non-pinned slots. Returns the count removed.
func (s *SlotService) CleanupExpired(ctx context.Context) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupExpiredLocked()
}

// cleanupExpiredLocked removes expired slots. Caller must hold s.mu.Lock().
func (s *SlotService) cleanupExpiredLocked() int {
	now := time.Now().UTC()
	removed := 0
	for key, slot := range s.slots {
		if slot.Pinned || slot.ExpiresAt == nil {
			continue
		}
		if now.After(*slot.ExpiresAt) {
			delete(s.slots, key)
			removed++
		}
	}
	return removed
}

// StartCleanupLoop runs a background goroutine that calls CleanupExpired
// at the given interval. Stops when ctx is cancelled.
func (s *SlotService) StartCleanupLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Debug("slot cleanup loop stopped", "reason", ctx.Err())
				return
			case <-ticker.C:
				removed := s.CleanupExpired(ctx)
				if removed > 0 {
					slog.Debug("slot cleanup removed expired", "count", removed)
				}
			}
		}
	}()
}

// generateSlotID generates a UUID string for slot identification (reserved
// for future DB-backed implementation).
func generateSlotID() string {
	return uuid.New().String()
}
