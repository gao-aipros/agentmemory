// Package auth provides shared context key definitions for user identity
// injection across the handler and MCP tool layers.
package auth

import "context"

// ContextKey is a private type for context keys to avoid collisions
// between packages that inject values into context.Context.
type ContextKey string

const (
	// UserIDKey is the context key for authenticated user ID.
	// Injected by AuthMiddleware and consumed by MCP tool handlers.
	UserIDKey ContextKey = "user_id"

	// UserEmailKey is the context key for authenticated user email.
	UserEmailKey ContextKey = "user_email"

	// UserNameKey is the context key for authenticated user name.
	UserNameKey ContextKey = "user_name"
)

// GetUserIDFromContext extracts the authenticated user ID from the request context.
func GetUserIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(UserIDKey).(string)
	return id
}
