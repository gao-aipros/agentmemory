package service

import (
	"context"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FileHistoryEntry represents a past observation about a specific file.
type FileHistoryEntry struct {
	File          string    `json:"file"`
	ObservationID string    `json:"observation_id"`
	Title         string    `json:"title"`
	Narrative     string    `json:"narrative"`
	Timestamp     time.Time `json:"timestamp"`
	SessionID     string    `json:"session_id"`
}

// FileHistoryService looks up past observations about specific files.
type FileHistoryService struct {
	queries *store.Queries
}

// NewFileHistoryService creates a new FileHistoryService backed by the given connection pool.
func NewFileHistoryService(pool *pgxpool.Pool) *FileHistoryService {
	return &FileHistoryService{
		queries: store.New(pool),
	}
}

// GetFileHistory returns past observations about the given files, optionally
// excluding entries from the specified session. For MVP, this returns an empty
// result set — the actual implementation requires DB queries that will be added
// in a future iteration. Always returns a valid (potentially empty) slice and
// never returns an error for missing files.
func (s *FileHistoryService) GetFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error) {
	// MVP: Return empty results. The actual implementation will query
	// observations joined against file metadata once the observation
	// store supports per-file tracking.
	_ = files
	_ = excludeSessionID
	return make([]FileHistoryEntry, 0), nil
}
