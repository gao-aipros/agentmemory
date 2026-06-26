package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// MemoryService handles explicit save-memory operations.
// Saved memories bypass the observe->compress pipeline and go straight
// to the memories table with associated vector embeddings for semantic search.
type MemoryService struct {
	queries  *store.Queries
	embedSvc *EmbeddingService
}

// NewMemoryService creates a new MemoryService.
func NewMemoryService(pool *pgxpool.Pool, embedSvc *EmbeddingService) *MemoryService {
	return &MemoryService{
		queries:  store.New(pool),
		embedSvc: embedSvc,
	}
}

// SaveMemory persists a manually saved memory with its embedding.
// Steps:
//  1. Generate an embedding for the content (best-effort — failure logs a warning)
//  2. Insert the memory record into the memories table with source="manual_save"
//  3. Insert the embedding into memory_embeddings if generation succeeded
//
// Returns the created Memory.
func (s *MemoryService) SaveMemory(ctx context.Context, content string, concepts []string, ownerUserID string) (*store.Memory, error) {
	id := uuid.New().String()

	// Insert the memory record (OwnerUserID only set when non-empty)
	var ownerUserIDPtr *string
	if ownerUserID != "" {
		ownerUserIDPtr = &ownerUserID
	}
	mem, err := s.queries.InsertMemory(ctx, store.InsertMemoryParams{
		ID:          id,
		OwnerType:   "user",
		OwnerUserID: ownerUserIDPtr,
		Visibility:  "private",
		Content:     content,
		Concepts:    concepts,
		Source:      "manual_save",
		Confidence:  0.8,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to insert memory: %w", err)
	}

	// Attempt to generate and store embedding (best-effort)
	if s.embedSvc != nil && s.embedSvc.Embedder() != nil {
		embedding, err := s.embedSvc.GenerateEmbedding(ctx, content)
		if err != nil {
			slog.Warn("failed to generate embedding for memory, continuing without embedding",
				"memory_id", id, "error", err)
		} else {
			vec := pgvector.NewVector(embedding)
			model := resolveEmbeddingModel()
			if err := s.queries.InsertMemoryEmbedding(ctx, store.InsertMemoryEmbeddingParams{
				ID:        uuid.New().String(),
				MemoryID:  id,
				Embedding: &vec,
				Model:     model,
			}); err != nil {
				slog.Warn("failed to insert memory embedding, continuing without embedding",
					"memory_id", id, "error", err)
			}
		}
	}

	return &mem, nil
}

// resolveEmbeddingModel returns the embedding model name from environment,
// defaulting to "text-embedding-ada-002".
func resolveEmbeddingModel() string {
	if model := os.Getenv("EMBEDDING_MODEL"); model != "" {
		return model
	}
	return "text-embedding-ada-002"
}
