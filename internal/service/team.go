package service

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TeamService manages team lifecycle — creation, retrieval, update, and deletion.
type TeamService struct {
	queries *store.Queries
}

// NewTeamService creates a new TeamService backed by the given connection pool.
func NewTeamService(pool *pgxpool.Pool) *TeamService {
	return &TeamService{
		queries: store.New(pool),
	}
}

// CreateTeam creates a new team with the given name, owner, and default visibility.
func (s *TeamService) CreateTeam(ctx context.Context, name, ownerID, defaultVisibility string) (*store.Team, error) {
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}
	if ownerID == "" {
		return nil, fmt.Errorf("owner ID is required")
	}
	if defaultVisibility == "" {
		defaultVisibility = "member_choice"
	}

	params := store.CreateTeamParams{
		ID:                uuid.New().String(),
		Name:              name,
		OwnerID:           ownerID,
		DefaultVisibility: defaultVisibility,
	}

	team, err := s.queries.CreateTeam(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}

	return &team, nil
}

// GetTeam retrieves a team by ID.
func (s *TeamService) GetTeam(ctx context.Context, id string) (*store.Team, error) {
	team, err := s.queries.GetTeam(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	return &team, nil
}

// UpdateTeam updates a team's name and default visibility. Only the owner may update.
func (s *TeamService) UpdateTeam(ctx context.Context, id, ownerID, name, defaultVisibility string) error {
	// Verify ownership
	team, err := s.queries.GetTeam(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find team: %w", err)
	}
	if team.OwnerID != ownerID {
		return fmt.Errorf("only the team owner can update the team")
	}

	params := store.UpdateTeamParams{
		ID:                id,
		Name:              name,
		DefaultVisibility: defaultVisibility,
	}

	_, err = s.queries.UpdateTeam(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update team: %w", err)
	}

	return nil
}

// DeleteTeam deletes a team by ID. Only the owner may delete.
func (s *TeamService) DeleteTeam(ctx context.Context, id, ownerID string) error {
	// Verify ownership
	team, err := s.queries.GetTeam(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find team: %w", err)
	}
	if team.OwnerID != ownerID {
		return fmt.Errorf("only the team owner can delete the team")
	}

	if err := s.queries.DeleteTeam(ctx, id); err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}

	return nil
}

// ListTeamsByOwner lists teams owned by a specific user, up to limit.
func (s *TeamService) ListTeamsByOwner(ctx context.Context, ownerID string, limit int32) ([]store.Team, error) {
	teams, err := s.queries.ListTeamsByOwner(ctx, store.ListTeamsByOwnerParams{
		OwnerID: ownerID,
		Limit:   limit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	return teams, nil
}
