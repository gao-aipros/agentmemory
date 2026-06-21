package service

// TeamOperationalMode represents the three operational modes for teams
// as defined in FR-018.
type TeamOperationalMode string

const (
	// OwnedByUser: per-user consolidation, auto-shares ALL consolidated Memory to team (no opt-out).
	ModeOwnedByUser TeamOperationalMode = "owned_by_user"

	// OwnedByTeam: single team-level consolidation, all members feed one pool (no opt-out).
	ModeOwnedByTeam TeamOperationalMode = "owned_by_team"

	// MemberChoice: per-user consolidation, AGENTMEMORY_SHARE_CONSOLIDATED checked on each
	// observe() call. When true, the memory is shared with the team.
	ModeMemberChoice TeamOperationalMode = "member_choice"
)

// ResolveSharing determines whether a memory should be shared to the team
// based on the team's operational mode and the per-call shareConsolidated flag.
//
// Mode semantics (FR-018 clarified):
//   - OwnedByUser: always returns true (auto-shares ALL consolidated Memory to team, no opt-out)
//   - OwnedByTeam: always returns true (all members feed one pool, no opt-out)
//   - MemberChoice: returns shareConsolidated (checked from AGENTMEMORY_SHARE_CONSOLIDATED env var per observe call)
func ResolveSharing(mode TeamOperationalMode, shareConsolidated bool) bool {
	switch mode {
	case ModeOwnedByUser:
		return true // Auto-share all, no opt-out
	case ModeOwnedByTeam:
		return true // All feed one pool, no opt-out
	case ModeMemberChoice:
		return shareConsolidated // Per-call decision
	default:
		// Unknown mode defaults to member_choice behavior
		return shareConsolidated
	}
}
