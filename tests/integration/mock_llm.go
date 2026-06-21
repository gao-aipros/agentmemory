package integration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/tmc/langchaingo/llms"
)

// MockLLMProvider implements llms.Model for integration tests.
// Returns pre-configured responses for known prompt patterns.
// Deprecated: retained for backward compatibility in tests that create services manually.
type MockLLMProvider struct{}

// Call returns simulated LLM responses based on the prompt content.
func (m *MockLLMProvider) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
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

// GenerateContent implements llms.Model.GenerateContent for the mock.
func (m *MockLLMProvider) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// Extract text from the last message and delegate to Call
	var prompt string
	for _, msg := range messages {
		for _, part := range msg.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				prompt += p.Text
			}
		}
	}
	response, err := m.Call(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{
			{Content: response},
		},
	}, nil
}

// Ensure MockLLMProvider implements llms.Model.
var _ llms.Model = (*MockLLMProvider)(nil)

// NewMockLLMService creates an LLMService with the mock provider for integration tests.
func NewMockLLMService() *service.LLMService {
	return service.NewLLMServiceWithModel(&MockLLMProvider{})
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
