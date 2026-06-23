package service

import (
	"context"
	"fmt"
	"time"

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
	getConceptFrequencies(ctx context.Context) ([]ConceptFreq, error)
}

// patternsQuerierImpl is the production implementation using raw SQL.
type patternsQuerierImpl struct {
	pool *pgxpool.Pool
}

func (q *patternsQuerierImpl) getConceptFrequencies(ctx context.Context) ([]ConceptFreq, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT unnest(concepts) AS concept, count(*) AS freq
		FROM observations
		WHERE concepts IS NOT NULL AND cardinality(concepts) > 0
		GROUP BY concept
		ORDER BY freq DESC
		LIMIT 50
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query concept frequencies: %w", err)
	}
	defer rows.Close()

	var concepts []ConceptFreq
	for rows.Next() {
		var cf ConceptFreq
		if err := rows.Scan(&cf.Concept, &cf.Count); err != nil {
			return nil, fmt.Errorf("failed to scan concept frequency: %w", err)
		}
		concepts = append(concepts, cf)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating concept frequencies: %w", err)
	}

	return concepts, nil
}

// PatternsService detects recurring patterns across sessions for a project.
type PatternsService struct {
	queries patternsQuerier
}

// NewPatternsService creates a new PatternsService backed by the given connection pool.
func NewPatternsService(pool *pgxpool.Pool) *PatternsService {
	return &PatternsService{
		queries: &patternsQuerierImpl{pool: pool},
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
	concepts, err := s.queries.getConceptFrequencies(ctx)
	if err != nil {
		return nil, err
	}
	if concepts == nil {
		concepts = make([]ConceptFreq, 0)
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
