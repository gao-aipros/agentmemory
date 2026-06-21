package service

import (
	"context"
	"fmt"
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
	Pinned      bool      `json:"pinned"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SlotService manages working-memory slots. MVP uses an in-memory map.
// Future versions will persist slots to the database.
type SlotService struct {
	pool  *pgxpool.Pool
	mu    sync.RWMutex
	slots map[string]*Slot // keyed by label
}

// NewSlotService creates a new SlotService.
func NewSlotService(pool *pgxpool.Pool) *SlotService {
	return &SlotService{
		pool:  pool,
		slots: make(map[string]*Slot),
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

	// Check for duplicate label. We key only by label in the MVP store.
	if _, exists := s.slots[label]; exists {
		return nil, fmt.Errorf("slot with label %q already exists", label)
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

	s.slots[label] = slot

	return slot, nil
}

// GetSlot returns the content of a named slot, or an empty string if not found.
func (s *SlotService) GetSlot(ctx context.Context, label string) (string, error) {
	if label == "" {
		return "", fmt.Errorf("label is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, ok := s.slots[label]
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
func (s *SlotService) ReplaceSlot(ctx context.Context, label, content string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	slot, ok := s.slots[label]
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

// DeleteSlot removes a slot by label. Returns an error if the slot does not exist.
func (s *SlotService) DeleteSlot(ctx context.Context, label string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.slots[label]; !ok {
		return fmt.Errorf("slot %q not found", label)
	}

	delete(s.slots, label)

	return nil
}

// AppendSlot appends text to an existing slot's content. Returns an error if
// the append would exceed the slot's sizeLimit or if the slot does not exist.
func (s *SlotService) AppendSlot(ctx context.Context, label, text string) error {
	if label == "" {
		return fmt.Errorf("label is required")
	}
	if text == "" {
		return nil // nothing to append
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	slot, ok := s.slots[label]
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

// generateSlotID generates a UUID string for slot identification (reserved
// for future DB-backed implementation).
func generateSlotID() string {
	return uuid.New().String()
}
