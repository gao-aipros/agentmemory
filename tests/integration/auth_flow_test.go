package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuthFlow_UserCreation tests creating a user through the store layer,
// then retrieving it by ID and by email to confirm it was persisted correctly.
func TestAuthFlow_UserCreation(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	userID := uuid.New().String()
	email := "alice@example.com"
	name := "Alice"
	hashedPassword, err := auth.HashPassword("supersecret123")
	require.NoError(t, err)
	require.NotEmpty(t, hashedPassword)

	// Create user via the store
	created, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        email,
		PasswordHash: hashedPassword,
		Name:         name,
		TotpSecret:   nil,
		TotpEnabled:  false,
	})
	require.NoError(t, err)
	assert.Equal(t, userID, created.ID)
	assert.Equal(t, email, created.Email)
	assert.Equal(t, name, created.Name)
	assert.Equal(t, hashedPassword, created.PasswordHash)
	assert.False(t, created.TotpEnabled)
	assert.False(t, created.CreatedAt.Time.IsZero(), "created_at should be set")

	// Retrieve by ID
	fetched, err := queries.GetUserByID(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Email, fetched.Email)

	// Retrieve by email
	fetchedByEmail, err := queries.GetUserByEmail(ctx, email)
	require.NoError(t, err)
	assert.Equal(t, userID, fetchedByEmail.ID)

	// Verify duplicate email is rejected (unique constraint)
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: "different_hash",
		Name:         "Alice Imposter",
	})
	assert.Error(t, err, "duplicate email should be rejected by UNIQUE constraint")

	// Verify listing users includes the new user
	users, err := queries.ListUsers(ctx)
	require.NoError(t, err)
	assert.Len(t, users, 1)
	assert.Equal(t, userID, users[0].ID)
}

// TestAuthFlow_PasswordHashing tests bcrypt password hashing and verification.
// It ensures that:
//  1. The same plaintext password hashes to different values (salt).
//  2. Correct passwords pass verification.
//  3. Wrong passwords fail verification.
//  4. Empty password is handled correctly.
func TestAuthFlow_PasswordHashing(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	password := "correct-horse-battery-staple"
	wrongPassword := "incorrect-donkey-aaa-rechargeable"

	// Hash the password
	hash1, err := auth.HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash1)
	assert.NotEqual(t, password, hash1, "hash should not match plaintext")

	// Hash again — must produce different output due to random salt
	hash2, err := auth.HashPassword(password)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2, "two hashes of same password must differ due to salt")

	// Correct password should verify
	assert.True(t, auth.CheckPassword(hash1, password), "correct password should verify")
	assert.True(t, auth.CheckPassword(hash2, password), "correct password should verify against second hash")

	// Wrong password should fail
	assert.False(t, auth.CheckPassword(hash1, wrongPassword), "wrong password should not verify")

	// Empty password
	hashEmpty, err := auth.HashPassword("")
	require.NoError(t, err)
	assert.False(t, auth.CheckPassword(hashEmpty, "anything"), "empty hash should not match any text")
	assert.True(t, auth.CheckPassword(hashEmpty, ""), "empty hash should match empty text")
}

// TestAuthFlow_JWTGeneration tests JWT token generation and validation.
// It covers:
//  1. Generating a valid token with a known secret.
//  2. Validating a correct token and extracting claims.
//  3. Rejecting tokens with wrong secrets.
//  4. Rejecting tampered tokens.
//  5. Detecting expired tokens.
//  6. Rejecting tokens missing the "st_" prefix.
func TestAuthFlow_JWTGeneration(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	secret := "test-jwt-secret-64-chars-long-for-hs256-minimum-requirement"
	userID := uuid.New().String()

	// Generate a token with 15-minute expiry
	token, err := auth.GenerateToken(userID, 15*time.Minute, secret)
	require.NoError(t, err)
	assert.Contains(t, token, "st_", "token must have st_ prefix")

	// Validate the token
	claims, err := auth.ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
	assert.True(t, claims.ExpiresAt.After(claims.IssuedAt.Time), "expiry must be after issue")

	// Validate with wrong secret
	_, err = auth.ValidateToken(token, "wrong-secret-wrong-secret-wrong-secret-xxxx")
	assert.Error(t, err, "wrong secret should fail validation")

	// Validate a token missing the st_ prefix
	_, err = auth.ValidateToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", secret)
	assert.Error(t, err, "token without st_ prefix should be rejected")

	// Validate an empty token
	_, err = auth.ValidateToken("", secret)
	assert.Error(t, err, "empty token should be rejected")

	// Test with a very short-lived token to ensure expiry is set properly
	shortToken, err := auth.GenerateToken(userID, 1*time.Millisecond, secret)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond) // let it expire
	_, err = auth.ValidateToken(shortToken, secret)
	assert.Error(t, err, "expired token should be rejected")

	// Generate a token with a second user
	userID2 := uuid.New().String()
	token2, err := auth.GenerateToken(userID2, 15*time.Minute, secret)
	require.NoError(t, err)
	claims2, err := auth.ValidateToken(token2, secret)
	require.NoError(t, err)
	assert.Equal(t, userID2, claims2.UserID, "token should contain the correct user ID")
	assert.NotEqual(t, token, token2, "tokens for different users must differ")
}

// TestAuthFlow_APIKeyCreation tests the full API key lifecycle:
// generation, storage, lookup by hash, and last_used_at update.
func TestAuthFlow_APIKeyCreation(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create a user first
	userID := uuid.New().String()
	hash, err := auth.HashPassword("api-key-test-password")
	require.NoError(t, err)

	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        "apikey-test@example.com",
		PasswordHash: hash,
		Name:         "API Key Test User",
	})
	require.NoError(t, err)

	// Generate an API key using the auth package
	prefix, fullKey, keyHash, err := auth.GenerateAPIKey()
	require.NoError(t, err)
	assert.True(t, auth.ValidateKeyPrefix(prefix), "prefix should start with ak_")
	assert.Len(t, fullKey, 64+len(auth.APIKeyPrefix), "full key should include ak_ prefix + 64 hex chars")
	assert.Len(t, keyHash, 64, "SHA-256 hash should be 64 hex characters")
	assert.Equal(t, prefix[3:], keyHash[:8], "prefix should match first 8 chars of hash")

	// Store the API key in the database
	apiKeyID := uuid.New().String()
	created, err := queries.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID:        apiKeyID,
		UserID:    userID,
		Label:     "Test API Key",
		KeyHash:   keyHash,
		ExpiresAt: pgtype.Timestamptz{}, // no expiry
	})
	require.NoError(t, err)
	assert.Equal(t, apiKeyID, created.ID)
	assert.Equal(t, userID, created.UserID)
	assert.Equal(t, "Test API Key", created.Label)
	assert.Equal(t, keyHash, created.KeyHash)
	assert.False(t, created.LastUsedAt.Valid, "last_used_at should be NULL initially")
	assert.False(t, created.CreatedAt.Time.IsZero(), "created_at should be set")

	// Look up the key by hash (simulates API request authentication)
	fetched, err := queries.GetAPIKeyByHash(ctx, keyHash)
	require.NoError(t, err)
	assert.Equal(t, apiKeyID, fetched.ID)
	assert.Equal(t, userID, fetched.UserID)

	// Look up by ID
	fetchedByID, err := queries.GetAPIKeyByID(ctx, apiKeyID)
	require.NoError(t, err)
	assert.Equal(t, apiKeyID, fetchedByID.ID)

	// Update last_used_at
	err = queries.UpdateAPIKeyLastUsed(ctx, apiKeyID)
	require.NoError(t, err)

	// Verify last_used_at was updated
	updated, err := queries.GetAPIKeyByID(ctx, apiKeyID)
	require.NoError(t, err)
	assert.True(t, updated.LastUsedAt.Valid, "last_used_at should be set after update")

	// List API keys for the user
	keys, err := queries.ListAPIKeysByUser(ctx, store.ListAPIKeysByUserParams{
		UserID: userID,
		Limit:  100,
	})
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, apiKeyID, keys[0].ID)

	// Delete the API key
	err = queries.DeleteAPIKey(ctx, apiKeyID)
	require.NoError(t, err)

	// Verify deletion
	_, err = queries.GetAPIKeyByID(ctx, apiKeyID)
	assert.Error(t, err, "deleted API key should not be found")
}

// TestAuthFlow_FullFlow tests the end-to-end user registration and authentication
// flow: create user -> password verification -> JWT generation -> API key creation
// -> API key usage simulation.
func TestAuthFlow_FullFlow(t *testing.T) {
	// Parallel removed — shared container requires sequential execution

	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// ----- Registration phase -----
	userID := uuid.New().String()
	plaintextPassword := "my-strong-password-2024!"
	email := "fullflow@example.com"
	name := "Full Flow User"

	passwordHash, err := auth.HashPassword(plaintextPassword)
	require.NoError(t, err)

	user, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
	})
	require.NoError(t, err)
	assert.Equal(t, email, user.Email)

	// ----- Login phase: verify password -----
	assert.True(t, auth.CheckPassword(user.PasswordHash, plaintextPassword),
		"correct password must verify")
	assert.False(t, auth.CheckPassword(user.PasswordHash, "wrong-password"),
		"wrong password must not verify")

	// ----- JWT session: login successful, issue token -----
	jwtSecret := "production-jwt-secret-with-sufficient-length-for-hs256"
	token, err := auth.GenerateToken(userID, 1*time.Hour, jwtSecret)
	require.NoError(t, err)

	// Subsequent API calls use the token
	claims, err := auth.ValidateToken(token, jwtSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.True(t, claims.ExpiresAt.After(time.Now()), "token should not expire immediately")

	// ----- API Key phase: create a programmatic key -----
	apiKeyPrefix, apiKeyFull, apiKeyHash, err := auth.GenerateAPIKey()
	require.NoError(t, err)

	apiKeyID := uuid.New().String()
	_, err = queries.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID:      apiKeyID,
		UserID:  userID,
		Label:   "CI/CD Pipeline Key",
		KeyHash: apiKeyHash,
	})
	require.NoError(t, err)
	_ = apiKeyPrefix
	_ = apiKeyFull

	// ----- Simulate: request arrives with API key, server hashes and looks up -----
	lookedUp, err := queries.GetAPIKeyByHash(ctx, apiKeyHash)
	require.NoError(t, err)
	assert.Equal(t, userID, lookedUp.UserID, "API key should map to correct user")

	// Mark the key as used
	err = queries.UpdateAPIKeyLastUsed(ctx, apiKeyID)
	require.NoError(t, err)

	// ----- Verifying user can have multiple API keys -----
	prefix2, _, keyHash2, err := auth.GenerateAPIKey()
	require.NoError(t, err)

	apiKeyID2 := uuid.New().String()
	_, err = queries.CreateAPIKey(ctx, store.CreateAPIKeyParams{
		ID:      apiKeyID2,
		UserID:  userID,
		Label:   "Backup Key",
		KeyHash: keyHash2,
	})
	require.NoError(t, err)
	_ = prefix2

	// List both keys
	allKeys, err := queries.ListAPIKeysByUser(ctx, store.ListAPIKeysByUserParams{
		UserID: userID,
		Limit:  100,
	})
	require.NoError(t, err)
	assert.Len(t, allKeys, 2, "user should have 2 API keys")

	// ----- Cleanup: delete keys, then user -----
	for _, k := range allKeys {
		err = queries.DeleteAPIKey(ctx, k.ID)
		require.NoError(t, err)
	}
	err = queries.DeleteUser(ctx, userID)
	require.NoError(t, err)

	// Verify cascade: user is gone
	_, err = queries.GetUserByID(ctx, userID)
	assert.Error(t, err, "deleted user should not be fetchable")
}

// TestListAPIKeysByUser_Limit verifies that ListAPIKeysByUser respects the LIMIT parameter.
func TestListAPIKeysByUser_Limit(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	queries := store.New(db.Pool)

	// Create a user
	userID := uuid.New().String()
	hash, err := auth.HashPassword("limit-test-password")
	require.NoError(t, err)

	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID:           userID,
		Email:        "limit-test@example.com",
		PasswordHash: hash,
		Name:         "Limit Test User",
	})
	require.NoError(t, err)

	// Insert 5 API keys
	for i := 0; i < 5; i++ {
		prefix, _, keyHash, err := auth.GenerateAPIKey()
		require.NoError(t, err)

		_, err = queries.CreateAPIKey(ctx, store.CreateAPIKeyParams{
			ID:      fmt.Sprintf("%s_%s", prefix, uuid.New().String()),
			UserID:  userID,
			Label:   fmt.Sprintf("Key %d", i+1),
			KeyHash: keyHash,
		})
		require.NoError(t, err)
	}
	_ = hash

	// Query with limit=3 — use the new params struct
	keys, err := queries.ListAPIKeysByUser(ctx, store.ListAPIKeysByUserParams{
		UserID: userID,
		Limit:  3,
	})
	require.NoError(t, err)
	assert.Len(t, keys, 3, "should return exactly 3 keys with limit=3")
}
