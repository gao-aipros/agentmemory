package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"
)

// EmbeddingService generates vector embeddings for observations
// and compressed observations. Uses langchaingo embeddings.Embedder for
// provider agnosticism.
type EmbeddingService struct {
	embedder embeddings.Embedder
}

// NewEmbeddingService creates an EmbeddingService from environment variables.
// EMBEDDING_PROVIDER: "openai-compatible" (default) — any backend speaking the
//   OpenAI embeddings API. Use EMBEDDING_BASE_URL to point at a non-OpenAI backend.
//   "openai" is accepted as a backward-compatible alias.
// EMBEDDING_MODEL: the embedding model name (provider-specific default if unset).
// EMBEDDING_API_KEY: API key for the embedding provider.
func NewEmbeddingService() (*EmbeddingService, error) {
	provider := strings.ToLower(os.Getenv("EMBEDDING_PROVIDER"))
	if provider == "" {
		provider = "openai-compatible"
	}

	var embedder embeddings.Embedder
	var err error

	switch provider {
	case "openai-compatible", "openai":
		embedder, err = newOpenAIEmbedder()
	default:
		return nil, fmt.Errorf("unsupported EMBEDDING_PROVIDER %q: must be \"openai-compatible\"", provider)
	}

	if err != nil {
		return nil, err
	}

	svc := &EmbeddingService{
		embedder: embedder,
	}
	return svc, nil
}

// NewEmbeddingServiceWithEmbedder creates an EmbeddingService with a pre-built embedder.
// Used in tests to inject mock embedders without requiring real API keys.
func NewEmbeddingServiceWithEmbedder(embedder embeddings.Embedder) *EmbeddingService {
	return &EmbeddingService{
		embedder: embedder,
	}
}

// Embedder returns the underlying langchaingo embeddings.Embedder (for testing/assertions).
func (s *EmbeddingService) Embedder() embeddings.Embedder {
	return s.embedder
}

// GenerateEmbedding returns a vector embedding for the given text.
// Uses RetryWithBackoff to handle transient API errors with exponential backoff.
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("embedding service not configured — set EMBEDDING_PROVIDER and required API key env vars")
	}
	var result []float32
	err := RetryWithBackoff(ctx, 3, 500*time.Millisecond, 10*time.Second, func() error {
		var innerErr error
		result, innerErr = s.embedder.EmbedQuery(ctx, text)
		return innerErr
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveEmbeddingAPIKey returns the effective API key for embedding.
// Priority: EMBEDDING_API_KEY > LLM_API_KEY > OPENAI_API_KEY > empty string.
func resolveEmbeddingAPIKey() string {
	if key := os.Getenv("EMBEDDING_API_KEY"); key != "" {
		return key
	}
	if key := os.Getenv("LLM_API_KEY"); key != "" {
		return key
	}
	return os.Getenv("OPENAI_API_KEY")
}

// resolveEmbeddingBaseURL returns the EMBEDDING_BASE_URL environment variable,
// or empty if not set.
func resolveEmbeddingBaseURL() string {
	return os.Getenv("EMBEDDING_BASE_URL")
}

// newOpenAIEmbedder creates an OpenAI embedder from environment variables.
func newOpenAIEmbedder() (embeddings.Embedder, error) {
	opts := []openai.Option{}

	if token := resolveEmbeddingAPIKey(); token != "" {
		opts = append(opts, openai.WithToken(token))
	}

	if baseURL := resolveEmbeddingBaseURL(); baseURL != "" {
		opts = append(opts, openai.WithBaseURL(baseURL))
	}

	if model := os.Getenv("EMBEDDING_MODEL"); model != "" {
		opts = append(opts, openai.WithEmbeddingModel(model))
	}

	llm, err := openai.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI client for embeddings: %w", err)
	}

	embedder, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedder: %w", err)
	}

	return embedder, nil
}
