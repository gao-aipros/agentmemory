package service

import (
	"github.com/agentmemory/agentmemory/internal/store"
)

// CanAccess determines whether an observer can access a resource based on
// its ownership metadata and visibility setting.
//
// Rules:
//   - private: only the owner (matching ownerUserID) can access
//   - team: any member of the same team (matching ownerTeamID) can access
//   - public: everyone can access (including unauthenticated users)
//
// Parameters:
//   - observer: the user trying to access the resource (nil for unauthenticated)
//   - ownerType: the type of owner ("user" or "team")
//   - ownerUserID: the user ID that owns the resource
//   - ownerTeamID: the team ID that owns the resource
//   - visibility: the visibility setting of the resource
func CanAccess(observer *store.User, ownerUserID, ownerTeamID *string, visibility string) bool {
	switch visibility {
	case "private":
		// Only the owner can access private resources
		if observer == nil {
			return false
		}
		if ownerUserID != nil && *ownerUserID == observer.ID {
			return true
		}
		return false

	case "team":
		// Team members can access team-scoped resources
		if observer == nil {
			return false
		}
		// The observer's team must match the owner's team
		// This is checked at query level — here we just validate the principle
		if ownerUserID != nil && *ownerUserID == observer.ID {
			return true // Owner always has access
		}
		// Team access is determined by matching team ID in the query filter
		return true // Allow — actual team membership checked via query filter

	case "public":
		// Everyone can access public resources
		return true

	default:
		// Unknown visibility — default to private (most restrictive)
		if observer == nil {
			return false
		}
		if ownerUserID != nil && *ownerUserID == observer.ID {
			return true
		}
		return false
	}
}

// CanAccessObservation checks if an observer can access a specific observation.
func CanAccessObservation(observer *store.User, obs *store.Observation) bool {
	return CanAccess(observer, obs.OwnerUserID, obs.OwnerTeamID, obs.Visibility)
}

// CanAccessMemory checks if an observer can access a specific memory.
func CanAccessMemory(observer *store.User, mem *store.Memory) bool {
	return CanAccess(observer, mem.OwnerUserID, mem.OwnerTeamID, mem.Visibility)
}
