package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RecallService provides the memory_recall tool: hybrid search + progressive disclosure.
// Supports format: "compact" (default), "full", "narrative".
type RecallService struct {
	searchSvc *SearchService
}

// NewRecallService creates a new RecallService.
func NewRecallService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *RecallService {
	return &RecallService{
		searchSvc: NewSearchService(pool, embedSvc),
	}
}

// RecallResult is the output of a memory recall operation.
type RecallResult struct {
	Query     string
	Format    string
	Compact   []CompactResult `json:"compact,omitempty"`
	Full      []FullResult    `json:"full,omitempty"`
	Narrative string          `json:"narrative,omitempty"`
	TotalHits int
}

// Recall performs the memory_recall operation.
// format: "compact" returns id/title/score; "full" returns all fields;
// "narrative" returns a concatenated narrative.
// userID enforces cross-tenant isolation.
func (s *RecallService) Recall(ctx context.Context, query string, limit int, format string, userID string) (*RecallResult, error) {
	if limit <= 0 {
		limit = 10
	}
	if format == "" {
		format = "compact"
	}

	// Perform hybrid search
	results, err := s.searchSvc.HybridSearch(ctx, query, limit, userID)
	if err != nil {
		return nil, fmt.Errorf("recall search failed: %w", err)
	}

	out := &RecallResult{
		Query:     query,
		Format:    format,
		TotalHits: len(results),
	}

	switch format {
	case "compact":
		out.Compact = make([]CompactResult, len(results))
		for i, r := range results {
			out.Compact[i] = CompactResult{
				ID:    r.ID,
				Title: r.Title,
				Score: r.CombinedScore,
			}
		}

	case "full":
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}
		full, err := s.searchSvc.SearchExpand(ctx, ids, userID)
		if err != nil {
			return nil, fmt.Errorf("recall expand failed: %w", err)
		}
		// Attach scores to full results
		scoreMap := make(map[string]float64)
		for _, r := range results {
			scoreMap[r.ID] = r.CombinedScore
		}
		for i := range full {
			full[i].Score = scoreMap[full[i].ID]
		}
		out.Full = full

	case "narrative":
		ids := make([]string, len(results))
		for i, r := range results {
			ids[i] = r.ID
		}
		full, err := s.searchSvc.SearchExpand(ctx, ids, userID)
		if err != nil {
			return nil, fmt.Errorf("recall expand failed: %w", err)
		}
		// Build narrative by concatenating observation narratives
		var sb strings.Builder
		for i, f := range full {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(fmt.Sprintf("[%s] %s: %s", f.ID, f.Timestamp, f.Narrative))
		}
		out.Narrative = sb.String()

	default:
		return nil, fmt.Errorf("unsupported format %q: must be compact, full, or narrative", format)
	}

	return out, nil
}
