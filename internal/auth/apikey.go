package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// APIKeyPrefix is prepended to all API key identifiers.
	APIKeyPrefix = "ak_"

	// APIKeyBytes is the number of random bytes for an API key (256-bit).
	APIKeyBytes = 32

	// APIKeyPrefixLength is the number of hex characters from the hash to include in the prefix.
	APIKeyPrefixLength = 8
)

// GenerateAPIKey creates a new API key.
// Returns:
//   - prefix: "ak_" + first 8 hex chars of the hash (used to identify the key)
//   - fullKey: the full plaintext key (given to the user once)
//   - hash: SHA-256 hash of the full key (stored in the database)
//   - error: any error during generation
func GenerateAPIKey() (prefix string, fullKey string, hash string, err error) {
	// Generate 32 random bytes
	keyBytes := make([]byte, APIKeyBytes)
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	fullKey = hex.EncodeToString(keyBytes)

	// Hash the full key for storage
	hash, err = HashKey(fullKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to hash key: %w", err)
	}

	// Prefix is "ak_" + first 8 characters of the hash
	prefix = APIKeyPrefix + hash[:APIKeyPrefixLength]

	return prefix, fullKey, hash, nil
}

// HashKey computes the SHA-256 hash of a key string and returns it as a hex string.
func HashKey(key string) (string, error) {
	h := sha256.New()
	if _, err := h.Write([]byte(key)); err != nil {
		return "", fmt.Errorf("failed to hash key: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ValidateKeyPrefix checks whether a string starts with the "ak_" API key prefix.
func ValidateKeyPrefix(prefix string) bool {
	return strings.HasPrefix(prefix, APIKeyPrefix)
}
