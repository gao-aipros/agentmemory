package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// SearchService provides hybrid search (BM25 + vector) over observations and memories.
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

// HybridSearch performs a combined BM25 + vector search over observations AND
// manually saved memories, scoped to a single user.
// Steps:
// 1. Generate an embedding for the query text
// 2. Execute the hybrid SQL query over observations (BM25 + vector via FULL OUTER JOIN)
// 3. Optionally run graph traversal to add graph bonus scores
// 4. Search memories using BM25 or hybrid memory queries
// 5. Merge both result sets, sort by CombinedScore, truncate to limit
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

	// Search observations
	obsResults := s.searchObservations(ctx, query, limit, vec, userID)

	// Search memories
	memResults := s.searchMemories(ctx, query, limit, vec, userID)

	// Merge, sort, and truncate
	return s.MergeSearchResults(obsResults, memResults, limit), nil
}

// searchObservations runs hybrid or BM25-only search over the observations table.
func (s *SearchService) searchObservations(ctx context.Context, query string, limit int, vec *pgvector.Vector, userID string) []SearchResult {
	var rows []store.HybridSearchRow
	if vec != nil {
		results, err := s.queries.HybridSearch(ctx, store.HybridSearchParams{
			QueryText:      query,
			QueryEmbedding: vec,
			ResultLimit:    int32(limit * 2), // Fetch more for reranking
			OwnerUserID:    userID,
		})
		if err != nil {
			slog.Error("observation search failed", "error", err)
			return nil
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
			slog.Error("memory bm25 search failed", "error", err)
			return nil
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
		if err == nil {
			for _, gt := range traversed {
				graphScores[gt.ID] = math.Min(gt.GraphScore, 1.0) // Cap graph score at 1.0
			}
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

	return results
}

// searchMemories runs hybrid or BM25-only search over the memories table.
func (s *SearchService) searchMemories(ctx context.Context, query string, limit int, vec *pgvector.Vector, userID string) []SearchResult {
	var rows []store.HybridSearchMemoriesRow

	if vec != nil {
		results, err := s.queries.HybridSearchMemories(ctx, store.HybridSearchMemoriesParams{
			QueryText:      query,
			QueryEmbedding: vec,
			ResultLimit:    int32(limit * 2),
			OwnerUserID:    &userID,
		})
		if err != nil {
			slog.Error("observation search failed", "error", err)
			return nil
		}
		rows = results
	} else {
		// Fallback: BM25-only search when no embedding provider
		bm25Results, err := s.queries.Bm25SearchMemories(ctx, store.Bm25SearchMemoriesParams{
			QueryText:   query,
			ResultLimit: int32(limit * 2),
			OwnerUserID: &userID,
		})
		if err != nil {
			slog.Error("memory bm25 search failed", "error", err)
			return nil
		}
		for _, r := range bm25Results {
			rows = append(rows, store.HybridSearchMemoriesRow{
				ID:        r.ID,
				Bm25Score: r.Bm25Score,
			})
		}
	}

	// Batch-fetch memory details in a single query (avoid N+1).
	memByID := make(map[string]store.Memory, len(rows))
	if len(rows) > 0 {
		memIDs := make([]string, len(rows))
		for i, r := range rows {
			memIDs[i] = r.ID
		}
		memories, err := s.queries.GetMemoriesByIDs(ctx, memIDs)
		if err != nil {
			slog.Error("failed to batch-fetch memories", "error", err)
		} else {
			for _, m := range memories {
				memByID[m.ID] = m
			}
		}
	}

	results := make([]SearchResult, 0, len(rows))
	for _, r := range rows {
		mem, ok := memByID[r.ID]
		if !ok {
			continue
		}
		combinedScore := CombineSearchScores(r.Bm25Score, r.VectorScore, 0)

		results = append(results, SearchResult{
			ID:            r.ID,
			Title:         truncate(mem.Content, 100),
			Narrative:     truncate(mem.Content, 200),
			Bm25Score:     r.Bm25Score,
			VectorScore:   r.VectorScore,
			CombinedScore: combinedScore,
		})
	}

	return results
}

// MergeSearchResults combines observation and memory search results, tags each
// with its source, sorts by CombinedScore descending, and truncates to limit.
func (s *SearchService) MergeSearchResults(obsResults, memResults []SearchResult, limit int) []SearchResult {
	// Tag observation results with source
	for i := range obsResults {
		obsResults[i].Source = "observation"
	}
	// Tag memory results with source
	for i := range memResults {
		memResults[i].Source = "manual_save"
	}

	// Combine both result sets
	all := make([]SearchResult, 0, len(obsResults)+len(memResults))
	all = append(all, obsResults...)
	all = append(all, memResults...)

	// Sort by CombinedScore descending
	sortSearchResults(all)

	// Truncate to limit
	if len(all) > limit {
		all = all[:limit]
	}

	return all
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
			ID:     r.ID,
			Title:  r.Title,
			Score:  r.CombinedScore,
			Source: r.Source,
		}
	}
	return compact, nil
}

// SearchExpand returns full details for the given IDs (observations and memories), scoped to userID.
// Queries both observations and memories tables, then merges results with source tagging.
// userID enforces cross-tenant isolation.
func (s *SearchService) SearchExpand(ctx context.Context, ids []string, userID string) ([]FullResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	results := make([]FullResult, 0, len(ids))

	// Fetch observations
	observations, err := s.queries.GetObservationsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch observations: %w", err)
	}

	for _, obs := range observations {
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
			Source:      "observation",
		})
	}

	// Fetch memories for IDs not found in observations
	foundIDs := make(map[string]bool)
	for _, r := range results {
		foundIDs[r.ID] = true
	}
	var missingIDs []string
	for _, id := range ids {
		if !foundIDs[id] {
			missingIDs = append(missingIDs, id)
		}
	}

	if len(missingIDs) > 0 {
		memories, err := s.queries.GetMemoriesByIDs(ctx, missingIDs)
		if err != nil {
			slog.Error("failed to fetch memories in SearchExpand", "error", err)
		} else {
			for _, mem := range memories {
				if mem.OwnerUserID == nil || *mem.OwnerUserID != userID {
					continue
				}
				results = append(results, FullResult{
					ID:          mem.ID,
					Title:       truncate(mem.Content, 100),
					Narrative:   mem.Content,
					Source:      "manual_save",
					OwnerUserID: *mem.OwnerUserID,
				})
			}
		}
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
