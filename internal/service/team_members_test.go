package service

import (
	"context"
	"strings"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
)

// mockTeamMembersQuerier implements teamMembersQuerier for testing ownership checks.
type mockTeamMembersQuerier struct {
	getTeam       func(ctx context.Context, id string) (store.Team, error)
	getUserByID   func(ctx context.Context, id string) (store.User, error)
	getUserTeam   func(ctx context.Context, userID string) (store.Team, error)
	addTeamMember func(ctx context.Context, params store.AddTeamMemberParams) (store.TeamMember, error)
	listMembers   func(ctx context.Context, teamID string) ([]store.TeamMember, error)
	removeMember  func(ctx context.Context, params store.RemoveTeamMemberParams) error
}

func (m *mockTeamMembersQuerier) GetTeam(ctx context.Context, id string) (store.Team, error) {
	return m.getTeam(ctx, id)
}

func (m *mockTeamMembersQuerier) GetUserByID(ctx context.Context, id string) (store.User, error) {
	return m.getUserByID(ctx, id)
}

func (m *mockTeamMembersQuerier) GetUserTeam(ctx context.Context, userID string) (store.Team, error) {
	return m.getUserTeam(ctx, userID)
}

func (m *mockTeamMembersQuerier) AddTeamMember(ctx context.Context, params store.AddTeamMemberParams) (store.TeamMember, error) {
	return m.addTeamMember(ctx, params)
}

func (m *mockTeamMembersQuerier) ListTeamMembers(ctx context.Context, teamID string) ([]store.TeamMember, error) {
	return m.listMembers(ctx, teamID)
}

func (m *mockTeamMembersQuerier) RemoveTeamMember(ctx context.Context, params store.RemoveTeamMemberParams) error {
	return m.removeMember(ctx, params)
}

// TestAddMemberRequiresOwnership verifies that only the team owner can add members.
func TestAddMemberRequiresOwnership(t *testing.T) {
	ctx := context.Background()

	t.Run("owner can add member", func(t *testing.T) {
		mock := &mockTeamMembersQuerier{
			getTeam: func(ctx context.Context, id string) (store.Team, error) {
				return store.Team{ID: "team-1", OwnerID: "owner-1"}, nil
			},
			getUserByID: func(ctx context.Context, id string) (store.User, error) {
				return store.User{ID: id}, nil
			},
			getUserTeam: func(ctx context.Context, userID string) (store.Team, error) {
				return store.Team{}, nil // return empty team (user not in any team)
			},
			addTeamMember: func(ctx context.Context, params store.AddTeamMemberParams) (store.TeamMember, error) {
				return store.TeamMember{ID: params.ID, TeamID: params.TeamID, UserID: params.UserID}, nil
			},
		}

		svc := newTeamMembersServiceWithQuerier(mock)
		err := svc.AddMember(ctx, "team-1", "user-2", "owner-1")
		if err != nil {
			t.Fatalf("owner should be able to add member, got error: %v", err)
		}
	})

	t.Run("non-owner cannot add member", func(t *testing.T) {
		mock := &mockTeamMembersQuerier{
			getTeam: func(ctx context.Context, id string) (store.Team, error) {
				return store.Team{ID: "team-1", OwnerID: "owner-1"}, nil
			},
		}

		svc := newTeamMembersServiceWithQuerier(mock)
		err := svc.AddMember(ctx, "team-1", "user-2", "non-owner-caller")
		if err == nil {
			t.Fatal("non-owner should not be able to add members")
		}
		if !strings.Contains(err.Error(), "only the team owner") {
			t.Fatalf("expected ownership error, got: %v", err)
		}
	})
}

// TestRemoveMemberRequiresOwnership verifies that only the team owner can remove members.
func TestRemoveMemberRequiresOwnership(t *testing.T) {
	ctx := context.Background()

	t.Run("owner can remove member", func(t *testing.T) {
		mock := &mockTeamMembersQuerier{
			getTeam: func(ctx context.Context, id string) (store.Team, error) {
				return store.Team{ID: "team-1", OwnerID: "owner-1"}, nil
			},
			listMembers: func(ctx context.Context, teamID string) ([]store.TeamMember, error) {
				return []store.TeamMember{
					{UserID: "user-2"},
				}, nil
			},
			removeMember: func(ctx context.Context, params store.RemoveTeamMemberParams) error {
				return nil
			},
		}

		svc := newTeamMembersServiceWithQuerier(mock)
		err := svc.RemoveMember(ctx, "team-1", "user-2", "owner-1")
		if err != nil {
			t.Fatalf("owner should be able to remove member, got error: %v", err)
		}
	})

	t.Run("non-owner cannot remove member", func(t *testing.T) {
		mock := &mockTeamMembersQuerier{
			getTeam: func(ctx context.Context, id string) (store.Team, error) {
				return store.Team{ID: "team-1", OwnerID: "owner-1"}, nil
			},
		}

		svc := newTeamMembersServiceWithQuerier(mock)
		err := svc.RemoveMember(ctx, "team-1", "user-2", "non-owner-caller")
		if err == nil {
			t.Fatal("non-owner should not be able to remove members")
		}
		if !strings.Contains(err.Error(), "only the team owner") {
			t.Fatalf("expected ownership error, got: %v", err)
		}
	})
}
