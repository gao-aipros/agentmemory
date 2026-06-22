package service

import (
	"context"
	"fmt"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserService manages user lifecycle — creation, retrieval, update, and deletion.
type UserService struct {
	queries *store.Queries
}

// NewUserService creates a new UserService backed by the given connection pool.
func NewUserService(pool *pgxpool.Pool) *UserService {
	return &UserService{
		queries: store.New(pool),
	}
}

// CreateUser creates a new user with the given email, password, and name.
// The password is bcrypt-hashed before storage. TOTP is disabled by default.
func (s *UserService) CreateUser(ctx context.Context, email, password, name string) (*store.User, error) {
	// Validate required fields
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Check if email is already taken
	existing, err := s.queries.GetUserByEmail(ctx, email)
	if err == nil && existing.ID != "" {
		return nil, fmt.Errorf("email already in use")
	}

	// Hash the password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	params := store.CreateUserParams{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		TotpSecret:   nil,
		TotpEnabled:  false,
	}

	user, err := s.queries.CreateUser(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &user, nil
}

// GetUser retrieves a user by ID.
func (s *UserService) GetUser(ctx context.Context, id string) (*store.User, error) {
	user, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by email address.
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*store.User, error) {
	user, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}
	return &user, nil
}

// UpdateUser updates a user's profile fields.
func (s *UserService) UpdateUser(ctx context.Context, id, email, name string) error {
	// Fetch the existing user to preserve password hash and TOTP settings
	existing, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	params := store.UpdateUserParams{
		ID:           id,
		Email:        email,
		PasswordHash: existing.PasswordHash,
		Name:         name,
		TotpSecret:   existing.TotpSecret,
		TotpEnabled:  existing.TotpEnabled,
	}

	_, err = s.queries.UpdateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// UpdatePassword updates a user's password (bcrypt-hashed).
func (s *UserService) UpdatePassword(ctx context.Context, id, newPassword string) error {
	existing, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	params := store.UpdateUserParams{
		ID:           id,
		Email:        existing.Email,
		PasswordHash: passwordHash,
		Name:         existing.Name,
		TotpSecret:   existing.TotpSecret,
		TotpEnabled:  existing.TotpEnabled,
	}

	_, err = s.queries.UpdateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// EnableTOTP enables TOTP for a user and stores the secret.
func (s *UserService) EnableTOTP(ctx context.Context, id, secret string) error {
	existing, err := s.queries.GetUserByID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to find user: %w", err)
	}

	params := store.UpdateUserParams{
		ID:           id,
		Email:        existing.Email,
		PasswordHash: existing.PasswordHash,
		Name:         existing.Name,
		TotpSecret:   &secret,
		TotpEnabled:  true,
	}

	_, err = s.queries.UpdateUser(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to enable TOTP: %w", err)
	}

	return nil
}

// DeleteUser deletes a user by ID. Related data is handled by ON DELETE CASCADE.
func (s *UserService) DeleteUser(ctx context.Context, id string) error {
	if err := s.queries.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

// ListAPIKeys lists all API keys for a user.
func (s *UserService) ListAPIKeys(ctx context.Context, userID string) ([]store.ApiKey, error) {
	keys, err := s.queries.ListAPIKeysByUser(ctx, store.ListAPIKeysByUserParams{
		UserID: userID,
		Limit:  100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

// CreateAPIKey creates a new API key for a user.
// Returns the stored API key record and the plaintext full key (shown only once).
func (s *UserService) CreateAPIKey(ctx context.Context, userID, label, expiresAt string) (*store.ApiKey, string, error) {
	if label == "" {
		return nil, "", fmt.Errorf("label is required")
	}

	prefix, fullKey, hash, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate API key: %w", err)
	}

	params := store.CreateAPIKeyParams{
		ID:      prefix + "_" + uuid.New().String(),
		UserID:  userID,
		Label:   label,
		KeyHash: hash,
	}

	if expiresAt != "" {
		t, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return nil, "", fmt.Errorf("invalid expires_at format, expected RFC3339: %w", err)
		}
		// Don't allow expiration in the past
		if t.Before(time.Now()) {
			return nil, "", fmt.Errorf("expires_at must be in the future")
		}
		params.ExpiresAt = pgtype.Timestamptz{Time: t, Valid: true}
	}

	apiKey, err := s.queries.CreateAPIKey(ctx, params)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create API key: %w", err)
	}

	return &apiKey, fullKey, nil
}

// DeleteAPIKey deletes an API key, ensuring it belongs to the specified user.
func (s *UserService) DeleteAPIKey(ctx context.Context, userID, keyID string) error {
	// Verify the key belongs to this user
	key, err := s.queries.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		return fmt.Errorf("API key not found: %w", err)
	}
	if key.UserID != userID {
		return fmt.Errorf("API key does not belong to user")
	}

	if err := s.queries.DeleteAPIKey(ctx, keyID); err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	return nil
}
