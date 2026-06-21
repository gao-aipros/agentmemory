package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// handler context keys are re-exported from internal/auth to keep auth knowledge
// in one canonical package shared by handler and mcp layers.
// The middleware uses auth.UserIDKey etc. directly from the auth package.

// AuthMiddleware returns an HTTP middleware that authenticates requests.
// It supports both JWT session tokens (st_ prefix) and API keys (ak_ prefix).
// On success, it injects user identity into the request context.
// On failure, it returns 401 Unauthorized.
func AuthMiddleware(pool *pgxpool.Pool) func(next http.Handler) http.Handler {
	queries := store.New(pool)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if token == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized",
					"message": "authentication required",
				})
				return
			}

			// Strip "Bearer " prefix if present
			token = strings.TrimPrefix(token, "Bearer ")

			var user *store.User
			var err error

			switch {
			case strings.HasPrefix(token, auth.TokenPrefix):
				// JWT session token
				user, err = validateSessionToken(r.Context(), token, config.GetJWTSecret(), queries)
			case strings.HasPrefix(token, auth.APIKeyPrefix):
				// API key — look up by prefix
				user, err = validateAPIKey(r.Context(), token, queries)
			default:
				err = fmt.Errorf("unknown token format")
			}

			if err != nil {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized",
					"message": err.Error(),
				})
				return
			}

			// Inject user identity into context
			ctx := context.WithValue(r.Context(), auth.UserIDKey, user.ID)
			ctx = context.WithValue(ctx, auth.UserEmailKey, user.Email)
			ctx = context.WithValue(ctx, auth.UserNameKey, user.Name)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireSessionToken is a middleware that rejects API key tokens (ak_ prefix).
// It is used for UI routes (/, /v1/socket) where only session tokens are allowed.
func RequireSessionToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")

		// If the token looks like an API key, reject it
		if strings.HasPrefix(token, auth.APIKeyPrefix) {
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "forbidden",
				"message": "API keys are not allowed for this endpoint; use a session token",
			})
			return
		}

		// Otherwise, pass through — AuthMiddleware will handle validation
		next.ServeHTTP(w, r)
	})
}

// validateSessionToken validates a JWT session token and returns the corresponding user.
func validateSessionToken(ctx context.Context, token string, secret string, queries *store.Queries) (*store.User, error) {
	claims, err := auth.ValidateToken(token, secret)
	if err != nil {
		return nil, fmt.Errorf("invalid session token: %w", err)
	}

	if claims.UserID == "" {
		return nil, fmt.Errorf("invalid token: missing user ID")
	}

	user, err := queries.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &user, nil
}

// validateAPIKey validates an API key token (by prefix lookup) and returns the corresponding user.
func validateAPIKey(ctx context.Context, token string, queries *store.Queries) (*store.User, error) {
	// Strip the "ak_" prefix before hashing — stored hash was computed from bare hex key
	bareKey := strings.TrimPrefix(token, auth.APIKeyPrefix)
	hash, err := auth.HashKey(bareKey)
	if err != nil {
		return nil, fmt.Errorf("failed to hash API key: %w", err)
	}

	apiKey, err := queries.GetAPIKeyByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	// Check expiration if set
	if apiKey.ExpiresAt.Valid && apiKey.ExpiresAt.Time.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Update last_used_at (best-effort, use background context to avoid cancel)
	_ = queries.UpdateAPIKeyLastUsed(context.WithoutCancel(ctx), apiKey.ID)

	// Fetch the user
	user, err := queries.GetUserByID(ctx, apiKey.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found for API key")
	}

	return &user, nil
}

// GetUserIDFromContext extracts the authenticated user ID from the request context.
func GetUserIDFromContext(ctx context.Context) string {
	return auth.GetUserIDFromContext(ctx)
}

// GetUserEmailFromContext extracts the authenticated user email from the request context.
func GetUserEmailFromContext(ctx context.Context) string {
	email, _ := ctx.Value(auth.UserEmailKey).(string)
	return email
}

// GetUserNameFromContext extracts the authenticated user name from the request context.
func GetUserNameFromContext(ctx context.Context) string {
	name, _ := ctx.Value(auth.UserNameKey).(string)
	return name
}
