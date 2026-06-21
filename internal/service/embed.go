package service

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// EmbeddingService generates and stores vector embeddings for observations
// and compressed observations. Uses langchaingo embedder interface for
// provider agnosticism.
type EmbeddingService struct {
	queries  *store.Queries
	provider EmbeddingProvider
}

// EmbeddingProvider abstracts the embedding backend so we stay provider-agnostic.
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewEmbeddingService creates a new EmbeddingService.
func NewEmbeddingService(pool *pgxpool.Pool, provider EmbeddingProvider) *EmbeddingService {
	return &EmbeddingService{
		queries:  store.New(pool),
		provider: provider,
	}
}

// GenerateEmbedding returns a vector embedding for the given text.
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	if s.provider == nil {
		return nil, fmt.Errorf("embedding service not configured — set EMBEDDING_PROVIDER and EMBEDDING_MODEL env vars")
	}
	return s.provider.Embed(ctx, text)
}

// StoreObservationEmbedding inserts an embedding for a raw observation.
func (s *EmbeddingService) StoreObservationEmbedding(ctx context.Context, observationID string, embedding []float32, model string) error {
	vec := pgvector.NewVector(embedding)
	return s.queries.InsertEmbedding(ctx, store.InsertEmbeddingParams{
		ObservationID: observationID,
		Embedding:     &vec,
		Model:         model,
	})
}

// StoreCompressedEmbedding inserts an embedding for a compressed observation.
func (s *EmbeddingService) StoreCompressedEmbedding(ctx context.Context, compressedID string, embedding []float32, model string) error {
	vec := pgvector.NewVector(embedding)
	return s.queries.InsertCompressedEmbedding(ctx, store.InsertCompressedEmbeddingParams{
		CompressedID: compressedID,
		Embedding:     &vec,
		Model:         model,
	})
}
