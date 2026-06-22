package service

import (
	"github.com/agentmemory/agentmemory/internal/store"
)

// VisibilityFilter applies row-level visibility rules to query results.
// It filters collections of observations and memories based on the
// authenticated user's identity and team membership.
type VisibilityFilter struct {
	userID     string
	userTeamID string
	queries    *store.Queries
}

// NewVisibilityFilter creates a new VisibilityFilter for the given user.
// If user is nil, the filter is for an unauthenticated user (public-only).
func NewVisibilityFilter(user *store.User, queries *store.Queries) *VisibilityFilter {
	vf := &VisibilityFilter{
		queries: queries,
	}
	if user != nil {
		vf.userID = user.ID
	}
	return vf
}

// SetUserTeam looks up and sets the user's current team ID for team-level access.
func (vf *VisibilityFilter) SetUserTeam(teamID string) {
	vf.userTeamID = teamID
}

// FilterObservations filters a slice of observations, returning only those
// the user is allowed to see based on visibility and ownership rules.
//
// Rules:
//   - Always allow own observations (private)
//   - Allow team-scoped observations if user is in that team
//   - Allow public observations
func (vf *VisibilityFilter) FilterObservations(observations []store.Observation) []store.Observation {
	if len(observations) == 0 {
		return observations
	}

	filtered := make([]store.Observation, 0, len(observations))
	for _, obs := range observations {
		if vf.canAccessObservation(&obs) {
			filtered = append(filtered, obs)
		}
	}
	return filtered
}

// FilterMemories filters a slice of memories, returning only those
// the user is allowed to see based on visibility and ownership rules.
//
// Rules:
//   - Always allow own memories (private)
//   - Allow team-scoped memories if user is in that team
//   - Allow public memories
func (vf *VisibilityFilter) FilterMemories(memories []store.Memory) []store.Memory {
	if len(memories) == 0 {
		return memories
	}

	filtered := make([]store.Memory, 0, len(memories))
	for _, mem := range memories {
		if vf.canAccessMemory(&mem) {
			filtered = append(filtered, mem)
		}
	}
	return filtered
}

// canAccessObservation checks if the current user can access a specific observation.
func (vf *VisibilityFilter) canAccessObservation(obs *store.Observation) bool {
	// Unauthenticated: only public
	if vf.userID == "" {
		return obs.Visibility == "public"
	}

	// Owner always has access
	if obs.OwnerUserID != nil && *obs.OwnerUserID == vf.userID {
		return true
	}

	// Team access: must be same team
	if obs.Visibility == "team" || obs.Visibility == "public" {
		if obs.OwnerTeamID != nil && vf.userTeamID != "" && *obs.OwnerTeamID == vf.userTeamID {
			return true
		}
	}

	// Public access
	if obs.Visibility == "public" {
		return true
	}

	return false
}

// canAccessMemory checks if the current user can access a specific memory.
func (vf *VisibilityFilter) canAccessMemory(mem *store.Memory) bool {
	// Unauthenticated: only public
	if vf.userID == "" {
		return mem.Visibility == "public"
	}

	// Owner always has access
	if mem.OwnerUserID != nil && *mem.OwnerUserID == vf.userID {
		return true
	}

	// Team access
	if mem.Visibility == "team" {
		if mem.OwnerTeamID != nil && vf.userTeamID != "" && *mem.OwnerTeamID == vf.userTeamID {
			return true
		}
	}

	// Public access
	if mem.Visibility == "public" {
		return true
	}

	return false
}

// BuildObservationVisibilityFilterClause returns SQL filter conditions for observations.
// Returns the SQL boolean expression and the parameter values.
// Example: "(owner_user_id = $1 OR visibility = 'public' OR (visibility = 'team' AND owner_team_id = $2))"
func (vf *VisibilityFilter) BuildObservationVisibilityFilterClause() (string, []interface{}) {
	if vf.userID == "" {
		// Unauthenticated: only public
		return "visibility = 'public'", nil
	}

	clause := "(owner_user_id = $1 OR visibility = 'public'"
	args := []interface{}{vf.userID}

	if vf.userTeamID != "" {
		clause += " OR (visibility = 'team' AND owner_team_id = $2)"
		args = append(args, vf.userTeamID)
	}

	clause += ")"
	return clause, args
}

// BuildMemoryVisibilityFilterClause returns SQL filter conditions for memories.
func (vf *VisibilityFilter) BuildMemoryVisibilityFilterClause() (string, []interface{}) {
	if vf.userID == "" {
		return "visibility = 'public'", nil
	}

	clause := "(owner_user_id = $1 OR visibility = 'public'"
	args := []interface{}{vf.userID}

	if vf.userTeamID != "" {
		clause += " OR (visibility = 'team' AND owner_team_id = $2)"
		args = append(args, vf.userTeamID)
	}

	clause += ")"
	return clause, args
}
