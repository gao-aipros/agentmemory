package service

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionService manages agent session lifecycle.
type SessionService struct {
	queries *store.Queries
}

// NewSessionService creates a new SessionService backed by the given connection pool.
func NewSessionService(pool *pgxpool.Pool) *SessionService {
	return &SessionService{
		queries: store.New(pool),
	}
}

// CreateSession starts a new session for the given user, optionally scoped to a team.
func (s *SessionService) CreateSession(ctx context.Context, userID, teamID string) (*store.Session, error) {
	id := uuid.New().String()

	params := store.CreateSessionParams{
		ID:     id,
		UserID: userID,
		TeamID: nilString(teamID),
	}

	session, err := s.queries.CreateSession(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &session, nil
}

// EndSession marks a session as ended and updates its ended_at timestamp.
func (s *SessionService) EndSession(ctx context.Context, sessionID string) (*store.Session, error) {
	session, err := s.queries.EndSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to end session: %w", err)
	}

	return &session, nil
}

// GetActiveSession returns the currently active session for a user, or nil if none exists.
func (s *SessionService) GetActiveSession(ctx context.Context, userID string) (*store.Session, error) {
	session, err := s.queries.GetActiveSession(ctx, userID)
	if err != nil {
		// If no rows, return nil
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	return &session, nil
}

// GetSession retrieves a session by ID.
func (s *SessionService) GetSession(ctx context.Context, sessionID string) (*store.Session, error) {
	session, err := s.queries.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return &session, nil
}

// nilString returns a *string if the input is non-empty, nil otherwise.
func nilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
