package service

import (
	"context"
	"fmt"
	"time"

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

// fileHistoryQuerier handles file history database queries.
type fileHistoryQuerier interface {
	getFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error)
}

// fileHistoryQuerierImpl is the production implementation using raw SQL.
type fileHistoryQuerierImpl struct {
	pool *pgxpool.Pool
}

func (q *fileHistoryQuerierImpl) getFileHistory(ctx context.Context, files []string, excludeSessionID string) ([]FileHistoryEntry, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, session_id, title, narrative, files, timestamp
		FROM observations
		WHERE files IS NOT NULL AND files && $1
		  AND session_id != $2
		ORDER BY timestamp DESC
		LIMIT 100
	`, files, excludeSessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query file history: %w", err)
	}
	defer rows.Close()

	var entries []FileHistoryEntry
	for rows.Next() {
		var e FileHistoryEntry
		var rowFiles []string
		if err := rows.Scan(&e.ObservationID, &e.SessionID, &e.Title, &e.Narrative, &rowFiles, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("failed to scan file history entry: %w", err)
		}
		if len(rowFiles) > 0 {
			e.File = rowFiles[0]
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating file history entries: %w", err)
	}

	return entries, nil
}

// FileHistoryService looks up past observations about specific files.
type FileHistoryService struct {
	queries fileHistoryQuerier
}

// NewFileHistoryService creates a new FileHistoryService backed by the given connection pool.
func NewFileHistoryService(pool *pgxpool.Pool) *FileHistoryService {
	return &FileHistoryService{
		queries: &fileHistoryQuerierImpl{pool: pool},
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
	return s.queries.getFileHistory(ctx, files, excludeSessionID)
}
