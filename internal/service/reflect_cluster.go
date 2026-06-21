package service

import (
	"fmt"
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

// DetectedPattern is a recurring pattern found across memory clusters.
type DetectedPattern struct {
	Pattern   string
	Frequency int
}

// SynthesizedInsight is a higher-order insight with low initial confidence,
// intended for human review before being promoted to a full memory.
type SynthesizedInsight struct {
	Content    string
	Confidence float64
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

// DetectPatterns analyzes a memory cluster and returns detected recurring patterns.
// Only clusters with 2+ memories produce patterns. Empty or single-memory clusters
// produce no patterns.
func DetectPatterns(cluster MemoryCluster) []DetectedPattern {
	if len(cluster.Memories) < 2 {
		return nil
	}

	// Count concept frequency within the cluster
	conceptFreq := make(map[string]int)
	for _, m := range cluster.Memories {
		for _, c := range m.Concepts {
			c = strings.ToLower(strings.TrimSpace(c))
			if c != "" {
				conceptFreq[c]++
			}
		}
	}

	// Produce patterns for concepts that appear multiple times
	var patterns []DetectedPattern
	for concept, freq := range conceptFreq {
		if freq >= 2 {
			patterns = append(patterns, DetectedPattern{
				Pattern:   fmt.Sprintf("Recurring focus on %s across multiple sessions", concept),
				Frequency: freq,
			})
		}
	}

	// If no concept-based patterns, produce a generic pattern
	if len(patterns) == 0 {
		patterns = append(patterns, DetectedPattern{
			Pattern:   fmt.Sprintf("Cluster of %d related memories", len(cluster.Memories)),
			Frequency: len(cluster.Memories),
		})
	}

	return patterns
}

// SynthesizeInsights converts detected patterns into higher-order insights
// with low initial confidence, intended for human review.
func SynthesizeInsights(patterns []DetectedPattern) []SynthesizedInsight {
	if len(patterns) == 0 {
		return nil
	}

	insights := make([]SynthesizedInsight, 0, len(patterns))
	for _, p := range patterns {
		confidence := 0.3 // Low base confidence — needs human review
		if p.Frequency >= 5 {
			confidence = 0.5 // Slightly higher confidence for frequent patterns
		}

		insights = append(insights, SynthesizedInsight{
			Content:    p.Pattern,
			Confidence: confidence,
		})
	}

	return insights
}
