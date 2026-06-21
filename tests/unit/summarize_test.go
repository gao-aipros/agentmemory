package unit

import (
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimateTokens_ShortText(t *testing.T) {
	tokens := service.EstimateTokens("hello world")
	assert.Greater(t, tokens, 0, "even short text should estimate > 0 tokens")
	assert.LessOrEqual(t, tokens, 5, "two words should be roughly 2-3 tokens")
}

func TestEstimateTokens_LongerText(t *testing.T) {
	longText := "This is a much longer piece of text that should estimate to more tokens than a short one."
	shortTokens := service.EstimateTokens("short")
	longTokens := service.EstimateTokens(longText)
	assert.Greater(t, longTokens, shortTokens, "longer text should have more tokens")
}

func TestChunkObservations_SingleObservation(t *testing.T) {
	obs := []service.SummarizeObservation{
		{Title: "Test", Narrative: "A simple test observation"},
	}

	chunks := service.ChunkObservations(obs, 1000)
	require.Len(t, chunks, 1, "single observation should produce one chunk")
	assert.Len(t, chunks[0], 1, "chunk should contain the single observation")
}

func TestChunkObservations_MultipleObservations(t *testing.T) {
	obs := []service.SummarizeObservation{
		{Title: "Obs 1", Narrative: "First observation"},
		{Title: "Obs 2", Narrative: "Second observation"},
		{Title: "Obs 3", Narrative: "Third observation"},
	}

	chunks := service.ChunkObservations(obs, 1000)
	assert.GreaterOrEqual(t, len(chunks), 1, "should produce at least one chunk")
}

func TestChunkObservations_TokenBudgetSplitsIntoChunks(t *testing.T) {
	// Create enough observations to force chunking with a very small token budget
	obs := make([]service.SummarizeObservation, 100)
	for i := range obs {
		obs[i] = service.SummarizeObservation{
			Title:     "Observation with a reasonably long title for testing",
			Narrative: "This narrative has enough text to consume several tokens per observation, forcing chunk splits.",
		}
	}

	// Small token budget should force multiple chunks
	chunks := service.ChunkObservations(obs, 200)
	assert.Greater(t, len(chunks), 1, "small token budget should split into multiple chunks")

	// Large token budget should fit everything in one chunk
	chunksLarge := service.ChunkObservations(obs, 100000)
	assert.Equal(t, 1, len(chunksLarge), "large token budget should fit all in one chunk")
}

func TestChunkObservations_EmptyList(t *testing.T) {
	chunks := service.ChunkObservations(nil, 1000)
	assert.Empty(t, chunks, "nil observations should produce empty chunks")

	chunks = service.ChunkObservations([]service.SummarizeObservation{}, 1000)
	assert.Empty(t, chunks, "empty observations should produce empty chunks")
}

func TestBuildSummarizePrompt(t *testing.T) {
	obs := []service.SummarizeObservation{
		{Title: "User login", Narrative: "User logged into the system"},
		{Title: "Database query", Narrative: "User queried the sessions table"},
	}

	prompt := service.BuildSummarizePrompt(obs)
	assert.NotEmpty(t, prompt, "summarize prompt should not be empty")
	assert.Contains(t, prompt, "User login", "prompt should include first observation title")
	assert.Contains(t, prompt, "Database query", "prompt should include second observation title")
	assert.Contains(t, prompt, "User logged into the system", "prompt should include first narrative")
}

func TestBuildSummarizePrompt_SingleObservation(t *testing.T) {
	obs := []service.SummarizeObservation{
		{Title: "Solo", Narrative: "Only one observation"},
	}

	prompt := service.BuildSummarizePrompt(obs)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "Solo")
}
