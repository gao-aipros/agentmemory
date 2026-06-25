package service

import (
	"strings"
)

// MemoryForReflection is a lightweight view of a memory used for reflection clustering.
type MemoryForReflection struct {
	ID       string
	Content  string
	Concepts []string
}

// MemoryCluster groups related memories that share concepts.
type MemoryCluster struct {
	Memories []MemoryForReflection
}

// GroupMemoriesByConcept clusters memories by shared concepts using
// a union-find style approach. Memories that share at least one concept
// are grouped into the same cluster.
func GroupMemoriesByConcept(memories []MemoryForReflection) []MemoryCluster {
	if len(memories) == 0 {
		return nil
	}

	// Build a concept → memory indices map
	conceptToMemories := make(map[string][]int)
	for i, m := range memories {
		for _, c := range m.Concepts {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" {
				conceptToMemories[c] = append(conceptToMemories[c], i)
			}
		}
	}

	// Union-Find to group memories by shared concepts
	parent := make([]int, len(memories))
	for i := range parent {
		parent[i] = i
	}

	var find func(x int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}

	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}

	// Union all memories that share a concept
	for _, indices := range conceptToMemories {
		if len(indices) <= 1 {
			continue
		}
		first := indices[0]
		for _, idx := range indices[1:] {
			union(first, idx)
		}
	}

	// Collect clusters
	clusterMap := make(map[int]*MemoryCluster)
	for i, m := range memories {
		root := find(i)
		if _, ok := clusterMap[root]; !ok {
			clusterMap[root] = &MemoryCluster{}
		}
		clusterMap[root].Memories = append(clusterMap[root].Memories, m)
	}

	clusters := make([]MemoryCluster, 0, len(clusterMap))
	for _, c := range clusterMap {
		clusters = append(clusters, *c)
	}

	return clusters
}

