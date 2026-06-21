package unit

import (
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestBuildConsolidationPrompt_FromSummary(t *testing.T) {
	summary := &service.ConsolidationInput{
		SessionID:    "sess-123",
		SummaryText:  "The agent discussed database schema design and implemented migration files.",
		Concepts:     []string{"database", "schema", "migrations"},
		Observations: 15,
	}

	prompt := service.BuildConsolidationPrompt(summary)

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "database schema design")
	assert.Contains(t, prompt, "database")
	assert.Contains(t, prompt, "migrations")

	promptLower := strings.ToLower(prompt)
	assert.True(t,
		strings.Contains(promptLower, "extract") ||
			strings.Contains(promptLower, "memory") ||
			strings.Contains(promptLower, "insight"),
		"consolidation prompt should instruct memory extraction")
}

func TestBuildConsolidationPrompt_EmptyConcepts(t *testing.T) {
	summary := &service.ConsolidationInput{
		SessionID:    "sess-456",
		SummaryText:  "Minimal session with no concepts extracted.",
		Concepts:     nil,
		Observations: 1,
	}

	prompt := service.BuildConsolidationPrompt(summary)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "Minimal session")
}

func TestParseConsolidationResponse_Valid(t *testing.T) {
	llmResponse := `{
		"memories": [
			{"content": "PostgreSQL schema uses pgvector for embeddings", "concepts": ["postgresql", "pgvector"]}
		],
		"lessons": [
			{"content": "Always run sqlc generate after changing .sql files", "context": "database workflow"}
		]
	}`

	result, err := service.ParseConsolidationResponse(llmResponse)
	assert.NoError(t, err)
	assert.Len(t, result.Memories, 1)
	assert.Len(t, result.Lessons, 1)
	assert.Equal(t, "PostgreSQL schema uses pgvector for embeddings", result.Memories[0].Content)
	assert.Equal(t, "Always run sqlc generate after changing .sql files", result.Lessons[0].Content)
}

func TestParseConsolidationResponse_InvalidJSON(t *testing.T) {
	_, err := service.ParseConsolidationResponse("not json")
	assert.Error(t, err)
}

func TestParseConsolidationResponse_EmptyLists(t *testing.T) {
	result, err := service.ParseConsolidationResponse(`{"memories": [], "lessons": []}`)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Memories)
	assert.Empty(t, result.Lessons)
}
