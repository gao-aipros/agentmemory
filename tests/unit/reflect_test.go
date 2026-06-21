package unit

import (
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestGroupMemoriesByConcept_EmptyList(t *testing.T) {
	clusters := service.GroupMemoriesByConcept(nil)
	assert.Empty(t, clusters, "nil memories should produce empty clusters")

	clusters = service.GroupMemoriesByConcept([]service.MemoryForReflection{})
	assert.Empty(t, clusters, "empty memories should produce empty clusters")
}

func TestGroupMemoriesByConcept_GroupsBySharedConcepts(t *testing.T) {
	memories := []service.MemoryForReflection{
		{ID: "m1", Content: "Using pgvector for embeddings", Concepts: []string{"pgvector", "embeddings"}},
		{ID: "m2", Content: "Embedding dimension is 1536", Concepts: []string{"embeddings", "dimensions"}},
		{ID: "m3", Content: "PostgreSQL connection pooling", Concepts: []string{"postgresql", "pooling"}},
	}

	clusters := service.GroupMemoriesByConcept(memories)

	// m1 and m2 share "embeddings", so they should be in the same cluster
	assert.GreaterOrEqual(t, len(clusters), 1, "should produce at least one cluster")
	assert.LessOrEqual(t, len(clusters), 3, "should not produce more clusters than memories")

	// Find the cluster containing m1 and m2
	foundShared := false
	for _, cluster := range clusters {
		ids := make(map[string]bool)
		for _, m := range cluster.Memories {
			ids[m.ID] = true
		}
		if ids["m1"] && ids["m2"] {
			foundShared = true
			break
		}
	}
	assert.True(t, foundShared, "m1 and m2 share 'embeddings' concept and should be clustered together")
}

func TestGroupMemoriesByConcept_NoSharedConcepts(t *testing.T) {
	memories := []service.MemoryForReflection{
		{ID: "m1", Content: "A", Concepts: []string{"alpha"}},
		{ID: "m2", Content: "B", Concepts: []string{"beta"}},
		{ID: "m3", Content: "C", Concepts: []string{"gamma"}},
	}

	clusters := service.GroupMemoriesByConcept(memories)
	assert.Equal(t, 3, len(clusters), "memories with no shared concepts should each be their own cluster")
}

func TestDetectPatterns_ReturnsPatterns(t *testing.T) {
	cluster := service.MemoryCluster{
		Memories: []service.MemoryForReflection{
			{ID: "m1", Content: "Test A passed", Concepts: []string{"testing", "unit"}},
			{ID: "m2", Content: "Test B also passed", Concepts: []string{"testing", "integration"}},
		},
	}

	patterns := service.DetectPatterns(cluster)
	assert.NotEmpty(t, patterns, "should detect at least one pattern")
	// Patterns should mention shared concepts
	foundTesting := false
	for _, p := range patterns {
		if containsIgnoreCase(p.Pattern, "test") {
			foundTesting = true
			break
		}
	}
	assert.True(t, foundTesting, "detected patterns should reference shared concepts")
}

func TestDetectPatterns_EmptyCluster(t *testing.T) {
	cluster := service.MemoryCluster{
		Memories: []service.MemoryForReflection{},
	}

	patterns := service.DetectPatterns(cluster)
	assert.Empty(t, patterns, "empty cluster should produce no patterns")
}

func TestDetectPatterns_SingleMemory(t *testing.T) {
	cluster := service.MemoryCluster{
		Memories: []service.MemoryForReflection{
			{ID: "m1", Content: "Single memory", Concepts: []string{"solo"}},
		},
	}

	patterns := service.DetectPatterns(cluster)
	assert.Empty(t, patterns, "single memory should not produce patterns")
}

func TestSynthesizeInsight_ProducesLowConfidence(t *testing.T) {
	patterns := []service.DetectedPattern{
		{Pattern: "Frequent use of pgvector for embedding storage", Frequency: 5},
		{Pattern: "Recurring need for connection pool tuning", Frequency: 3},
	}

	insights := service.SynthesizeInsights(patterns)
	assert.NotEmpty(t, insights, "should synthesize at least one insight")

	for _, insight := range insights {
		assert.LessOrEqual(t, insight.Confidence, 0.5,
			"synthesized insights should have low confidence (<= 0.5)")
		assert.NotEmpty(t, insight.Content, "insight content should not be empty")
	}
}

func TestSynthesizeInsight_EmptyPatterns(t *testing.T) {
	insights := service.SynthesizeInsights(nil)
	assert.Empty(t, insights)

	insights = service.SynthesizeInsights([]service.DetectedPattern{})
	assert.Empty(t, insights)
}

// containsIgnoreCase is a case-insensitive substring check using the standard library.
func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
