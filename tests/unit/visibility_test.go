package unit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// T081: Visibility Rules Tests
// =============================================================================
//
// Tests the visibility access control logic: private, team, and public
// visibility levels. Uses test-local types and functions to validate the
// expected behavior before the real service implementation exists.
//
// These tests serve as executable specifications for the auth visibility
// checks that will be implemented in the auth or service package.

// Visibility represents the visibility level of a resource.
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityTeam    Visibility = "team"
	VisibilityPublic  Visibility = "public"
)

// Observer represents an entity trying to access a resource.
type Observer struct {
	UserID   string  // non-empty if authenticated
	TeamIDs  []string // teams the observer belongs to
}

// ResourceOwner represents who owns a resource.
type ResourceOwner struct {
	OwnerUserID string
	OwnerTeamID string // empty if personal resource
}

// CanAccess checks whether an observer can access a resource with the given
// visibility and ownership. This mirrors the expected service-level visibility
// check that will eventually live in the auth or service package.
func CanAccess(visibility Visibility, owner ResourceOwner, observer *Observer) bool {
	if visibility == VisibilityPublic {
		return true
	}

	// Unauthenticated (nil observer) can only see public
	if observer == nil {
		return false
	}

	// Private: only the owner can see it
	if visibility == VisibilityPrivate {
		return observer.UserID == owner.OwnerUserID
	}

	// Team: owner and team members can see it
	if visibility == VisibilityTeam {
		// Owner always has access
		if observer.UserID == owner.OwnerUserID {
			return true
		}
		// Team members can access if the resource has a team ID
		if owner.OwnerTeamID != "" {
			for _, teamID := range observer.TeamIDs {
				if teamID == owner.OwnerTeamID {
					return true
				}
			}
		}
		return false
	}

	// Unknown visibility: deny by default (secure)
	return false
}

// =============================================================================
// Private Visibility Tests
// =============================================================================

func TestVisibility_PrivateVisibleToOwnerOnly(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-alice",
		OwnerTeamID: "team-alpha",
	}

	// Owner should see private
	assert.True(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID:  "user-alice",
		TeamIDs: []string{"team-alpha"},
	}), "owner should see their own private resource")

	// Different user, same team — should NOT see private
	assert.False(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID:  "user-bob",
		TeamIDs: []string{"team-alpha"},
	}), "team member should not see another user's private resource")

	// Different user, different team — should NOT see private
	assert.False(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID:  "user-charlie",
		TeamIDs: []string{"team-beta"},
	}), "unrelated user should not see private resource")

	// Owner but no team membership — owner still sees private
	assert.True(t, CanAccess(VisibilityPrivate, ResourceOwner{
		OwnerUserID: "user-alice",
		OwnerTeamID: "",
	}, &Observer{
		UserID:  "user-alice",
		TeamIDs: []string{},
	}), "owner without team should still see own private resource")
}

func TestVisibility_PrivateVisibleToOwnerOnly_TableDriven(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-dave",
		OwnerTeamID: "team-omega",
	}

	tests := []struct {
		name     string
		observer *Observer
		expected bool
	}{
		{
			name:     "owner_themselves",
			observer: &Observer{UserID: "user-dave", TeamIDs: []string{"team-omega"}},
			expected: true,
		},
		{
			name:     "owner_no_team_context",
			observer: &Observer{UserID: "user-dave", TeamIDs: []string{}},
			expected: true,
		},
		{
			name:     "owner_different_team",
			observer: &Observer{UserID: "user-dave", TeamIDs: []string{"team-other"}},
			expected: true, // owner is owner regardless of team context
		},
		{
			name:     "teammate_same_team",
			observer: &Observer{UserID: "user-eve", TeamIDs: []string{"team-omega"}},
			expected: false,
		},
		{
			name:     "teammate_multiple_teams",
			observer: &Observer{UserID: "user-frank", TeamIDs: []string{"team-alpha", "team-omega"}},
			expected: false,
		},
		{
			name:     "different_user_different_team",
			observer: &Observer{UserID: "user-grace", TeamIDs: []string{"team-beta"}},
			expected: false,
		},
		{
			name:     "unauthenticated_nil",
			observer: nil,
			expected: false,
		},
		{
			name:     "unauthenticated_empty_user",
			observer: &Observer{UserID: "", TeamIDs: []string{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanAccess(VisibilityPrivate, owner, tt.observer)
			assert.Equal(t, tt.expected, result,
				"private visibility: %s expected=%v got=%v", tt.name, tt.expected, result)
		})
	}
}

// =============================================================================
// Team Visibility Tests
// =============================================================================

func TestVisibility_TeamVisibleToTeamMembers(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-alice",
		OwnerTeamID: "team-alpha",
	}

	// Owner should see team resource
	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-alice",
		TeamIDs: []string{"team-alpha"},
	}), "owner should see team resource")

	// Team member should see team resource
	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-bob",
		TeamIDs: []string{"team-alpha"},
	}), "team member should see team resource")

	// User in multiple teams including the right one
	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-charlie",
		TeamIDs: []string{"team-beta", "team-alpha", "team-gamma"},
	}), "user in multiple teams should see resource if member of correct team")

	// User NOT in the team
	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-dave",
		TeamIDs: []string{"team-beta"},
	}), "user not in team should not see team resource")

	// User with no team memberships
	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-eve",
		TeamIDs: []string{},
	}), "user with no team memberships should not see team resource")

	// Unauthenticated
	assert.False(t, CanAccess(VisibilityTeam, owner, nil),
		"unauthenticated should not see team resource")
}

func TestVisibility_TeamVisibility_TableDriven(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-henry",
		OwnerTeamID: "team-sigma",
	}

	tests := []struct {
		name     string
		observer *Observer
		expected bool
	}{
		{
			name:     "owner_same_team",
			observer: &Observer{UserID: "user-henry", TeamIDs: []string{"team-sigma"}},
			expected: true,
		},
		{
			name:     "owner_empty_teams",
			observer: &Observer{UserID: "user-henry", TeamIDs: []string{}},
			expected: true, // owner always sees
		},
		{
			name:     "owner_different_team",
			observer: &Observer{UserID: "user-henry", TeamIDs: []string{"team-other"}},
			expected: true, // owner always sees
		},
		{
			name:     "teammate_same_team",
			observer: &Observer{UserID: "user-iris", TeamIDs: []string{"team-sigma"}},
			expected: true,
		},
		{
			name:     "teammate_multiple_teams_includes_correct",
			observer: &Observer{UserID: "user-jack", TeamIDs: []string{"team-alpha", "team-sigma"}},
			expected: true,
		},
		{
			name:     "non_teammate",
			observer: &Observer{UserID: "user-kate", TeamIDs: []string{"team-beta"}},
			expected: false,
		},
		{
			name:     "non_teammate_no_teams",
			observer: &Observer{UserID: "user-liam", TeamIDs: []string{}},
			expected: false,
		},
		{
			name:     "unauthenticated_nil",
			observer: nil,
			expected: false,
		},
		{
			name:     "unauthenticated_empty_id",
			observer: &Observer{UserID: "", TeamIDs: []string{}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanAccess(VisibilityTeam, owner, tt.observer)
			assert.Equal(t, tt.expected, result,
				"team visibility: %s expected=%v got=%v", tt.name, tt.expected, result)
		})
	}
}

// =============================================================================
// Public Visibility Tests
// =============================================================================

func TestVisibility_PublicVisibleToAll(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-public-owner",
		OwnerTeamID: "team-public-alpha",
	}

	tests := []struct {
		name     string
		observer *Observer
	}{
		{"owner", &Observer{UserID: "user-public-owner", TeamIDs: []string{"team-public-alpha"}}},
		{"team_member", &Observer{UserID: "user-other", TeamIDs: []string{"team-public-alpha"}}},
		{"different_team", &Observer{UserID: "user-stranger", TeamIDs: []string{"team-other"}}},
		{"no_team", &Observer{UserID: "user-loner", TeamIDs: []string{}}},
		{"unauthenticated_nil", nil},
		{"unauthenticated_empty", &Observer{UserID: "", TeamIDs: []string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, CanAccess(VisibilityPublic, owner, tt.observer),
				"public visibility should be accessible to %s", tt.name)
		})
	}
}

func TestVisibility_PublicIsAlwaysTrue(t *testing.T) {
	// Public visibility should return true for any observer state
	owner := ResourceOwner{OwnerUserID: "anyone", OwnerTeamID: "anyteam"}

	for i := 0; i < 50; i++ {
		assert.True(t, CanAccess(VisibilityPublic, owner, &Observer{
			UserID:  "random-user-" + string(rune('a'+i%26)),
			TeamIDs: []string{},
		}), "public should always be accessible")
	}
}

// =============================================================================
// Nil Observer (Unauthenticated) Tests
// =============================================================================

func TestVisibility_NilObserverCanOnlySeePublic(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-hidden",
		OwnerTeamID: "team-hidden",
	}

	assert.False(t, CanAccess(VisibilityPrivate, owner, nil),
		"nil observer should not see private")
	assert.False(t, CanAccess(VisibilityTeam, owner, nil),
		"nil observer should not see team")
	assert.True(t, CanAccess(VisibilityPublic, owner, nil),
		"nil observer should see public")
}

func TestVisibility_EmptyUserIDObserverCanOnlySeePublic(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-hidden",
		OwnerTeamID: "team-hidden",
	}

	emptyObserver := &Observer{UserID: "", TeamIDs: []string{}}

	assert.False(t, CanAccess(VisibilityPrivate, owner, emptyObserver),
		"empty user observer should not see private")
	assert.False(t, CanAccess(VisibilityTeam, owner, emptyObserver),
		"empty user observer should not see team")
	assert.True(t, CanAccess(VisibilityPublic, owner, emptyObserver),
		"empty user observer should see public")
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestVisibility_UnknownVisibilityDeniesAccess(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-owner",
		OwnerTeamID: "team-alpha",
	}

	observer := &Observer{
		UserID:  "user-owner",
		TeamIDs: []string{"team-alpha"},
	}

	// Unknown/unrecognized visibility should default to deny
	assert.False(t, CanAccess(Visibility("unknown"), owner, observer),
		"unknown visibility should deny access (secure default)")
	assert.False(t, CanAccess(Visibility(""), owner, observer),
		"empty visibility should deny access")
	assert.False(t, CanAccess(Visibility("internal"), owner, observer),
		"unsupported visibility should deny access")
}

func TestVisibility_TeamResourceWithNoOwnerTeamID(t *testing.T) {
	// When a team-visibility resource has no OwnerTeamID set,
	// only the owner can see it (because there's no team to match against)
	owner := ResourceOwner{
		OwnerUserID: "user-orphan",
		OwnerTeamID: "",
	}

	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-orphan",
		TeamIDs: []string{"team-x"},
	}), "owner should see own team-visibility resource even without team ID")

	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-other",
		TeamIDs: []string{"team-x"},
	}), "non-owner should not see team resource with no OwnerTeamID")
}

func TestVisibility_EmptyTeamIDsList(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-owner",
		OwnerTeamID: "team-required",
	}

	// User with empty team list — not in any team
	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-other",
		TeamIDs: []string{},
	}), "user with no teams should not see team resource")

	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-other",
		TeamIDs: nil,
	}), "user with nil teams should not see team resource")
}

func TestVisibility_OwnerUserIDMismatchButTeamMatch(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-alice",
		OwnerTeamID: "team-shared",
	}

	// Team visibility: non-owner in same team can see
	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-bob",
		TeamIDs: []string{"team-shared"},
	}), "non-owner team member should see team resource")

	// Private visibility: only owner, not even team member
	assert.False(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID:  "user-bob",
		TeamIDs: []string{"team-shared"},
	}), "non-owner team member should not see private resource")
}

func TestVisibility_SameUserIDDifferentTeamAccess(t *testing.T) {
	// A resource owner is in team-alpha, observer is same user but
	// currently acting in team-beta context
	owner := ResourceOwner{
		OwnerUserID: "user-flex",
		OwnerTeamID: "team-alpha",
	}

	// Owner should still see private resource regardless of team context
	assert.True(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID:  "user-flex",
		TeamIDs: []string{"team-beta"},
	}), "owner always sees own private resource")

	// Owner should see team resource regardless of team context
	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID:  "user-flex",
		TeamIDs: []string{"team-beta"},
	}), "owner always sees own team resource")
}

func TestVisibility_MultipleTeamsFullMatrix(t *testing.T) {
	// Comprehensive matrix: all combinations of visibility, ownership, and observer
	type testCase struct {
		name       string
		visibility Visibility
		owner      ResourceOwner
		observer   *Observer
		expected   bool
	}

	alice := ResourceOwner{OwnerUserID: "alice", OwnerTeamID: "team-red"}
	bobInRed := &Observer{UserID: "bob", TeamIDs: []string{"team-red"}}
	bobInBlue := &Observer{UserID: "bob", TeamIDs: []string{"team-blue"}}
	charlieNoTeam := &Observer{UserID: "charlie", TeamIDs: []string{}}

	tests := []testCase{
		// Alice sees all her own resources
		{"alice_private_own", VisibilityPrivate, alice,
			&Observer{UserID: "alice", TeamIDs: []string{"team-red"}}, true},
		{"alice_team_own", VisibilityTeam, alice,
			&Observer{UserID: "alice", TeamIDs: []string{"team-red"}}, true},
		{"alice_public_own", VisibilityPublic, alice,
			&Observer{UserID: "alice", TeamIDs: []string{"team-red"}}, true},

		// Bob in team-red sees team, not private
		{"bob_red_private", VisibilityPrivate, alice, bobInRed, false},
		{"bob_red_team", VisibilityTeam, alice, bobInRed, true},
		{"bob_red_public", VisibilityPublic, alice, bobInRed, true},

		// Bob in team-blue sees only public
		{"bob_blue_private", VisibilityPrivate, alice, bobInBlue, false},
		{"bob_blue_team", VisibilityTeam, alice, bobInBlue, false},
		{"bob_blue_public", VisibilityPublic, alice, bobInBlue, true},

		// Charlie (no team) sees only public
		{"charlie_private", VisibilityPrivate, alice, charlieNoTeam, false},
		{"charlie_team", VisibilityTeam, alice, charlieNoTeam, false},
		{"charlie_public", VisibilityPublic, alice, charlieNoTeam, true},

		// Unauthenticated sees only public
		{"nil_private", VisibilityPrivate, alice, nil, false},
		{"nil_team", VisibilityTeam, alice, nil, false},
		{"nil_public", VisibilityPublic, alice, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CanAccess(tt.visibility, tt.owner, tt.observer)
			assert.Equal(t, tt.expected, result,
				"%s: visibility=%s expected=%v got=%v",
				tt.name, tt.visibility, tt.expected, result)
		})
	}
}

// =============================================================================
// Personal (non-team) Resource Tests
// =============================================================================

func TestVisibility_PersonalResourcePrivate(t *testing.T) {
	// Personal resource with no team ID: private visibility
	owner := ResourceOwner{
		OwnerUserID: "user-personal",
		OwnerTeamID: "",
	}

	assert.True(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID: "user-personal", TeamIDs: []string{},
	}), "owner sees own personal private resource")

	assert.False(t, CanAccess(VisibilityPrivate, owner, &Observer{
		UserID: "user-other", TeamIDs: []string{},
	}), "other user cannot see personal private resource")
}

func TestVisibility_PersonalResourceTeam(t *testing.T) {
	// Personal resource with no team ID but set to team visibility
	// Since there's no team to match, only owner can see it
	owner := ResourceOwner{
		OwnerUserID: "user-personal",
		OwnerTeamID: "",
	}

	assert.True(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID: "user-personal", TeamIDs: []string{"any-team"},
	}), "owner sees own resource regardless")

	assert.False(t, CanAccess(VisibilityTeam, owner, &Observer{
		UserID: "user-other", TeamIDs: []string{"any-team"},
	}), "other user cannot see team resource with no OwnerTeamID")
}

func TestVisibility_PersonalResourcePublic(t *testing.T) {
	owner := ResourceOwner{
		OwnerUserID: "user-personal",
		OwnerTeamID: "",
	}

	assert.True(t, CanAccess(VisibilityPublic, owner, &Observer{
		UserID: "user-any", TeamIDs: []string{},
	}), "anyone sees public personal resource")
	assert.True(t, CanAccess(VisibilityPublic, owner, nil),
		"unauthenticated sees public personal resource")
}
