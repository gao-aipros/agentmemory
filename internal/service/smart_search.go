package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SmartSearchService provides the memory_smart_search tool.
// Hybrid search + optional expand IDs for progressive disclosure.
type SmartSearchService struct {
	searchSvc *SearchService
}

// NewSmartSearchService creates a new SmartSearchService.
func NewSmartSearchService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *SmartSearchService {
	return &SmartSearchService{
		searchSvc: NewSearchService(pool, embedSvc),
	}
}

// SmartSearchResult is the output of a smart search operation.
type SmartSearchResult struct {
	Query    string
	Compact  []CompactResult
	Expanded []FullResult `json:"expanded,omitempty"`
	Limit    int
}

// Search performs a smart search with optional progressive disclosure expansion.
// Returns compact results by default. If expandIDs are provided, those IDs are
// expanded to full results in the same response.
// userID enforces cross-tenant isolation.
func (s *SmartSearchService) Search(ctx context.Context, query string, limit int, expandIDs []string, userID string) (*SmartSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Perform hybrid search
	results, err := s.searchSvc.HybridSearch(ctx, query, limit, userID)
	if err != nil {
		return nil, fmt.Errorf("smart search failed: %w", err)
	}

	out := &SmartSearchResult{
		Query: query,
		Limit: limit,
	}

	// Build compact results
	out.Compact = make([]CompactResult, len(results))
	for i, r := range results {
		out.Compact[i] = CompactResult{
			ID:    r.ID,
			Title: r.Title,
			Score: r.CombinedScore,
		}
	}

	// Expand requested IDs
	if len(expandIDs) > 0 {
		expanded, err := s.searchSvc.SearchExpand(ctx, expandIDs)
		if err != nil {
			return nil, fmt.Errorf("smart search expand failed: %w", err)
		}
		// Attach scores
		scoreMap := make(map[string]float64)
		for _, r := range results {
			scoreMap[r.ID] = r.CombinedScore
		}
		for i := range expanded {
			expanded[i].Score = scoreMap[expanded[i].ID]
		}
		out.Expanded = expanded
	}

	return out, nil
}
