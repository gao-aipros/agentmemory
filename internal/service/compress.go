package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"golang.org/x/sync/semaphore"
)

// CompressionService picks up raw observations and compresses them into
// searchable compressed_observations using an LLM. Compression runs
// asynchronously — triggered by RecordObservation and completed
// within a target of 30 seconds.
type CompressionService struct {
	queries    *store.Queries
	llmService *LLMService
	embedSvc   *EmbeddingService
	sem        *semaphore.Weighted
}

// NewCompressionService creates a new CompressionService.
func NewCompressionService(pool *pgxpool.Pool, llm *LLMService, embedSvc *EmbeddingService) *CompressionService {
	return &CompressionService{
		queries:    store.New(pool),
		llmService: llm,
		embedSvc:   embedSvc,
		sem:        semaphore.NewWeighted(20),
	}
}

// TriggerAsync kicks off an asynchronous compression for the given observation.
// This is non-blocking — it launches a goroutine and returns immediately.
// Concurrent compressions are bounded by a semaphore to prevent unbounded
// goroutine growth.
func (s *CompressionService) TriggerAsync(ctx context.Context, obs *store.Observation) {
	go func() {
		// Acquire semaphore slot to bound concurrency.
		if err := s.sem.Acquire(context.Background(), 1); err != nil {
			return // context cancelled, server shutting down
		}
		defer s.sem.Release(1)

		// Recover from panics to prevent a single bad observation from
		// crashing the server.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("compression goroutine panicked", "panic", r)
			}
		}()

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

// CompressFile reads a markdown file, compresses it to reduce token usage,
// saves a .original.md backup, and writes the compressed version back.
// Compression preserves headings, URLs, and code blocks while reducing
// excessive whitespace and blank lines.
func (s *CompressionService) CompressFile(ctx context.Context, filePath string) error {
	if filePath == "" {
		return fmt.Errorf("file path is required")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create backup before compressing
	backupPath := strings.TrimSuffix(filePath, ".md") + ".original.md"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	compressed := compressMarkdownLines(lines)

	if err := os.WriteFile(filePath, []byte(compressed), 0644); err != nil {
		return fmt.Errorf("failed to write compressed file: %w", err)
	}

	slog.Info("compressed file", "path", filePath, "backup", backupPath,
		"original_bytes", len(data), "compressed_bytes", len(compressed))
	return nil
}

var blankLineRegex = regexp.MustCompile(`^\s*$`)

// compressMarkdownLines reduces excessive whitespace in markdown lines
// while preserving code blocks, headings, and link references.
func compressMarkdownLines(lines []string) string {
	var out strings.Builder
	inCodeBlock := false
	blankCount := 0

	for _, line := range lines {
		isBlank := blankLineRegex.MatchString(line)

		// Track code block fences
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}

		if inCodeBlock {
			// Preserve code blocks exactly
			out.WriteString(line)
			out.WriteByte('\n')
			blankCount = 0
			continue
		}

		if isBlank {
			blankCount++
			if blankCount <= 2 {
				out.WriteByte('\n')
			}
		} else {
			blankCount = 0
			// Trim trailing whitespace
			out.WriteString(strings.TrimRight(line, " \t"))
			out.WriteByte('\n')
		}
	}

	return out.String()
}
