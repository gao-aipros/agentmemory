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
	ID          string
	Title       string
	Narrative   string
	Facts       string
	Concepts    []string
	Files       []string
	Score       float64
	Timestamp   string
	OwnerUserID string
}

