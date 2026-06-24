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

// patternsQuerier handles pattern-related database queries.
type patternsQuerier interface {
	GetConceptFrequencies(ctx context.Context) ([]store.GetConceptFrequenciesRow, error)
}

// PatternsService detects recurring patterns across sessions for a project.
type PatternsService struct {
	queries patternsQuerier
}

// NewPatternsService creates a new PatternsService backed by the given connection pool.
func NewPatternsService(pool *pgxpool.Pool) *PatternsService {
	return &PatternsService{
		queries: store.New(pool),
	}
}

// newPatternsServiceWithQuerier creates a PatternsService with a custom querier (for testing).
func newPatternsServiceWithQuerier(q patternsQuerier) *PatternsService {
	return &PatternsService{
		queries: q,
	}
}

// DetectPatterns analyzes past session data for a project and returns a
// PatternSummary with detected concept frequencies.
func (s *PatternsService) DetectPatterns(ctx context.Context, project string) (*PatternSummary, error) {
	storeRows, err := s.queries.GetConceptFrequencies(ctx)
	if err != nil {
		return nil, err
	}
	concepts := make([]ConceptFreq, len(storeRows))
	for i, row := range storeRows {
		concepts[i] = ConceptFreq{
			Concept: row.Concept.(string),
			Count:   int(row.Freq),
		}
	}
	return &PatternSummary{
		Project:      project,
		TopConcepts:  concepts,
		ToolUsage:    make([]ToolFreq, 0),
		FilePatterns: make([]FilePattern, 0),
		SessionCount: 0,
		GeneratedAt:  time.Now().UTC(),
	}, nil
}
