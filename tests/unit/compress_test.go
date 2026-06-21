package unit

import (
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCompressionPrompt_IncludesObservationFields(t *testing.T) {
	title := "User asked about database schema"
	narrative := "The user wanted to understand the PostgreSQL schema for the memory platform."
	facts := "Database uses PostgreSQL with pgvector extension"
	concepts := []string{"postgresql", "schema", "pgvector"}

	prompt := service.BuildCompressionPrompt(title, narrative, facts, concepts)

	assert.NotEmpty(t, prompt, "compression prompt should not be empty")
	assert.Contains(t, prompt, title, "prompt should include the observation title")
	assert.Contains(t, prompt, narrative, "prompt should include the observation narrative")
	assert.Contains(t, prompt, facts, "prompt should include the observation facts")
	assert.Contains(t, prompt, concepts[0], "prompt should include concepts")
	assert.Contains(t, prompt, concepts[1], "prompt should include concepts")
	assert.Contains(t, prompt, concepts[2], "prompt should include concepts")
}

func TestBuildCompressionPrompt_MissingOptionalFields(t *testing.T) {
	title := "Simple observation"
	narrative := "Just a test"

	// Should not panic or error with empty facts/concepts
	prompt := service.BuildCompressionPrompt(title, narrative, "", nil)
	assert.NotEmpty(t, prompt, "prompt should be generated even with missing optional fields")
	assert.Contains(t, prompt, title)
	assert.Contains(t, prompt, narrative)
}

func TestBuildCompressionPrompt_HasExpectedPromptFormat(t *testing.T) {
	prompt := service.BuildCompressionPrompt("Test Title", "Test Narrative", "Some facts", []string{"concept1"})

	// The prompt should instruct the LLM to compress/summarize
	promptLower := strings.ToLower(prompt)
	assert.True(t,
		strings.Contains(promptLower, "summarize") || strings.Contains(promptLower, "compress") ||
			strings.Contains(promptLower, "summary") || strings.Contains(promptLower, "concise"),
		"prompt should instruct compression, got: %s", prompt)
}

func TestParseCompressionResponse_ValidJSON(t *testing.T) {
	llmResponse := `{"compressed_text": "User inquired about the database schema.", "concepts": ["database", "schema"]}`

	text, concepts, err := service.ParseCompressionResponse(llmResponse)
	require.NoError(t, err)
	assert.Equal(t, "User inquired about the database schema.", text)
	assert.Equal(t, []string{"database", "schema"}, concepts)
}

func TestParseCompressionResponse_InvalidJSON(t *testing.T) {
	_, _, err := service.ParseCompressionResponse("not valid json")
	assert.Error(t, err, "should return error for invalid JSON")
}

func TestParseCompressionResponse_MissingFields(t *testing.T) {
	_, _, err := service.ParseCompressionResponse(`{}`)
	assert.Error(t, err, "should return error when required fields are missing")
}
