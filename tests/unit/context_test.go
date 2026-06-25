package unit

import (
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Context Injection Budget Allocation Tests (T057)
// =============================================================================

func TestContextBudget_DefaultTokenLimit(t *testing.T) {
	budget := service.DefaultContextBudget()
	assert.Equal(t, 2000, budget.TotalTokens, "default budget should be 2000 tokens")
	assert.Equal(t, 1466, budget.SourceTokens, "source content should be ~1466 tokens")
	assert.Equal(t, 534, budget.OverheadTokens, "overhead should be ~534 tokens")
}

func TestContextBudget_TotalEqualsSourcePlusOverhead(t *testing.T) {
	budget := service.DefaultContextBudget()
	assert.Equal(t, budget.TotalTokens, budget.SourceTokens+budget.OverheadTokens,
		"total tokens must equal source + overhead")
}

func TestContextBudget_CustomLimit(t *testing.T) {
	budget := service.NewContextBudget(2000)
	assert.Equal(t, 2000, budget.TotalTokens)
	// Source tokens should be proportional: ~73% of total (2000 * 1466/2000 = 1466)
	expectedSource := 1466
	assert.InDelta(t, expectedSource, budget.SourceTokens, 5)
}

func TestContextBudget_SourceTokenAllocation(t *testing.T) {
	budget := service.DefaultContextBudget()

	// Verify the 5 source buckets get reasonable allocations
	alloc := budget.BucketAllocation()

	assert.Greater(t, alloc.Graph, 0, "graph should have some allocation")
	assert.Greater(t, alloc.Lessons, 0, "lessons should have some allocation")
	assert.Greater(t, alloc.Observations, 0, "observations should have some allocation")
	assert.Greater(t, alloc.Recap, 0, "recap should have some allocation")
	assert.Greater(t, alloc.WorkingMemory, 0, "working memory should have some allocation")

	totalAllocated := alloc.Graph + alloc.Lessons + alloc.Observations +
		alloc.Recap + alloc.WorkingMemory
	assert.LessOrEqual(t, totalAllocated, budget.SourceTokens,
		"total bucket allocation must not exceed source token budget")
}

func TestContextBudget_EstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minCount int
		maxCount int
	}{
		{"empty", "", 0, 0},
		{"single_word", "hello", 1, 3},
		{"three_words", "hello world again", 2, 6},
		{"long_text", "the quick brown fox jumps over the lazy dog", 8, 18},
		{"code_snippet", "func Foo(x int) int { return x * 2 }", 8, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := service.EstimateTokens(tt.text)
			assert.GreaterOrEqual(t, count, tt.minCount,
				"token count should be >= min for %q", tt.text)
			assert.LessOrEqual(t, count, tt.maxCount,
				"token count should be <= max for %q", tt.text)
		})
	}
}

func TestContextBudget_EstimateTokensEmptyString(t *testing.T) {
	assert.Equal(t, 0, service.EstimateTokens(""))
}

func TestContextBudget_MultipleSentences(t *testing.T) {
	text := "The PostgreSQL connection pool was configured. We set max connections to 25. The team decided on shared visibility."
	tokens := service.EstimateTokens(text)
	// ~15 words * 1.3 = ~20 tokens
	assert.Greater(t, tokens, 10)
	assert.Less(t, tokens, 40)
}

func TestApplyBudget_WrapsOutputInXmlContextTag(t *testing.T) {
	budget := service.DefaultContextBudget()
	assembled := &service.AssembledContext{
		Graph:         "node1: PostgreSQL connection pool",
		Lessons:       "lesson1: Always set connection timeout",
		Observations:  "obs1: Observed connection pool settings",
		Recap:         "Previous session: database infrastructure",
		WorkingMemory: "Current task: search feature",
	}

	result := service.ApplyBudget(assembled, budget)
	require.NotEmpty(t, result)

	// Must start with opening XML tag
	assert.Contains(t, result, "<agentmemory-context",
		"output must open with <agentmemory-context> tag")
	assert.Contains(t, result, "version=\"2\"",
		"output must include version=\"2\" attribute")

	// Must end with closing XML tag
	assert.Contains(t, result, "</agentmemory-context>",
		"output must close with </agentmemory-context> tag")

	// Opening tag must precede closing tag
	openIdx := strings.Index(result, "<agentmemory-context")
	closeIdx := strings.Index(result, "</agentmemory-context>")
	assert.Greater(t, closeIdx, openIdx,
		"closing tag must appear after opening tag")

	// Content between tags should still contain markdown headers
	assert.Contains(t, result, "### Context (AgentMemory v2)",
		"inner markdown structure must be preserved")
}

func TestContextBudget_RespectsTotalLimit(t *testing.T) {
	// Build a context block that would exceed the budget
	budget := service.DefaultContextBudget()
	assembled := &service.AssembledContext{
		Graph:         "node1: PostgreSQL connection pool (5 related observations)",
		Lessons:       "lesson1: Always set connection timeout to 30s",
		Observations:  "obs1: Observed connection pool settings on 2026-06-21",
		Recap:         "Previous session summary: Set up database infrastructure...",
		WorkingMemory: "Current task: implementing search feature",
	}

	result := service.ApplyBudget(assembled, budget)
	require.NotEmpty(t, result)

	// Verify result doesn't exceed total token budget
	resultTokens := service.EstimateTokens(result)
	assert.LessOrEqual(t, resultTokens, budget.TotalTokens,
		"applied context must respect total token budget, got %d tokens for %d budget",
		resultTokens, budget.TotalTokens)
}
