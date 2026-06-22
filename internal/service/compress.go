package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// CompressionService picks up raw observations and compresses them into
// searchable compressed_observations using an LLM. Compression runs
// asynchronously — triggered by RecordObservation and completed
// within a target of 30 seconds.
type CompressionService struct {
	queries    *store.Queries
	llmService *LLMService
	embedSvc   *EmbeddingService
}

// NewCompressionService creates a new CompressionService.
func NewCompressionService(pool *pgxpool.Pool, llm *LLMService, embedSvc *EmbeddingService) *CompressionService {
	return &CompressionService{
		queries:    store.New(pool),
		llmService: llm,
		embedSvc:   embedSvc,
	}
}

// TriggerAsync kicks off an asynchronous compression for the given observation.
// This is non-blocking — it launches a goroutine and returns immediately.
func (s *CompressionService) TriggerAsync(ctx context.Context, obs *store.Observation) {
	go func() {
		// Use a background context with timeout so the goroutine doesn't
		// outlive the request context.
		bgCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.compress(bgCtx, obs); err != nil {
			slog.Warn("compression failed for observation",
				"observation_id", obs.ID,
				"error", err,
			)
		}
	}()
}

// compress performs the actual compression work: build prompt, call LLM,
// store the compressed observation and its embedding.
func (s *CompressionService) compress(ctx context.Context, obs *store.Observation) error {
	// Build the prompt
	facts := ""
	if obs.Facts != nil {
		facts = *obs.Facts
	}
	prompt := BuildCompressionPrompt(obs.Title, obs.Narrative, facts, obs.Concepts)

	// Call LLM
	response, err := s.llmService.Call(ctx, prompt)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	compressedText, concepts, err := ParseCompressionResponse(response)
	if err != nil {
		return fmt.Errorf("failed to parse LLM response: %w", err)
	}

	// Store compressed observation
	compressedID := uuid.New().String()
	compParams := store.InsertCompressedObservationParams{
		ID:             compressedID,
		ObservationIds: []string{obs.ID},
		SessionID:      obs.SessionID,
		Visibility:     obs.Visibility,
		CompressedText: compressedText,
		Concepts:       concepts,
	}

	_, err = s.queries.InsertCompressedObservation(ctx, compParams)
	if err != nil {
		return fmt.Errorf("failed to insert compressed observation: %w", err)
	}

	// Generate and store embedding (if embed service is available)
	if s.embedSvc != nil {
		embedding, err := s.embedSvc.GenerateEmbedding(ctx, compressedText)
		if err != nil {
			slog.Warn("failed to generate embedding for compressed observation",
				"compressed_id", compressedID,
				"error", err,
			)
			// Non-fatal — we still have the compressed text
		} else {
			vec := pgvector.NewVector(embedding)
			if err := s.queries.InsertCompressedEmbedding(ctx, store.InsertCompressedEmbeddingParams{
				CompressedID: compressedID,
				Embedding:    &vec,
				Model:        "default",
			}); err != nil {
				slog.Warn("failed to store compressed embedding",
					"compressed_id", compressedID,
					"error", err,
				)
			}
		}
	}

	return nil
}
