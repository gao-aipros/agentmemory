package service

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SearchService provides hybrid search (BM25 + vector) over observations.
type SearchService struct {
	queries *store.Queries
	embedSvc *EmbeddingService
}

// NewSearchService creates a new SearchService.
func NewSearchService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *SearchService {
	return &SearchService{
		queries:  store.New(pool),
		embedSvc: embedSvc,
	}
}

// HybridSearch performs a combined BM25 + vector search.
// Steps:
// 1. Generate an embedding for the query text
// 2. Execute the hybrid SQL query (BM25 + vector via FULL OUTER JOIN)
// 3. Optionally run graph traversal to add graph bonus scores
// Returns ordered search results.
func (s *SearchService) HybridSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Generate embedding for the query
	var vec *pgvector.Vector
	if s.embedSvc != nil && s.embedSvc.provider != nil {
		embedding, err := s.embedSvc.GenerateEmbedding(ctx, query)
		if err != nil {
			// Log but continue — vector search will be skipped
			vec = nil
		} else {
			v := pgvector.NewVector(embedding)
			vec = &v
		}
	}

	// Execute hybrid search
	var rows []store.HybridSearchRow
	if vec != nil {
		results, err := s.queries.HybridSearch(ctx, store.HybridSearchParams{
			QueryText:      query,
			QueryEmbedding: vec,
			ResultLimit:    int32(limit * 2), // Fetch more for reranking
		})
		if err != nil {
			return nil, fmt.Errorf("hybrid search failed: %w", err)
		}
		rows = results
	} else {
		// Fallback: BM25-only search when no embedding provider
		bm25Results, err := s.queries.Bm25Search(ctx, store.Bm25SearchParams{
			QueryText:   query,
			ResultLimit: int32(limit * 2),
		})
		if err != nil {
			return nil, fmt.Errorf("bm25 search failed: %w", err)
		}
		for _, r := range bm25Results {
			rows = append(rows, store.HybridSearchRow{
				ID:            r.ID,
				CombinedScore: r.Bm25Score * 0.4, // Only BM25 component
				Bm25Score:     r.Bm25Score,
				VectorScore:   0,
			})
		}
	}

	// Collect seed IDs for graph traversal
	seedIds := make([]string, 0, len(rows))
	for _, r := range rows {
		seedIds = append(seedIds, r.ID)
	}

	// Run graph traversal to get graph bonus scores
	graphScores := make(map[string]float64)
	if len(seedIds) > 0 {
		traversed, err := s.queries.GraphTraversal(ctx, seedIds)
		if err != nil {
			// Log but continue — graph scores will be 0
			traversed = nil
		}
		for _, gt := range traversed {
			graphScores[gt.ID] = math.Min(gt.GraphScore, 1.0) // Cap graph score at 1.0
		}
	}

	// Build final ranked results with graph bonus
	results := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		graphScore := graphScores[r.ID]
		combinedScore := CombineSearchScores(r.Bm25Score, r.VectorScore, graphScore)

		// Fetch observation details for title/narrative
		var title, narrative string
		obs, err := s.queries.GetObservation(ctx, r.ID)
		if err == nil {
			title = obs.Title
			narrative = truncate(obs.Narrative, 200)
		}

		results = append(results, SearchResult{
			ID:            r.ID,
			Title:         title,
			Narrative:     narrative,
			Bm25Score:     r.Bm25Score,
			VectorScore:   r.VectorScore,
			GraphScore:    graphScore,
			CombinedScore: combinedScore,
		})
	}

	// Sort by combined score descending
	sortSearchResults(results)

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// sortSearchResults sorts results by CombinedScore descending.
func sortSearchResults(results []SearchResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].CombinedScore > results[i].CombinedScore {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// SearchCompact returns lightweight search results for progressive disclosure.
func (s *SearchService) SearchCompact(ctx context.Context, query string, limit int) ([]CompactResult, error) {
	results, err := s.HybridSearch(ctx, query, limit)
	if err != nil {
		return nil, err
	}

	compact := make([]CompactResult, len(results))
	for i, r := range results {
		compact[i] = CompactResult{
			ID:    r.ID,
			Title: r.Title,
			Score: r.CombinedScore,
		}
	}
	return compact, nil
}

// SearchExpand returns full observation details for the given IDs.
func (s *SearchService) SearchExpand(ctx context.Context, ids []string) ([]FullResult, error) {
	results := make([]FullResult, 0, len(ids))

	for _, id := range ids {
		obs, err := s.queries.GetObservation(ctx, id)
		if err != nil {
			continue // Skip missing observations
		}

		facts := ""
		if obs.Facts != nil {
			facts = *obs.Facts
		}

		timestamp := ""
		if obs.Timestamp.Valid {
			timestamp = obs.Timestamp.Time.Format("2006-01-02T15:04:05Z")
		}

		results = append(results, FullResult{
			ID:        obs.ID,
			Title:     obs.Title,
			Narrative: obs.Narrative,
			Facts:     facts,
			Concepts:  obs.Concepts,
			Files:     obs.Files,
			Timestamp: timestamp,
		})
	}

	return results, nil
}

// truncate shortens text to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Try to break at a word boundary
	idx := strings.LastIndex(s[:maxLen], " ")
	if idx > maxLen/2 {
		return s[:idx] + "..."
	}
	return s[:maxLen-3] + "..."
}
