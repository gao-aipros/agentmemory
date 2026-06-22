package unit

import (
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewLLMService_LLMAPIKey_Used verifies that LLM_API_KEY alone
// (without provider-specific env vars) works for the openai provider.
func TestNewLLMService_LLMAPIKey_Used(t *testing.T) {
	clearLLMEnv()

	os.Setenv("LLM_API_KEY", "my-unified-key")
	os.Setenv("LLM_PROVIDER", "openai")
	// OPENAI_API_KEY is NOT set — LLM_API_KEY should be used

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_LLMAPIKey_FallbackToOpenAI verifies that when LLM_API_KEY
// is empty but OPENAI_API_KEY is set, the fallback works for openai provider.
func TestNewLLMService_LLMAPIKey_FallbackToOpenAI(t *testing.T) {
	clearLLMEnv()

	// LLM_API_KEY is intentionally not set
	os.Setenv("OPENAI_API_KEY", "openai-fallback-key")
	os.Setenv("LLM_PROVIDER", "openai")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_LLMAPIKey_FallbackToAnthropic verifies that when LLM_API_KEY
// is empty but ANTHROPIC_API_KEY is set, the fallback works for anthropic provider.
func TestNewLLMService_LLMAPIKey_FallbackToAnthropic(t *testing.T) {
	clearLLMEnv()

	// LLM_API_KEY is intentionally not set
	os.Setenv("ANTHROPIC_API_KEY", "anthropic-fallback-key")
	os.Setenv("LLM_PROVIDER", "anthropic")
	os.Setenv("LLM_MODEL", "claude-sonnet-4-20250514")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "Anthropic model should not be nil")
}

// TestNewLLMService_LLMAPIKey_PriorityOverProviderKey verifies that when both
// LLM_API_KEY and OPENAI_API_KEY are set, LLM_API_KEY takes priority (service
// is created successfully, proving no conflict).
func TestNewLLMService_LLMAPIKey_PriorityOverProviderKey(t *testing.T) {
	clearLLMEnv()

	os.Setenv("LLM_API_KEY", "primary-key")
	os.Setenv("OPENAI_API_KEY", "secondary-key")
	os.Setenv("LLM_PROVIDER", "openai")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_LLMAPIKey_Empty verifies that when both LLM_API_KEY and
// provider-specific keys are empty, the service returns an error.
func TestNewLLMService_LLMAPIKey_Empty(t *testing.T) {
	clearLLMEnv()

	// Neither LLM_API_KEY nor OPENAI_API_KEY is set
	os.Setenv("LLM_PROVIDER", "openai")

	svc, err := service.NewLLMService()
	assert.Error(t, err, "missing API key should return an error")
	assert.Nil(t, svc, "service should be nil when API key is missing")
}

// TestNewLLMService_LLMBaseURL_Set verifies that the service is created
// successfully when LLM_BASE_URL is set (proving it doesn't break creation).
func TestNewLLMService_LLMBaseURL_Set(t *testing.T) {
	clearLLMEnv()

	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")
	os.Setenv("LLM_BASE_URL", "https://custom-openai.example.com/v1")
	os.Setenv("LLM_PROVIDER", "openai")

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}

// TestNewLLMService_LLMBaseURL_Empty verifies that when LLM_BASE_URL is not
// set, the service is still created successfully.
func TestNewLLMService_LLMBaseURL_Empty(t *testing.T) {
	clearLLMEnv()

	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")
	os.Setenv("LLM_PROVIDER", "openai")
	// LLM_BASE_URL is intentionally not set

	svc, err := service.NewLLMService()
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Model(), "LLM model should not be nil")
}
