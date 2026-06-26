package service

// SearchWeights defines the coefficient weights for hybrid search scoring.
// BM25 * 0.4 + Vector * 0.6 + Graph * 0.3 + Memory * 0.4
type SearchWeights struct {
	BM25   float64
	Vector float64
	Graph  float64
	Memory float64
}

// DefaultSearchWeights returns the standard search weight configuration.
func DefaultSearchWeights() SearchWeights {
	return SearchWeights{
		BM25:   0.4,
		Vector: 0.6,
		Graph:  0.3,
		Memory: 0.4,
	}
}

// GetSearchWeights returns the default search weights (exported for tests).
func GetSearchWeights() SearchWeights {
	return DefaultSearchWeights()
}

// CombineSearchScores computes the weighted combined score from three search streams.
// Formula: BM25*0.4 + vector*0.6 + graph*0.3
func CombineSearchScores(bm25Score, vectorScore, graphScore float64) float64 {
	w := DefaultSearchWeights()
	return bm25Score*w.BM25 + vectorScore*w.Vector + graphScore*w.Graph
}

// SearchResult represents a single result from hybrid search.
type SearchResult struct {
	ID            string
	Title         string
	Narrative     string
	Bm25Score     float64
	VectorScore   float64
	GraphScore    float64
	CombinedScore float64
	Source        string
}
