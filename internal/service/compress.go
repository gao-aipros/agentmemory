package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CompressionService provides file-level compression utilities.
// Batch compression of observations is now handled by the Scheduler pipeline.
type CompressionService struct{}

// NewCompressionService creates a new CompressionService.
func NewCompressionService(pool *pgxpool.Pool, llm *LLMService, embedSvc *EmbeddingService) *CompressionService {
	return &CompressionService{}
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
