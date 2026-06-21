package unit

import (
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearLLMEnv() {
	os.Unsetenv("LLM_PROVIDER")
	os.Unsetenv("LLM_MODEL")
	os.Unsetenv("OPENAI_API_KEY")
	os.Unsetenv("ANTHROPIC_API_KEY")
}

// TestNewLLMService_OpenAI verifies that NewLLMService creates an OpenAI model
// when LLM_PROVIDER=openai (or defaults to openai when unset).
func TestNewLLMService_OpenAI(t *testing.T) {
	clearLLMEnv()
	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")
	// LLM_PROVIDER defaults to "openai" when unset

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_OpenAI_Explicit verifies explicit LLM_PROVIDER=openai.
func TestNewLLMService_OpenAI_Explicit(t *testing.T) {
	clearLLMEnv()
	os.Setenv("LLM_PROVIDER", "openai")
	os.Setenv("LLM_MODEL", "gpt-4o-mini")
	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_Anthropic verifies LLM_PROVIDER=anthropic creates an Anthropic model.
func TestNewLLMService_Anthropic(t *testing.T) {
	clearLLMEnv()
	os.Setenv("LLM_PROVIDER", "anthropic")
	os.Setenv("LLM_MODEL", "claude-sonnet-4-20250514")
	os.Setenv("ANTHROPIC_API_KEY", "test-key-for-unit-test")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "Anthropic model should not be nil")
}

// TestNewLLMService_InvalidProvider verifies an invalid provider returns an error.
func TestNewLLMService_InvalidProvider(t *testing.T) {
	clearLLMEnv()
	os.Setenv("LLM_PROVIDER", "nonexistent-provider")

	svc, err := service.NewLLMService()
	assert.Error(t, err, "invalid provider should return an error")
	assert.Nil(t, svc, "service should be nil for invalid provider")
}

// TestNewLLMService_MissingAPIKey verifies missing API key returns an error.
func TestNewLLMService_MissingAPIKey(t *testing.T) {
	clearLLMEnv()
	os.Setenv("LLM_PROVIDER", "openai")
	// OPENAI_API_KEY intentionally not set

	// openai.New() will fail with ErrMissingToken when OPENAI_API_KEY is not set
	svc, err := service.NewLLMService()
	assert.Error(t, err, "missing API key should return an error")
	assert.Nil(t, svc)
}
