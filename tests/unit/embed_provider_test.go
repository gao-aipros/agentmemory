package unit

import (
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clearEmbedEnv() {
	os.Unsetenv("EMBEDDING_PROVIDER")
	os.Unsetenv("EMBEDDING_MODEL")
	os.Unsetenv("EMBEDDING_API_KEY")
	os.Unsetenv("EMBEDDING_BASE_URL")
	os.Unsetenv("LLM_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
}

// TestNewEmbeddingService_OpenAI verifies EMBEDDING_PROVIDER=openai creates an OpenAI embedder.
func TestNewEmbeddingService_OpenAI(t *testing.T) {
	clearEmbedEnv()
	os.Setenv("EMBEDDING_PROVIDER", "openai")
	os.Setenv("EMBEDDING_MODEL", "text-embedding-3-small")
	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")

	// nil pool is fine — we just test provider creation, not DB operations
	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "Embedding embedder should not be nil")
}

// TestNewEmbeddingService_DefaultToOpenAI verifies EMBEDDING_PROVIDER defaults to openai.
func TestNewEmbeddingService_DefaultToOpenAI(t *testing.T) {
	clearEmbedEnv()
	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")
	// EMBEDDING_PROVIDER unset → defaults to "openai"

	svc, err := service.NewEmbeddingService(nil)
	require.NoError(t, err)
	require.NotNil(t, svc)
	assert.NotNil(t, svc.Embedder(), "default embedder should not be nil")
}

// TestNewEmbeddingService_InvalidProvider verifies invalid EMBEDDING_PROVIDER returns error.
func TestNewEmbeddingService_InvalidProvider(t *testing.T) {
	clearEmbedEnv()
	os.Setenv("EMBEDDING_PROVIDER", "nonexistent-provider")
	os.Setenv("OPENAI_API_KEY", "test-key-for-unit-test")

	svc, err := service.NewEmbeddingService(nil)
	assert.Error(t, err, "invalid embedding provider should return an error")
	assert.Nil(t, svc)
}

// TestNewEmbeddingService_MissingAPIKey verifies missing OPENAI_API_KEY returns error.
func TestNewEmbeddingService_MissingAPIKey(t *testing.T) {
	clearEmbedEnv()
	os.Setenv("EMBEDDING_PROVIDER", "openai")
	// OPENAI_API_KEY intentionally not set

	svc, err := service.NewEmbeddingService(nil)
	assert.Error(t, err, "missing API key should return an error")
	assert.Nil(t, svc)
}
