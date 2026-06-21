package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProceduralService stores and recalls trigger-to-procedure mappings.
// MVP uses an in-memory map; future versions will persist to the database.
type ProceduralService struct {
	pool *pgxpool.Pool
	mu   sync.RWMutex
	data map[string][]string // trigger -> list of procedure texts
}

// NewProceduralService creates a new ProceduralService.
func NewProceduralService(pool *pgxpool.Pool) *ProceduralService {
	return &ProceduralService{
		pool: pool,
		data: make(map[string][]string),
	}
}

// StoreProcedure records a procedure text associated with a trigger phrase.
// Returns a unique ID for the stored procedure.
func (s *ProceduralService) StoreProcedure(ctx context.Context, trigger, procedure string) (string, error) {
	if trigger == "" {
		return "", fmt.Errorf("trigger is required")
	}
	if procedure == "" {
		return "", fmt.Errorf("procedure is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()

	// Prefix the procedure with its ID so RecallProcedure can return structured data.
	entry := fmt.Sprintf("[%s] %s", id, procedure)
	s.data[trigger] = append(s.data[trigger], entry)

	return id, nil
}

// RecallProcedure returns all procedure texts associated with the given
// trigger phrase. Returns an empty slice if no procedures match.
func (s *ProceduralService) RecallProcedure(ctx context.Context, trigger string) ([]string, error) {
	if trigger == "" {
		return nil, fmt.Errorf("trigger is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	procedures, ok := s.data[trigger]
	if !ok {
		return []string{}, nil
	}

	// Return a copy to avoid data races on the slice header.
	result := make([]string, len(procedures))
	copy(result, procedures)

	return result, nil
}
