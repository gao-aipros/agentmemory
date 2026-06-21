package service

// ConsolidationVisibility determines how memories and lessons are scoped
// during consolidation — private to the user, shared with the team, or public.
type ConsolidationVisibility string

const (
	// VisibilityPrivate means the consolidated memory is only visible to the owner.
	VisibilityPrivate ConsolidationVisibility = "private"

	// VisibilityTeam means the consolidated memory is visible to team members.
	VisibilityTeam ConsolidationVisibility = "team"

	// VisibilityPublic means the consolidated memory is publicly visible.
	VisibilityPublic ConsolidationVisibility = "public"
)

// ConsolidationMode holds the visibility and ownership settings for a consolidation run.
type ConsolidationMode struct {
	Visibility ConsolidationVisibility

	// OwnedByUser controls "Owned by User" mode: per-user consolidation where
	// memories are scoped to the individual but auto-shared to their team.
	OwnedByUser bool

	// OwnerUserID is the user who owns the consolidated memories.
	OwnerUserID string

	// OwnerTeamID is the team context for team-scoped consolidation.
	OwnerTeamID string
}

// DefaultConsolidationMode builds a default consolidation mode from team configuration.
// When defaultVisibility is "member_choice", the default is private.
// When ShareConsolidated is true, memories are auto-shared to the team.
func DefaultConsolidationMode(defaultVisibility string, shareConsolidated bool) ConsolidationMode {
	mode := ConsolidationMode{
		OwnedByUser: true,
	}

	switch defaultVisibility {
	case "team":
		mode.Visibility = VisibilityTeam
	case "public":
		mode.Visibility = VisibilityPublic
	default:
		mode.Visibility = VisibilityPrivate
	}

	// When shareConsolidated is true, auto-share memories to the team
	// by promoting visibility from private to team scope.
	if shareConsolidated && mode.Visibility == VisibilityPrivate {
		mode.Visibility = VisibilityTeam
	}

	return mode
}

// VisibilityFromString converts a string to a ConsolidationVisibility.
// Defaults to private for unrecognized values.
func VisibilityFromString(v string) ConsolidationVisibility {
	switch v {
	case "team":
		return VisibilityTeam
	case "public":
		return VisibilityPublic
	default:
		return VisibilityPrivate
	}
}
