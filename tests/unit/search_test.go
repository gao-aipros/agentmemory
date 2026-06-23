package unit

import (
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Search Weight Normalization Tests (T056)
// =============================================================================

func TestCombineSearchScores_AllStreamsPresent(t *testing.T) {
	// BM25 0.4 + vector 0.6 + graph 0.3
	bm25 := 0.8
	vector := 0.9
	graph := 0.5

	expected := bm25*0.4 + vector*0.6 + graph*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001,
		"combined score should be BM25*0.4 + vector*0.6 + graph*0.3")
}

func TestCombineSearchScores_ZeroBM25(t *testing.T) {
	bm25 := 0.0
	vector := 0.7
	graph := 0.3

	expected := 0.0*0.4 + 0.7*0.6 + 0.3*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_ZeroVector(t *testing.T) {
	bm25 := 0.5
	vector := 0.0
	graph := 0.4

	expected := 0.5*0.4 + 0.0*0.6 + 0.4*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_ZeroGraph(t *testing.T) {
	bm25 := 0.6
	vector := 0.8
	graph := 0.0

	expected := 0.6*0.4 + 0.8*0.6 + 0.0*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_AllZero(t *testing.T) {
	actual := service.CombineSearchScores(0.0, 0.0, 0.0)
	assert.Equal(t, 0.0, actual)
}

func TestCombineSearchScores_AllMax(t *testing.T) {
	// Maximum possible: 1.0*0.4 + 1.0*0.6 + 1.0*0.3 = 1.3
	actual := service.CombineSearchScores(1.0, 1.0, 1.0)
	assert.InDelta(t, 1.3, actual, 0.0001)
}

func TestCombineSearchScores_BM25Dominant(t *testing.T) {
	// When BM25 is high and others are low
	bm25 := 1.0
	vector := 0.1
	graph := 0.0

	expected := 1.0*0.4 + 0.1*0.6 + 0.0*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_VectorDominant(t *testing.T) {
	// When vector is high and others are low
	bm25 := 0.1
	vector := 1.0
	graph := 0.0

	expected := 0.1*0.4 + 1.0*0.6 + 0.0*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_GraphDominant(t *testing.T) {
	// When graph bonus is high but BM25 and vector are moderate
	bm25 := 0.3
	vector := 0.3
	graph := 1.0

	expected := 0.3*0.4 + 0.3*0.6 + 1.0*0.3
	actual := service.CombineSearchScores(bm25, vector, graph)

	assert.InDelta(t, expected, actual, 0.0001)
}

func TestCombineSearchScores_WeightsInvariant(t *testing.T) {
	// Verify that the weights are exactly as specified
	weights := service.GetSearchWeights()

	assert.Equal(t, 0.4, weights.BM25, "BM25 weight must be 0.4")
	assert.Equal(t, 0.6, weights.Vector, "vector weight must be 0.6")
	assert.Equal(t, 0.3, weights.Graph, "graph weight must be 0.3")
}

// =============================================================================
// Search Isolation Tests — verify user-scoped search parameters
// =============================================================================

// TestSearchParams_RequiresOwnerUserID verifies that the search parameter types
// include an OwnerUserID field for cross-tenant isolation. This is a compile-time
// assertion: if the store.HybridSearchParams and store.Bm25SearchParams types
// do NOT include an OwnerUserID field, this test will not compile (RED phase).
func TestSearchParams_RequiresOwnerUserID(t *testing.T) {
	// These will fail to compile if OwnerUserID field is missing — that's by design
	// in the RED phase. Once implemented, these assertions verify the fields exist.
	bp := store.Bm25SearchParams{
		QueryText:   "test query",
		ResultLimit: 10,
		OwnerUserID: "user-123",
	}
	assert.Equal(t, "user-123", bp.OwnerUserID, "Bm25SearchParams must have OwnerUserID field")

	hp := store.HybridSearchParams{
		QueryText:   "test query",
		ResultLimit: 10,
		OwnerUserID: "user-456",
	}
	assert.Equal(t, "user-456", hp.OwnerUserID, "HybridSearchParams must have OwnerUserID field")

	// VectorSearchParams must also include OwnerUserID for cross-tenant isolation.
	// This assertion will fail to compile in the RED phase (field missing).
	ownerID := "user-789"
	vp := store.VectorSearchParams{
		OwnerUserID: &ownerID,
	}
	assert.NotNil(t, vp.OwnerUserID, "VectorSearchParams must have OwnerUserID field")
	assert.Equal(t, "user-789", *vp.OwnerUserID, "VectorSearchParams OwnerUserID must match")

	// Verify NULL owner bypasses the filter (for admin/internal use).
	vpNull := store.VectorSearchParams{
		OwnerUserID: nil,
	}
	assert.Nil(t, vpNull.OwnerUserID, "VectorSearchParams OwnerUserID should accept nil for admin bypass")
}

// TestGraphTraversalParams_RequiresOwnerUserID verifies that GraphTraversal
// parameters include an OwnerUserID field for cross-tenant isolation.
// This is a compile-time assertion: if the type or field does not exist,
// this test will not compile (RED phase).
func TestGraphTraversalParams_RequiresOwnerUserID(t *testing.T) {
	// With a specific owner — only returns graph nodes linked to that user's observations.
	ownerID := "user-42"
	gtp := store.GraphTraversalParams{
		OwnerUserID: &ownerID,
	}
	assert.NotNil(t, gtp.OwnerUserID, "GraphTraversalParams must have OwnerUserID field")
	assert.Equal(t, "user-42", *gtp.OwnerUserID, "GraphTraversalParams OwnerUserID must match")

	// NULL owner bypasses the filter (for admin/internal use).
	gtpNull := store.GraphTraversalParams{
		OwnerUserID: nil,
	}
	assert.Nil(t, gtpNull.OwnerUserID, "GraphTraversalParams OwnerUserID should accept nil for admin bypass")
}

// TestListAllLessonsParams_RequiresTeamID verifies that ListAllLessons
// parameters include a TeamID field for cross-tenant isolation.
// This is a compile-time assertion: if the type or field does not exist,
// this test will not compile (RED phase).
// =============================================================================
// VectorSearch Nil Embedding Guard Tests (Issue #92)
// =============================================================================

// TestVectorSearch_NilEmbedding verifies that VectorSearch returns an error
// when called with a nil embedding, rather than passing nil to pgvector.
func TestVectorSearch_NilEmbedding(t *testing.T) {
	q := &store.Queries{} // nil db — nil check happens before db query
	_, err := q.VectorSearch(nil, store.VectorSearchParams{
		Embedding:   nil,
		OwnerUserID: nil,
		Limit:       10,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding must not be nil")
}

func TestListAllLessonsParams_RequiresTeamID(t *testing.T) {
	// With a specific team — only returns lessons for that team.
	teamID := "team-alpha"
	lp := store.ListAllLessonsParams{
		TeamID: &teamID,
	}
	assert.NotNil(t, lp.TeamID, "ListAllLessonsParams must have TeamID field")
	assert.Equal(t, "team-alpha", *lp.TeamID, "ListAllLessonsParams TeamID must match")

	// NULL team bypasses the filter (for admin/internal use).
	lpNull := store.ListAllLessonsParams{
		TeamID: nil,
	}
	assert.Nil(t, lpNull.TeamID, "ListAllLessonsParams TeamID should accept nil for admin bypass")
}
