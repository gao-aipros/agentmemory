package service

// CompactResult is a lightweight search result for progressive disclosure.
// Contains only id, title, and score — details fetched on expand.
type CompactResult struct {
	ID    string
	Title string
	Score float64
}

// FullResult is a fully expanded search result with all observation details.
type FullResult struct {
	ID        string
	Title     string
	Narrative string
	Facts     string
	Concepts  []string
	Files     []string
	Score     float64
	Timestamp string
}

// ProgressiveDisclosure encapsulate the two-step progressive disclosure pattern:
// 1. SearchCompact returns minimal results (id, title, score)
// 2. SearchExpand returns full details for selected IDs
type ProgressiveDisclosure struct {
	Results     []CompactResult
	Expandable  bool
	TotalHits   int
}
