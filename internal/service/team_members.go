package service

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TeamMembersService manages team membership — adding, removing, and listing members.
type TeamMembersService struct {
	queries *store.Queries
}

// NewTeamMembersService creates a new TeamMembersService backed by the given connection pool.
func NewTeamMembersService(pool *pgxpool.Pool) *TeamMembersService {
	return &TeamMembersService{
		queries: store.New(pool),
	}
}

// AddMember adds a user to a team. A user can only be in one team at a time (one-to-many).
func (s *TeamMembersService) AddMember(ctx context.Context, teamID, userID string) error {
	// Check that the team exists
	_, err := s.queries.GetTeam(ctx, teamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// Check that the user exists
	_, err = s.queries.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	// Check if user is already in ANY team (one-to-many constraint)
	existingTeam, err := s.queries.GetUserTeam(ctx, userID)
	if err == nil && existingTeam.ID != "" {
		return fmt.Errorf("user is already a member of team %q (users can only be in one team)", existingTeam.Name)
	}

	// Add the member
	params := store.AddTeamMemberParams{
		ID:     uuid.New().String(),
		TeamID: teamID,
		UserID: userID,
	}

	_, err = s.queries.AddTeamMember(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}

// RemoveMember removes a user from a team. Uses exit/re-join pattern: DELETE row, no soft-delete.
func (s *TeamMembersService) RemoveMember(ctx context.Context, teamID, userID string) error {
	// Verify the membership exists
	members, err := s.queries.ListTeamMembers(ctx, teamID)
	if err != nil {
		return fmt.Errorf("failed to list team members: %w", err)
	}

	found := false
	for _, m := range members {
		if m.UserID == userID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("user is not a member of this team")
	}

	if err := s.queries.RemoveTeamMember(ctx, store.RemoveTeamMemberParams{TeamID: teamID, UserID: userID}); err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	return nil
}

// ListMembers lists all members of a team.
func (s *TeamMembersService) ListMembers(ctx context.Context, teamID string) ([]store.TeamMember, error) {
	members, err := s.queries.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("failed to list team members: %w", err)
	}
	return members, nil
}

// GetUserTeam returns the user's current team, or nil if the user is not in any team.
func (s *TeamMembersService) GetUserTeam(ctx context.Context, userID string) (*store.Team, error) {
	team, err := s.queries.GetUserTeam(ctx, userID)
	if err != nil {
		// If no team found, return nil
		return nil, nil
	}
	return &team, nil
}
