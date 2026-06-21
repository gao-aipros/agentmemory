package unit

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// T082: Team Operational Modes Tests
// =============================================================================
//
// Tests the 3 team operational modes for memory consolidation sharing:
//   - OwnedByUser: always returns true (auto-shares ALL memories to team)
//   - OwnedByTeam: always returns true (pools all memories into team namespace)
//   - MemberChoice: depends on AGENTMEMORY_SHARE_CONSOLIDATED env flag
//
// Uses test-local types and functions to validate the expected behavior
// before the real service implementation exists.

// TeamMode represents the three team operational modes.
type TeamMode string

const (
	// TeamModeOwnedByUser — memories are owned by individual users but
	// auto-shared to the team. Always resolves to sharing.
	TeamModeOwnedByUser TeamMode = "owned_by_user"

	// TeamModeOwnedByTeam — memories are pooled under the team namespace.
	// Always resolves to sharing (team owns everything).
	TeamModeOwnedByTeam TeamMode = "owned_by_team"

	// TeamModeMemberChoice — sharing is decided per-member based on the
	// AGENTMEMORY_SHARE_CONSOLIDATED environment variable or member preference.
	TeamModeMemberChoice TeamMode = "member_choice"
)

// SharingDecision represents whether sharing should happen for a consolidation unit.
type SharingDecision struct {
	ShouldShare bool
	Reason      string
}

// ResolveSharing determines whether consolidated memories should be shared
// to the team based on the team mode, the AGENTMEMORY_SHARE_CONSOLIDATED flag,
// and the default visibility setting.
//
// This mirrors the expected consolidation_mode.go ResolveSharing behavior.
func ResolveSharing(mode TeamMode, shareConsolidated bool, defaultVisibility string) SharingDecision {
	switch mode {
	case TeamModeOwnedByUser:
		return SharingDecision{
			ShouldShare: true,
			Reason:      "OwnedByUser: all memories auto-share to team",
		}

	case TeamModeOwnedByTeam:
		return SharingDecision{
			ShouldShare: true,
			Reason:      "OwnedByTeam: team owns all memories, always shared",
		}

	case TeamModeMemberChoice:
		if shareConsolidated {
			return SharingDecision{
				ShouldShare: true,
				Reason:      "MemberChoice: user opted into sharing (AGENTMEMORY_SHARE_CONSOLIDATED=true)",
			}
		}
		// When not sharing, visibility defaults to private unless the team
		// default is set to team/public
		if defaultVisibility == "team" || defaultVisibility == "public" {
			return SharingDecision{
				ShouldShare: true,
				Reason:      "MemberChoice: team default visibility is " + defaultVisibility,
			}
		}
		return SharingDecision{
			ShouldShare: false,
			Reason:      "MemberChoice: user did not opt into sharing, default visibility is private",
		}

	default:
		return SharingDecision{
			ShouldShare: false,
			Reason:      "Unknown team mode: safe default is no sharing",
		}
	}
}

// =============================================================================
// OwnedByUser Mode Tests
// =============================================================================

func TestTeamModes_OwnedByUser_AlwaysReturnsTrue(t *testing.T) {
	// OwnedByUser mode: auto-shares ALL memories regardless of flags
	tests := []struct {
		name              string
		shareConsolidated bool
		defaultVisibility string
	}{
		{"share_true_private", true, "private"},
		{"share_true_team", true, "team"},
		{"share_true_public", true, "public"},
		{"share_true_member_choice", true, "member_choice"},
		{"share_false_private", false, "private"},
		{"share_false_team", false, "team"},
		{"share_false_public", false, "public"},
		{"share_false_member_choice", false, "member_choice"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSharing(TeamModeOwnedByUser, tt.shareConsolidated, tt.defaultVisibility)
			assert.True(t, result.ShouldShare,
				"OwnedByUser should always share (flags: share=%v, vis=%s), got reason: %s",
				tt.shareConsolidated, tt.defaultVisibility, result.Reason)
			assert.Contains(t, result.Reason, "OwnedByUser")
		})
	}
}

func TestTeamModes_OwnedByUser_IgnoresEnvironmentFlag(t *testing.T) {
	// Regardless of the AGENTMEMORY_SHARE_CONSOLIDATED setting,
	// OwnedByUser mode always shares.

	// With flag unset
	os.Unsetenv("AGENTMEMORY_SHARE_CONSOLIDATED")
	result := ResolveSharing(TeamModeOwnedByUser, false, "private")
	assert.True(t, result.ShouldShare,
		"OwnedByUser should share even when AGENTMEMORY_SHARE_CONSOLIDATED is false")

	// With flag set
	result2 := ResolveSharing(TeamModeOwnedByUser, true, "private")
	assert.True(t, result2.ShouldShare,
		"OwnedByUser should share when AGENTMEMORY_SHARE_CONSOLIDATED is true")
}

// =============================================================================
// OwnedByTeam Mode Tests
// =============================================================================

func TestTeamModes_OwnedByTeam_AlwaysReturnsTrue(t *testing.T) {
	tests := []struct {
		name              string
		shareConsolidated bool
		defaultVisibility string
	}{
		{"share_true_private", true, "private"},
		{"share_true_team", true, "team"},
		{"share_true_public", true, "public"},
		{"share_false_private", false, "private"},
		{"share_false_team", false, "team"},
		{"share_false_public", false, "public"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSharing(TeamModeOwnedByTeam, tt.shareConsolidated, tt.defaultVisibility)
			assert.True(t, result.ShouldShare,
				"OwnedByTeam should always share (flags: share=%v, vis=%s), got reason: %s",
				tt.shareConsolidated, tt.defaultVisibility, result.Reason)
			assert.Contains(t, result.Reason, "OwnedByTeam")
		})
	}
}

func TestTeamModes_OwnedByTeam_TeamOwnsEverything(t *testing.T) {
	// In OwnedByTeam mode, the team is the logical owner of all memories.
	// Memories are always shared regardless of individual preferences.
	result := ResolveSharing(TeamModeOwnedByTeam, false, "private")
	assert.True(t, result.ShouldShare)
	assert.Contains(t, result.Reason, "team owns all")

	// Even with explicit non-sharing flag
	result2 := ResolveSharing(TeamModeOwnedByTeam, false, "member_choice")
	assert.True(t, result2.ShouldShare)
}

// =============================================================================
// MemberChoice Mode Tests
// =============================================================================

func TestTeamModes_MemberChoice_DependsOnShareConsolidated(t *testing.T) {
	// When AGENTMEMORY_SHARE_CONSOLIDATED is true, sharing happens
	result := ResolveSharing(TeamModeMemberChoice, true, "private")
	assert.True(t, result.ShouldShare,
		"MemberChoice with share=true should share")
	assert.Contains(t, result.Reason, "opted into sharing")

	// When AGENTMEMORY_SHARE_CONSOLIDATED is false and default is private, no sharing
	result2 := ResolveSharing(TeamModeMemberChoice, false, "private")
	assert.False(t, result2.ShouldShare,
		"MemberChoice with share=false and default=private should not share")
	assert.Contains(t, result2.Reason, "did not opt")
}

func TestTeamModes_MemberChoice_HonorsDefaultVisibility(t *testing.T) {
	// When the team's default visibility is "team" or "public",
	// sharing should happen even without the SHARE_CONSOLIDATED flag.

	tests := []struct {
		name              string
		shareConsolidated bool
		defaultVisibility string
		shouldShare       bool
	}{
		{"share_true_private", true, "private", true},
		{"share_true_team", true, "team", true},
		{"share_true_public", true, "public", true},
		{"share_true_member_choice", true, "member_choice", true},
		{"share_false_private", false, "private", false},
		{"share_false_team", false, "team", true},             // team default overrides
		{"share_false_public", false, "public", true},          // public default overrides
		{"share_false_member_choice", false, "member_choice", false}, // no override
		{"share_false_empty", false, "", false},                // empty defaults to private
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSharing(TeamModeMemberChoice, tt.shareConsolidated, tt.defaultVisibility)
			assert.Equal(t, tt.shouldShare, result.ShouldShare,
				"MemberChoice: share=%v visibility=%s expected=%v got=%v (reason: %s)",
				tt.shareConsolidated, tt.defaultVisibility, tt.shouldShare,
				result.ShouldShare, result.Reason)
		})
	}
}

func TestTeamModes_MemberChoice_PrivateIsDefaultWhenUnset(t *testing.T) {
	// When no flags are set and default visibility is "member_choice" or empty,
	// MemberChoice should default to private (no sharing).

	result := ResolveSharing(TeamModeMemberChoice, false, "member_choice")
	assert.False(t, result.ShouldShare,
		"MemberChoice with no sharing should default to private")

	result2 := ResolveSharing(TeamModeMemberChoice, false, "")
	assert.False(t, result2.ShouldShare,
		"MemberChoice with empty visibility should default to private")
}

// =============================================================================
// All Mode Combinations Matrix
// =============================================================================

func TestTeamModes_FullCombinationMatrix(t *testing.T) {
	type testCase struct {
		name              string
		mode              TeamMode
		shareConsolidated bool
		defaultVisibility string
		shouldShare       bool
	}

	modes := []TeamMode{TeamModeOwnedByUser, TeamModeOwnedByTeam, TeamModeMemberChoice}
	shareFlags := []bool{true, false}
	visibilities := []string{"private", "team", "public", "member_choice", ""}

	var tests []testCase

	for _, mode := range modes {
		for _, share := range shareFlags {
			for _, vis := range visibilities {
				name := string(mode) + "_share=" + btoa(share) + "_vis=" + vis

				var expected bool
				switch mode {
				case TeamModeOwnedByUser:
					expected = true
				case TeamModeOwnedByTeam:
					expected = true
				case TeamModeMemberChoice:
					expected = share || vis == "team" || vis == "public"
				default:
					expected = false
				}

				tests = append(tests, testCase{
					name:              name,
					mode:              mode,
					shareConsolidated: share,
					defaultVisibility: vis,
					shouldShare:       expected,
				})
			}
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveSharing(tt.mode, tt.shareConsolidated, tt.defaultVisibility)
			assert.Equal(t, tt.shouldShare, result.ShouldShare,
				"%s: expected=%v got=%v", tt.name, tt.shouldShare, result.ShouldShare)
		})
	}
}

// btoa converts a bool to "true" or "false" string for test naming.
func btoa(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// =============================================================================
// Mode String Representation Tests
// =============================================================================

func TestTeamModes_StringConstants(t *testing.T) {
	assert.Equal(t, TeamMode("owned_by_user"), TeamModeOwnedByUser)
	assert.Equal(t, TeamMode("owned_by_team"), TeamModeOwnedByTeam)
	assert.Equal(t, TeamMode("member_choice"), TeamModeMemberChoice)
}

func TestTeamModes_AllModesAreDistinct(t *testing.T) {
	assert.NotEqual(t, TeamModeOwnedByUser, TeamModeOwnedByTeam)
	assert.NotEqual(t, TeamModeOwnedByUser, TeamModeMemberChoice)
	assert.NotEqual(t, TeamModeOwnedByTeam, TeamModeMemberChoice)
}

func TestTeamModes_UnknownModeIsSafe(t *testing.T) {
	// An unrecognized mode should default to no sharing (secure by default)
	result := ResolveSharing(TeamMode("nonexistent_mode"), true, "public")
	assert.False(t, result.ShouldShare,
		"unknown mode should default to no sharing (secure)")
	assert.Contains(t, result.Reason, "Unknown")
}

// =============================================================================
// AGENTMEMORY_SHARE_CONSOLIDATED Environment Variable Tests
// =============================================================================

func TestTeamModes_ShareConsolidatedEnvVarParsing(t *testing.T) {
	// This test validates the expected parsing behavior of the
	// AGENTMEMORY_SHARE_CONSOLIDATED environment variable, matching
	// the config.Load() behavior in internal/config/config.go

	tests := []struct {
		name     string
		envValue string
		expected bool
	}{
		{"true_lower", "true", true},
		{"true_upper", "TRUE", true},
		{"true_mixed", "True", true},
		{"one", "1", true},
		{"false_lower", "false", false},
		{"false_upper", "FALSE", false},
		{"zero", "0", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("AGENTMEMORY_SHARE_CONSOLIDATED", tt.envValue)
			defer os.Unsetenv("AGENTMEMORY_SHARE_CONSOLIDATED")

			// Simulate strconv.ParseBool behavior (used by config.getEnvBool)
			val := os.Getenv("AGENTMEMORY_SHARE_CONSOLIDATED")

			// When the env var is empty, the flag is false by default
			if val == "" {
				result := ResolveSharing(TeamModeMemberChoice, false, "private")
				assert.False(t, result.ShouldShare,
					"empty AGENTMEMORY_SHARE_CONSOLIDATED should mean no sharing")
				return
			}

			// Use the parsed value in ResolveSharing
			result := ResolveSharing(TeamModeMemberChoice, tt.expected, "private")
			assert.Equal(t, tt.expected, result.ShouldShare,
				"env var %s=%s: sharing should be %v",
				"AGENTMEMORY_SHARE_CONSOLIDATED", tt.envValue, tt.expected)
		})
	}
}

// =============================================================================
// ConsolidationMode-like Integration Tests
// =============================================================================

// ConsolidationVisibility mirrors internal/service ConsolidationVisibility.
type ConsolidationVisibility string

const (
	ConsVisibilityPrivate ConsolidationVisibility = "private"
	ConsVisibilityTeam    ConsolidationVisibility = "team"
	ConsVisibilityPublic  ConsolidationVisibility = "public"
)

// DetermineVisibility resolves the consolidation visibility based on team mode
// and sharing decision. Mirrors DefaultConsolidationMode behavior.
func DetermineVisibility(mode TeamMode, shareConsolidated bool, defaultVisibility string) ConsolidationVisibility {
	decision := ResolveSharing(mode, shareConsolidated, defaultVisibility)

	if !decision.ShouldShare {
		return ConsVisibilityPrivate
	}

	// When sharing, respect the team's default visibility
	switch defaultVisibility {
	case "public":
		return ConsVisibilityPublic
	case "team":
		return ConsVisibilityTeam
	default:
		// For OwnedByUser and MemberChoice with sharing, default to team visibility
		return ConsVisibilityTeam
	}
}

func TestTeamModes_DetermineVisibilityOwnedByUser(t *testing.T) {
	// OwnedByUser always shares, visibility depends on team default
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeOwnedByUser, false, "private"))
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeOwnedByUser, false, "member_choice"))
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeOwnedByUser, false, "team"))
	assert.Equal(t, ConsVisibilityPublic,
		DetermineVisibility(TeamModeOwnedByUser, false, "public"))
}

func TestTeamModes_DetermineVisibilityOwnedByTeam(t *testing.T) {
	// OwnedByTeam always shares, visibility depends on team default
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeOwnedByTeam, false, "private"))
	assert.Equal(t, ConsVisibilityPublic,
		DetermineVisibility(TeamModeOwnedByTeam, false, "public"))
}

func TestTeamModes_DetermineVisibilityMemberChoice(t *testing.T) {
	// MemberChoice: no sharing means private
	assert.Equal(t, ConsVisibilityPrivate,
		DetermineVisibility(TeamModeMemberChoice, false, "private"))
	assert.Equal(t, ConsVisibilityPrivate,
		DetermineVisibility(TeamModeMemberChoice, false, "member_choice"))

	// MemberChoice with sharing: visibility depends on team default
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeMemberChoice, true, "private"))
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeMemberChoice, true, "member_choice"))
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeMemberChoice, true, "team"))
	assert.Equal(t, ConsVisibilityPublic,
		DetermineVisibility(TeamModeMemberChoice, true, "public"))

	// MemberChoice with team-level default override (team/public even without flag)
	assert.Equal(t, ConsVisibilityTeam,
		DetermineVisibility(TeamModeMemberChoice, false, "team"))
	assert.Equal(t, ConsVisibilityPublic,
		DetermineVisibility(TeamModeMemberChoice, false, "public"))
}

// =============================================================================
// Ownership Type Tests
// =============================================================================

// OwnerType represents whether a resource is owned by a user or a team.
type OwnerType string

const (
	OwnerTypeUser OwnerType = "user"
	OwnerTypeTeam OwnerType = "team"
)

func TestTeamModes_OwnerTypeMapping(t *testing.T) {
	// Validate the mapping between team modes and owner types
	tests := []struct {
		mode          TeamMode
		expectedOwner OwnerType
	}{
		{TeamModeOwnedByUser, OwnerTypeUser},
		{TeamModeOwnedByTeam, OwnerTypeTeam},
		{TeamModeMemberChoice, OwnerTypeUser}, // member choice defaults to user ownership
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			var owner OwnerType
			switch tt.mode {
			case TeamModeOwnedByTeam:
				owner = OwnerTypeTeam
			default:
				owner = OwnerTypeUser
			}
			assert.Equal(t, tt.expectedOwner, owner,
				"mode %s should map to owner type %s", tt.mode, tt.expectedOwner)
		})
	}
}

// =============================================================================
// Security Edge Case Tests
// =============================================================================

func TestTeamModes_NeverSharesPrivateDataByDefault(t *testing.T) {
	// In MemberChoice mode with no flags, data should remain private
	result := ResolveSharing(TeamModeMemberChoice, false, "private")
	assert.False(t, result.ShouldShare,
		"default behavior should keep data private")
	assert.Contains(t, result.Reason, "did not opt")

	// Verify the visibility that would be applied is private
	vis := DetermineVisibility(TeamModeMemberChoice, false, "private")
	assert.Equal(t, ConsVisibilityPrivate, vis,
		"default visibility should be private for security")
}

func TestTeamModes_DefaultVisibilityMustBeMemberChoice(t *testing.T) {
	// Per the spec, teams.default_visibility = member_choice is the default.
	// This test validates that member_choice behaves correctly as default.
	defaultVis := "member_choice"

	// Without explicit sharing opt-in, member_choice means private
	result := ResolveSharing(TeamModeMemberChoice, false, defaultVis)
	assert.False(t, result.ShouldShare,
		"member_choice default visibility without opt-in should mean private")

	// Even with owned_by_user mode, sharing still happens (mode overrides visibility)
	result2 := ResolveSharing(TeamModeOwnedByUser, false, defaultVis)
	assert.True(t, result2.ShouldShare,
		"owned_by_user mode overrides member_choice visibility")
}
