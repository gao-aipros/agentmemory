package unit

import (
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
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
