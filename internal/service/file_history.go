package service

import (
	"context"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FileHistoryEntry represents a past observation about a specific file.
type FileHistoryEntry struct {
	File          string             `json:"file"`
	ObservationID string             `json:"observation_id"`
	Title         string             `json:"title"`
	Narrative     string             `json:"narrative"`
	Timestamp     pgtype.Timestamptz `json:"timestamp"`
	SessionID     string             `json:"session_id"`
}

// fileHistoryQuerier handles file history database queries.
type fileHistoryQuerier interface {
	GetFileHistory(ctx context.Context, arg store.GetFileHistoryParams) ([]store.GetFileHistoryRow, error)
}

// FileHistoryService looks up past observations about specific files.
type FileHistoryService struct {
	queries fileHistoryQuerier
}

// NewFileHistoryService creates a new FileHistoryService backed by the given connection pool.
func NewFileHistoryService(pool *pgxpool.Pool) *FileHistoryService {
	return &FileHistoryService{
		queries: store.New(pool),
	}
}

// newFileHistoryServiceWithQuerier creates a FileHistoryService with a custom querier (for testing).
func newFileHistoryServiceWithQuerier(q fileHistoryQuerier) *FileHistoryService {
	return &FileHistoryService{
		queries: q,
	}
}

// GetFileHistory returns past observations about the given files, optionally
// excluding entries from the specified session.
func (s *FileHistoryService) GetFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error) {
	rows, err := s.queries.GetFileHistory(ctx, store.GetFileHistoryParams{Files: files, SessionID: excludeSessionID})
	if err != nil {
		return nil, err
	}

	entries := make([]FileHistoryEntry, len(rows))
	for i, row := range rows {
		entry := FileHistoryEntry{
			ObservationID: row.ID,
			SessionID:     row.SessionID,
			Title:         row.Title,
			Narrative:     row.Narrative,
			Timestamp:     row.Timestamp,
		}
		if len(row.Files) > 0 {
			entry.File = row.Files[0]
		}
		entries[i] = entry
	}
	return entries, nil
}
