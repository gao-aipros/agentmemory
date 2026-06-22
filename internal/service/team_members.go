package service

import (
	"context"
	"fmt"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// teamMembersQuerier is the subset of *store.Queries methods used by TeamMembersService.
// The concrete *store.Queries satisfies this interface, enabling mock-based unit testing.
type teamMembersQuerier interface {
	GetTeam(ctx context.Context, id string) (store.Team, error)
	GetUserByID(ctx context.Context, id string) (store.User, error)
	GetUserTeam(ctx context.Context, userID string) (store.Team, error)
	AddTeamMember(ctx context.Context, params store.AddTeamMemberParams) (store.TeamMember, error)
	ListTeamMembers(ctx context.Context, teamID string) ([]store.TeamMember, error)
	RemoveTeamMember(ctx context.Context, params store.RemoveTeamMemberParams) error
}

// TeamMembersService manages team membership — adding, removing, and listing members.
type TeamMembersService struct {
	queries teamMembersQuerier
}

// NewTeamMembersService creates a new TeamMembersService backed by the given connection pool.
func NewTeamMembersService(pool *pgxpool.Pool) *TeamMembersService {
	return &TeamMembersService{
		queries: store.New(pool),
	}
}

// newTeamMembersServiceWithQuerier creates a TeamMembersService with a custom querier (for testing).
func newTeamMembersServiceWithQuerier(q teamMembersQuerier) *TeamMembersService {
	return &TeamMembersService{queries: q}
}

// AddMember adds a user to a team. Only the team owner may add members.
// A user can only be in one team at a time (one-to-many).
func (s *TeamMembersService) AddMember(ctx context.Context, teamID, userID, callerID string) error {
	// Check that the team exists
	team, err := s.queries.GetTeam(ctx, teamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	// Verify caller is the team owner
	if team.OwnerID != callerID {
		return fmt.Errorf("only the team owner can add members")
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

// RemoveMember removes a user from a team. Only the team owner may remove members.
// Uses exit/re-join pattern: DELETE row, no soft-delete.
func (s *TeamMembersService) RemoveMember(ctx context.Context, teamID, userID, callerID string) error {
	// Check that the team exists and caller is the owner
	team, err := s.queries.GetTeam(ctx, teamID)
	if err != nil {
		return fmt.Errorf("team not found: %w", err)
	}

	if team.OwnerID != callerID {
		return fmt.Errorf("only the team owner can remove members")
	}

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

// ListMembers lists all members of a team. The caller must be the team owner or a team member.
func (s *TeamMembersService) ListMembers(ctx context.Context, teamID, callerID string) ([]store.TeamMember, error) {
	// Check that the team exists
	team, err := s.queries.GetTeam(ctx, teamID)
	if err != nil {
		return nil, fmt.Errorf("team not found: %w", err)
	}

	// Verify caller is owner or member
	if team.OwnerID != callerID {
		userTeam, err := s.queries.GetUserTeam(ctx, callerID)
		if err != nil || userTeam.ID != teamID {
			return nil, fmt.Errorf("only team owner or members can list team members")
		}
	}

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
