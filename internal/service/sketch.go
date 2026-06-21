package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SketchAction is an individual action within a sketch's ephemeral action graph.
type SketchAction struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    int      `json:"priority"`
	Requires    []string `json:"requires"`
}

// Sketch is an ephemeral action graph for exploratory or draft work.
// Sketches auto-expire after their TTL unless promoted to permanent actions.
type Sketch struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Project     string         `json:"project"`
	Actions     []SketchAction `json:"actions"`
	Promoted    bool           `json:"promoted"`
	CreatedAt   time.Time      `json:"created_at"`
	ExpiresAt   time.Time      `json:"expires_at"`
}

// SketchService manages ephemeral action-graph sketches. Sketches support
// exploratory planning: create an action graph, iterate, then either promote
// to permanent actions or discard on expiry.
type SketchService struct {
	pool     *pgxpool.Pool
	mu       sync.RWMutex
	sketches map[string]*Sketch
}

// NewSketchService creates a new SketchService.
func NewSketchService(pool *pgxpool.Pool) *SketchService {
	return &SketchService{
		pool:     pool,
		sketches: make(map[string]*Sketch),
	}
}

// CreateSketch creates a new ephemeral sketch with the given metadata.
// expiresInMs sets the TTL after which the sketch may be discarded;
// 0 or negative means default of 1 hour.
func (s *SketchService) CreateSketch(ctx context.Context, title, description, project string, expiresInMs int64) (*Sketch, error) {
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	ttl := expiresInMs
	if ttl <= 0 {
		ttl = 3600000 // default 1 hour in ms
	}

	sketch := &Sketch{
		ID:          uuid.New().String(),
		Title:       title,
		Description: description,
		Project:     project,
		Actions:     []SketchAction{},
		Promoted:    false,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Duration(ttl) * time.Millisecond),
	}

	s.sketches[sketch.ID] = sketch

	log.Printf("[INFO] sketch: created sketch %q (%s)", sketch.ID, title)

	return sketch, nil
}

// PromoteSketch marks a sketch as promoted, converting its ephemeral actions
// into permanent actions (or preparing them for promotion).
func (s *SketchService) PromoteSketch(ctx context.Context, sketchID, project string) (*Sketch, error) {
	if sketchID == "" {
		return nil, fmt.Errorf("sketch ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sketch, ok := s.sketches[sketchID]
	if !ok {
		return nil, fmt.Errorf("sketch %q not found", sketchID)
	}

	if sketch.Promoted {
		return nil, fmt.Errorf("sketch %q has already been promoted", sketchID)
	}

	sketch.Promoted = true

	// If a project override is provided, update the sketch's project.
	if project != "" {
		sketch.Project = project
	}

	log.Printf("[INFO] sketch: promoted sketch %q (%s) with %d actions",
		sketchID, sketch.Title, len(sketch.Actions))

	return sketch, nil
}
