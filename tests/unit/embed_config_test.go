package unit

import (
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewEmbeddingService_EmbeddingAPIKey_Used verifies that EMBEDDING_API_KEY
// alone (without LLM_API_KEY or OPENAI_API_KEY) works.
func TestNewEmbeddingService_EmbeddingAPIKey_Used(t *testing.T) {
	clearEmbedEnv()

	os.Setenv("EMBEDDING_API_KEY", "my-embed-key")
	// LLM_API_KEY and OPENAI_API_KEY are NOT set — EMBEDDING_API_KEY should be used

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}

// TestNewEmbeddingService_EmbeddingAPIKey_FallbackToLLMAPIKey verifies that
// when EMBEDDING_API_KEY is empty but LLM_API_KEY is set, the LLM key is used.
func TestNewEmbeddingService_EmbeddingAPIKey_FallbackToLLMAPIKey(t *testing.T) {
	clearEmbedEnv()

	// EMBEDDING_API_KEY is intentionally not set
	os.Setenv("LLM_API_KEY", "llm-fallback-key")
	// OPENAI_API_KEY is NOT set either

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}

// TestNewEmbeddingService_EmbeddingAPIKey_FallbackToOpenAI verifies that when
// both EMBEDDING_API_KEY and LLM_API_KEY are empty but OPENAI_API_KEY is set,
// the fallback works.
func TestNewEmbeddingService_EmbeddingAPIKey_FallbackToOpenAI(t *testing.T) {
	clearEmbedEnv()

	// EMBEDDING_API_KEY and LLM_API_KEY are intentionally not set
	os.Setenv("OPENAI_API_KEY", "openai-fallback-key")

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}

// TestNewEmbeddingService_NoKey_Error verifies that when all API key env vars
// are empty, NewEmbeddingService returns an error.
func TestNewEmbeddingService_NoKey_Error(t *testing.T) {
	clearEmbedEnv()

	// None of EMBEDDING_API_KEY, LLM_API_KEY, or OPENAI_API_KEY is set

	svc, err := service.NewEmbeddingService(nil)
	assert.Error(t, err, "missing all API keys should return an error")
	assert.Nil(t, svc, "service should be nil when API key is missing")
}

// TestNewEmbeddingService_EmbeddingBaseURL_Set verifies that the service is
// created successfully when EMBEDDING_BASE_URL is set (without OPENAI_API_KEY,
// using EMBEDDING_API_KEY instead).
func TestNewEmbeddingService_EmbeddingBaseURL_Set(t *testing.T) {
	clearEmbedEnv()

	os.Setenv("EMBEDDING_API_KEY", "test-key-for-unit-test")
	os.Setenv("EMBEDDING_BASE_URL", "https://custom-openai.example.com/v1")

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}

// TestNewEmbeddingService_EmbeddingBaseURL_Empty verifies that when
// EMBEDDING_BASE_URL is not set, the service is still created successfully.
func TestNewEmbeddingService_EmbeddingBaseURL_Empty(t *testing.T) {
	clearEmbedEnv()

	os.Setenv("EMBEDDING_API_KEY", "test-key-for-unit-test")
	// EMBEDDING_BASE_URL is intentionally not set

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}
