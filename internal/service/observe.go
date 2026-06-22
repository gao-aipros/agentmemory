package service

import (
	"context"
	"fmt"
	"time"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ObservationService records agent observations and triggers downstream compression.
type ObservationService struct {
	queries    *store.Queries
	compressor *CompressionService
}

// RecordObservationInput is the input for recording a new observation.
type RecordObservationInput struct {
	SessionID   string
	OwnerType   string
	OwnerUserID string
	OwnerTeamID string
	Type        string
	Title       string
	Narrative   string
	Facts       string
	Concepts    []string
	Files       []string
	Importance  *float64
}

// NewObservationService creates a new ObservationService.
func NewObservationService(pool *pgxpool.Pool, compressor *CompressionService) *ObservationService {
	return &ObservationService{
		queries:    store.New(pool),
		compressor: compressor,
	}
}

// RecordObservation validates and records a new observation.
// Returns the created observation and any validation error.
func (s *ObservationService) RecordObservation(ctx context.Context, input RecordObservationInput) (*store.Observation, error) {
	// Validate hook type
	if !ValidateHookType(input.Type) {
		return nil, fmt.Errorf("invalid hook type: %q", input.Type)
	}

	// Validate importance (nil means use default 0.5)
	imp := 0.5
	if input.Importance != nil {
		imp = *input.Importance
	}
	if !ValidateImportance(imp) {
		return nil, fmt.Errorf("importance must be between 0.0 and 1.0, got %f", imp)
	}

	// Validate required fields
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if input.Narrative == "" {
		return nil, fmt.Errorf("narrative is required")
	}
	if input.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Visibility is always 'private' per the CHECK constraint
	visibility := "private"

	// Set defaults
	ownerType := input.OwnerType
	if ownerType == "" {
		ownerType = "user"
	}

	id := uuid.New().String()

	params := store.InsertObservationParams{
		ID:          id,
		SessionID:   input.SessionID,
		OwnerType:   ownerType,
		OwnerUserID: nilString(input.OwnerUserID),
		OwnerTeamID: nilString(input.OwnerTeamID),
		Visibility:  visibility,
		Type:        input.Type,
		Title:       input.Title,
		Narrative:   input.Narrative,
		Facts:       nilString(input.Facts),
		Concepts:    input.Concepts,
		Files:       input.Files,
		Importance:  imp,
		Timestamp:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}

	obs, err := s.queries.InsertObservation(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to insert observation: %w", err)
	}

	// Trigger async compression
	if s.compressor != nil {
		s.compressor.TriggerAsync(ctx, &obs)
	}

	return &obs, nil
}
