package service

import (
	"context"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PatternSummary aggregates detected patterns across sessions, concepts,
// tool usage, and file patterns for a given project.
type PatternSummary struct {
	Project      string        `json:"project"`
	TopConcepts  []ConceptFreq `json:"top_concepts"`
	ToolUsage    []ToolFreq    `json:"tool_usage"`
	FilePatterns []FilePattern `json:"file_patterns"`
	SessionCount int           `json:"session_count"`
	GeneratedAt  time.Time     `json:"generated_at"`
}

// ConceptFreq holds a concept and its occurrence count.
type ConceptFreq struct {
	Concept string `json:"concept"`
	Count   int    `json:"count"`
}

// ToolFreq holds a tool name and its usage count.
type ToolFreq struct {
	Tool  string `json:"tool"`
	Count int    `json:"count"`
}

// FilePattern describes a recurring file naming or modification pattern.
type FilePattern struct {
	Pattern     string `json:"pattern"`
	Count       int    `json:"count"`
	Description string `json:"description"`
}

// PatternsService detects recurring patterns across sessions for a project.
type PatternsService struct {
	queries *store.Queries
}

// NewPatternsService creates a new PatternsService backed by the given connection pool.
func NewPatternsService(pool *pgxpool.Pool) *PatternsService {
	return &PatternsService{
		queries: store.New(pool),
	}
}

// DetectPatterns analyzes past session data for a project and returns a
// PatternSummary with detected concept frequencies, tool usage patterns,
// and file modification patterns. For MVP, this returns a summary with
// empty slices — full pattern detection requires LLM integration that will
// be added in a future iteration. Always returns a valid PatternSummary
// without error.
func (s *PatternsService) DetectPatterns(ctx context.Context, project string) (*PatternSummary, error) {
	// MVP: Return an empty summary. The actual implementation will query
	// sessions and observations, then run LLM-driven pattern clustering
	// to extract concepts, tool frequencies, and file patterns.
	_ = project
	return &PatternSummary{
		Project:      project,
		TopConcepts:  make([]ConceptFreq, 0),
		ToolUsage:    make([]ToolFreq, 0),
		FilePatterns: make([]FilePattern, 0),
		SessionCount: 0,
		GeneratedAt:  time.Now().UTC(),
	}, nil
}
