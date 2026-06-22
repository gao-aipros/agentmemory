package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SearchService provides hybrid search (BM25 + vector) over observations.
type SearchService struct {
	queries  *store.Queries
	embedSvc *EmbeddingService
}

// NewSearchService creates a new SearchService.
func NewSearchService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *SearchService {
	return &SearchService{
		queries:  store.New(pool),
		embedSvc: embedSvc,
	}
}

// HybridSearch performs a combined BM25 + vector search scoped to a single user.
// Steps:
// 1. Generate an embedding for the query text
// 2. Execute the hybrid SQL query (BM25 + vector via FULL OUTER JOIN)
// 3. Optionally run graph traversal to add graph bonus scores
// Returns ordered search results.
// userID enforces cross-tenant isolation — results are scoped to this user only.
func (s *SearchService) HybridSearch(ctx context.Context, query string, limit int, userID string) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}

	// Generate embedding for the query
	var vec *pgvector.Vector
	if s.embedSvc != nil && s.embedSvc.Embedder() != nil {
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
			OwnerUserID:    userID,
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
			OwnerUserID: userID,
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
		traversed, err := s.queries.GraphTraversal(ctx, store.GraphTraversalParams{
			Column1:     seedIds,
			OwnerUserID: &userID,
		})
		if err != nil {
			// Log but continue — graph scores will be 0
			traversed = nil
		}
		for _, gt := range traversed {
			graphScores[gt.ID] = math.Min(gt.GraphScore, 1.0) // Cap graph score at 1.0
		}
	}

	// Batch-fetch all observation details in a single query (avoid N+1).
	obsByID := make(map[string]store.Observation, len(rows))
	if len(seedIds) > 0 {
		batchObs, err := s.queries.GetObservationsByIDs(ctx, seedIds)
		if err == nil {
			for _, o := range batchObs {
				obsByID[o.ID] = o
			}
		}
	}

	// Build final ranked results with graph bonus
	results := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		graphScore := graphScores[r.ID]
		combinedScore := CombineSearchScores(r.Bm25Score, r.VectorScore, graphScore)

		var title, narrative string
		if obs, ok := obsByID[r.ID]; ok {
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

	// Sort by combined score descending using stdlib sort.
	sortSearchResults(results)

	// Apply limit
	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// sortSearchResults sorts results by CombinedScore descending.
func sortSearchResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].CombinedScore > results[j].CombinedScore
	})
}

// SearchCompact returns lightweight search results for progressive disclosure.
func (s *SearchService) SearchCompact(ctx context.Context, query string, limit int, userID string) ([]CompactResult, error) {
	results, err := s.HybridSearch(ctx, query, limit, userID)
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

// SearchExpand returns full observation details for the given IDs, scoped to userID.
// Uses a single batch query to avoid N+1, then filters by owner in-memory.
// userID enforces cross-tenant isolation — only observations owned by this user are returned.
func (s *SearchService) SearchExpand(ctx context.Context, ids []string, userID string) ([]FullResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	observations, err := s.queries.GetObservationsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch observations: %w", err)
	}

	results := make([]FullResult, 0, len(observations))
	for _, obs := range observations {
		// Cross-tenant isolation: skip observations not owned by the requesting user.
		if obs.OwnerUserID == nil || *obs.OwnerUserID != userID {
			continue
		}

		facts := ""
		if obs.Facts != nil {
			facts = *obs.Facts
		}

		timestamp := ""
		if obs.Timestamp.Valid {
			timestamp = obs.Timestamp.Time.Format("2006-01-02T15:04:05Z")
		}

		ownerID := ""
		if obs.OwnerUserID != nil {
			ownerID = *obs.OwnerUserID
		}

		results = append(results, FullResult{
			ID:          obs.ID,
			Title:       obs.Title,
			Narrative:   obs.Narrative,
			Facts:       facts,
			Concepts:    obs.Concepts,
			Files:       obs.Files,
			Timestamp:   timestamp,
			OwnerUserID: ownerID,
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
