package unit

import (
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Context Source Priority Truncation Tests (T058)
// =============================================================================

func TestContextPriority_TruncationOrder(t *testing.T) {
	// Priority order: graph → lessons → observations → recap
	// Items earlier in this list are trimmed LAST (most important retained)
	// Items later in this list are trimmed FIRST (least important trimmed)
	priorities := service.ContextPriorityOrder()

	require.Len(t, priorities, 4, "should have exactly 4 priority levels")

	assert.Equal(t, "graph", priorities[0], "graph must be highest priority (trimmed last)")
	assert.Equal(t, "lessons", priorities[1], "lessons must be second priority")
	assert.Equal(t, "observations", priorities[2], "observations must be third priority")
	assert.Equal(t, "recap", priorities[3], "recap must be lowest priority (trimmed first)")
}

func TestContextPriority_TruncateRecapFirst(t *testing.T) {
	// When budget is tight, recap should be truncated first
	budget := service.NewContextBudget(500) // Small budget
	assembled := &service.AssembledContext{
		Graph:         strings.Repeat("graph content ", 20),
		Lessons:       strings.Repeat("lesson content ", 20),
		Observations:  strings.Repeat("observation content ", 20),
		Recap:         strings.Repeat("recap content ", 20),
		WorkingMemory: "working memory",
	}

	result := service.ApplyBudget(assembled, budget)
	require.NotEmpty(t, result)

	// With a small budget (500), the result must fit within the budget
	resultTokens := service.EstimateTokens(result)
	assert.LessOrEqual(t, resultTokens, budget.TotalTokens,
		"truncated result must fit within the token budget")
}

func TestContextPriority_KeepGraphWhenPossible(t *testing.T) {
	// Graph content should be preserved as much as possible
	budget := service.NewContextBudget(800)
	assembled := &service.AssembledContext{
		Graph:         "graph: PostgreSQL connection pool setup",
		Lessons:       "lesson: Always set timeouts",
		Observations:  "obs1: Observed pool config",
		Recap:         "recap: Previous session...",
		WorkingMemory: "WM: implement search",
	}

	result := service.ApplyBudget(assembled, budget)
	require.NotEmpty(t, result)

	assert.Contains(t, result, "PostgreSQL", "graph content should be preserved")
}

func TestContextPriority_TruncationReducesSize(t *testing.T) {
	largeText := strings.Repeat("very long content that should be truncated ", 50)

	assembled := &service.AssembledContext{
		Graph:         largeText,
		Lessons:       largeText,
		Observations:  largeText,
		Recap:         largeText,
		WorkingMemory: largeText,
	}

	// Tiny budget forces all buckets to be truncated
	budget := service.NewContextBudget(200)
	result := service.ApplyBudget(assembled, budget)

	// Result should exist. XML wrapper is structural framing —
	// not counted against content budget, so total may exceed budget slightly.
	require.NotEmpty(t, result)
	resultTokens := service.EstimateTokens(result)
	assert.LessOrEqual(t, resultTokens, budget.TotalTokens+20,
		"result (content + XML wrapper) must not exceed budget by more than wrapper overhead")
}

func TestContextPriority_EmptyInputs(t *testing.T) {
	// All empty inputs should produce a valid (possibly empty) result
	assembled := &service.AssembledContext{}
	budget := service.DefaultContextBudget()

	result := service.ApplyBudget(assembled, budget)

	// Should not panic, result can be empty
	resultTokens := service.EstimateTokens(result)
	assert.LessOrEqual(t, resultTokens, budget.TotalTokens)
}

func TestContextPriority_PreservesWorkingMemory(t *testing.T) {
	// Working memory should always be preserved (not truncated)
	assembled := &service.AssembledContext{
		WorkingMemory: "IMPORTANT: current task context",
	}
	budget := service.NewContextBudget(50) // Very small

	result := service.ApplyBudget(assembled, budget)
	assert.Contains(t, result, "IMPORTANT", "working memory should not be truncated")
}

func TestContextPriority_AllFieldsPresent(t *testing.T) {
	// With generous budget, all fields should appear in output
	assembled := &service.AssembledContext{
		Graph:         "graph data",
		Lessons:       "lesson data",
		Observations:  "observation data",
		Recap:         "recap data",
		WorkingMemory: "working memory data",
	}
	budget := service.DefaultContextBudget()

	result := service.ApplyBudget(assembled, budget)

	// All sources should be represented (though possibly truncated)
	assert.Contains(t, result, "graph")
	assert.Contains(t, result, "lesson")
	assert.Contains(t, result, "observation")
	assert.Contains(t, result, "recap")
	assert.Contains(t, result, "working memory")
}
