package integration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/service"
)

// MockLLMProvider implements service.LLMProvider for integration tests.
// Returns pre-configured responses for known prompt patterns.
type MockLLMProvider struct{}

// Call returns simulated LLM responses based on the prompt content.
// For compression: returns a compressed summary.
// For summarization: returns a session summary.
// For consolidation: returns extracted memories and lessons.
func (m *MockLLMProvider) Call(ctx context.Context, prompt string) (string, error) {
	// Compression prompt
	if contains(prompt, "Compress the following observation") {
		resp := service.CompressionResult{
			CompressedText: "Compressed: user interaction with database schema discussion",
			Concepts:       []string{"database", "schema", "postgresql"},
		}
		b, _ := json.Marshal(resp)
		return string(b), nil
	}

	// Summarization prompt
	if contains(prompt, "Summarize the following agent session observations") {
		return "Session summary: The agent discussed database architecture and implemented migration files. Key decisions included schema design and extension choices.", nil
	}

	// Consolidation prompt
	if contains(prompt, "Extract key insights") {
		resp := service.ConsolidationResult{
			Memories: []service.ExtractedMemory{
				{
					Content:  "PostgreSQL schema uses pgvector for embedding storage",
					Concepts: []string{"postgresql", "pgvector", "embeddings"},
				},
				{
					Content:  "Migrations are managed via golang-migrate",
					Concepts: []string{"migrations", "golang-migrate"},
				},
			},
			Lessons: []service.ExtractedLesson{
				{
					Content: "Always run sqlc generate after modifying .sql query files",
					Context: "database workflow",
				},
			},
		}
		b, _ := json.Marshal(resp)
		return string(b), nil
	}

	return "", fmt.Errorf("mock LLM: unrecognized prompt pattern")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure MockLLMProvider implements LLMProvider.
var _ service.LLMProvider = (*MockLLMProvider)(nil)
