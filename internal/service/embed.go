package service

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/openai"
)

// EmbeddingService generates and stores vector embeddings for observations
// and compressed observations. Uses langchaingo embeddings.Embedder for
// provider agnosticism.
type EmbeddingService struct {
	queries  *store.Queries
	embedder embeddings.Embedder
}

// NewEmbeddingService creates an EmbeddingService from environment variables.
// EMBEDDING_PROVIDER: "openai" (default, currently the only supported provider).
// EMBEDDING_MODEL: the embedding model name (provider-specific default if unset).
// OPENAI_API_KEY: required for OpenAI embeddings.
// pool may be nil for testing provider creation only.
func NewEmbeddingService(pool *pgxpool.Pool) (*EmbeddingService, error) {
	provider := strings.ToLower(os.Getenv("EMBEDDING_PROVIDER"))
	if provider == "" {
		provider = "openai"
	}

	var embedder embeddings.Embedder
	var err error

	switch provider {
	case "openai":
		embedder, err = newOpenAIEmbedder()
	default:
		return nil, fmt.Errorf("unsupported EMBEDDING_PROVIDER %q: must be \"openai\"", provider)
	}

	if err != nil {
		return nil, err
	}

	svc := &EmbeddingService{
		embedder: embedder,
	}
	if pool != nil {
		svc.queries = store.New(pool)
	}
	return svc, nil
}

// NewEmbeddingServiceWithEmbedder creates an EmbeddingService with a pre-built embedder.
// Used in tests to inject mock embedders without requiring real API keys.
func NewEmbeddingServiceWithEmbedder(pool *pgxpool.Pool, embedder embeddings.Embedder) *EmbeddingService {
	svc := &EmbeddingService{
		embedder: embedder,
	}
	if pool != nil {
		svc.queries = store.New(pool)
	}
	return svc
}

// Embedder returns the underlying langchaingo embeddings.Embedder (for testing/assertions).
func (s *EmbeddingService) Embedder() embeddings.Embedder {
	return s.embedder
}

// GenerateEmbedding returns a vector embedding for the given text.
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.embedder == nil {
		return nil, fmt.Errorf("embedding service not configured — set EMBEDDING_PROVIDER and required API key env vars")
	}
	return s.embedder.EmbedQuery(ctx, text)
}

// StoreObservationEmbedding inserts an embedding for a raw observation.
func (s *EmbeddingService) StoreObservationEmbedding(ctx context.Context, observationID string, embedding []float32, model string) error {
	if s.queries == nil {
		return fmt.Errorf("embedding service has no database pool")
	}
	vec := pgvector.NewVector(embedding)
	return s.queries.InsertEmbedding(ctx, store.InsertEmbeddingParams{
		ObservationID: observationID,
		Embedding:     &vec,
		Model:         model,
	})
}

// StoreCompressedEmbedding inserts an embedding for a compressed observation.
func (s *EmbeddingService) StoreCompressedEmbedding(ctx context.Context, compressedID string, embedding []float32, model string) error {
	if s.queries == nil {
		return fmt.Errorf("embedding service has no database pool")
	}
	vec := pgvector.NewVector(embedding)
	return s.queries.InsertCompressedEmbedding(ctx, store.InsertCompressedEmbeddingParams{
		CompressedID: compressedID,
		Embedding:     &vec,
		Model:         model,
	})
}

// newOpenAIEmbedder creates an OpenAI embedder from environment variables.
func newOpenAIEmbedder() (embeddings.Embedder, error) {
	opts := []openai.Option{}

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
