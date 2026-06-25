package unit

import (
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

func TestGroupMemoriesByConcept_SingleMemory(t *testing.T) {
	memories := []service.MemoryForReflection{
		{ID: "m1", Content: "A single memory", Concepts: []string{"alpha"}},
	}

	clusters := service.GroupMemoriesByConcept(memories)
	assert.Equal(t, 1, len(clusters), "single memory should produce one cluster")
	assert.Equal(t, 1, len(clusters[0].Memories), "cluster should contain one memory")
	assert.Equal(t, "m1", clusters[0].Memories[0].ID)
}

func TestGroupMemoriesByConcept_NoConcepts(t *testing.T) {
	memories := []service.MemoryForReflection{
		{ID: "m1", Content: "Memory without concepts 1"},
		{ID: "m2", Content: "Memory without concepts 2"},
	}

	clusters := service.GroupMemoriesByConcept(memories)
	// Memories with no shared concepts cannot be grouped together.
	assert.Equal(t, 2, len(clusters), "memories without concepts should each be their own cluster")
}
