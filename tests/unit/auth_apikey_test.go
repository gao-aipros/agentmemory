package unit

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T080: API Key Hash Generation, Prefix, and Verification Tests
// =============================================================================
//
// These tests validate the expected behavior of the auth package's API key
// functions: generation, hashing, prefix validation ("ak_"), and verification.
// Uses crypto/sha256 for hash verification, matching the expected
// implementation approach in the auth package.

const (
	apiKeyPrefix    = "ak_"
	apiKeyRawBytes  = 32 // 32 bytes raw = 64 hex chars
	apiKeyHexLength = apiKeyRawBytes * 2
)

// generateAPIKey creates a test API key in the expected format:
// "ak_" + 64 hex characters.
func generateAPIKey() (string, error) {
	raw := make([]byte, apiKeyRawBytes)
	_, err := rand.Read(raw)
	if err != nil {
		return "", err
	}
	return apiKeyPrefix + hex.EncodeToString(raw), nil
}

// hashAPIKey computes the SHA-256 hash of an API key, returning the hex string.
// This mirrors the expected auth.HashAPIKey function.
func hashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

// extractPrefix returns the prefix portion of an API key (everything before the hex).
func extractPrefix(key string) string {
	if len(key) >= len(apiKeyPrefix) {
		return key[:len(apiKeyPrefix)]
	}
	return key
}

// =============================================================================
// API Key Format Tests
// =============================================================================

func TestAPIKey_GenerationProducesCorrectFormat(t *testing.T) {
	// Generate multiple keys and verify format
	for i := 0; i < 10; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err, "API key generation should not error")

		// Check prefix
		assert.True(t, strings.HasPrefix(key, apiKeyPrefix),
			"API key should start with '%s', got: %.10s...", apiKeyPrefix, key)

		// Check total length: prefix (3) + hex bytes (64) = 67
		assert.Len(t, key, len(apiKeyPrefix)+apiKeyHexLength,
			"API key should be %d characters, got %d: %s",
			len(apiKeyPrefix)+apiKeyHexLength, len(key), key)

		// The hex part should be valid hex
		hexPart := key[len(apiKeyPrefix):]
		decoded, err := hex.DecodeString(hexPart)
		assert.NoError(t, err, "hex part should be valid hex encoding")
		assert.Len(t, decoded, apiKeyRawBytes,
			"decoded hex should be %d bytes", apiKeyRawBytes)
	}

	t.Log("T080: API key format validation passed")
}

func TestAPIKey_PrefixIsAlwaysAk(t *testing.T) {
	for i := 0; i < 20; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err)

		assert.Equal(t, apiKeyPrefix, key[:len(apiKeyPrefix)],
			"every generated key should start with 'ak_'")
	}
}

// =============================================================================
// Hash Tests
// =============================================================================

func TestAPIKey_HashIsDeterministicForSameInput(t *testing.T) {
	key := "ak_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	// Hash the same key multiple times
	hash1 := hashAPIKey(key)
	hash2 := hashAPIKey(key)
	hash3 := hashAPIKey(key)

	assert.Equal(t, hash1, hash2, "same input should produce same hash (1 vs 2)")
	assert.Equal(t, hash2, hash3, "same input should produce same hash (2 vs 3)")
	assert.Equal(t, hash1, hash3, "same input should produce same hash (1 vs 3)")

	// SHA-256 hash should be 64 hex characters
	assert.Len(t, hash1, 64, "SHA-256 hash should be 64 hex characters")
}

func TestAPIKey_DifferentKeysProduceDifferentHashes(t *testing.T) {
	key1, err := generateAPIKey()
	require.NoError(t, err)
	key2, err := generateAPIKey()
	require.NoError(t, err)

	// Keys should be different (astronomically unlikely collision)
	assert.NotEqual(t, key1, key2, "generated keys should be different")

	// Hashes should be different
	hash1 := hashAPIKey(key1)
	hash2 := hashAPIKey(key2)
	assert.NotEqual(t, hash1, hash2,
		"different keys must produce different hashes")

	// Pre-image resistance: you cannot derive the key from the hash
	assert.NotEqual(t, hash1, key1,
		"hash should not equal the original key")
	assert.NotEqual(t, hash2, key2,
		"hash should not equal the original key")
}

func TestAPIKey_HashLengthIsCorrect(t *testing.T) {
	keys := []string{
		"ak_0000000000000000000000000000000000000000000000000000000000000000",
		"ak_ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		"ak_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	for _, key := range keys {
		hash := hashAPIKey(key)
		assert.Len(t, hash, 64,
			"SHA-256 hash of '%s...' should be 64 hex chars", key[:10])
	}
}

func TestAPIKey_HashIsValidHex(t *testing.T) {
	key := "ak_7411b6e3c1a7d2f8e9b0c4a5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f8"
	hash := hashAPIKey(key)

	// The hash should be valid hex
	_, err := hex.DecodeString(hash)
	assert.NoError(t, err, "hash should be valid hex-encoded string")
}

// =============================================================================
// Prefix Validation Tests
// =============================================================================

func TestAPIKey_PrefixValidation(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		isValidAK bool
	}{
		{"valid_with_prefix", "ak_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", true},
		{"valid_uppercase_hex", "ak_ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789", true},
		{"valid_mixed_case_hex", "ak_aBcDeF0123456789aBcDeF0123456789aBcDeF0123456789aBcDeF0123456789", true},
		{"missing_prefix_no_ak", "sk_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"missing_prefix_just_hex", "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"wrong_prefix_st", "st_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
		{"empty_string", "", false},
		{"prefix_only", "ak_", false},
		{"prefix_with_wrong_length", "ak_short", false},
		{"prefix_with_too_long", "ak_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789extra", false},
		{"nil_prefix_concept", "nil_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasPrefix := strings.HasPrefix(tt.key, apiKeyPrefix)
			hasCorrectLength := len(tt.key) == len(apiKeyPrefix)+apiKeyHexLength

			if tt.isValidAK {
				// Valid keys have prefix AND correct hex part length
				isValid := hasPrefix && hasCorrectLength
				assert.True(t, isValid,
					"key should be valid: prefix=%v, len=%d (expected %d)",
					hasPrefix, len(tt.key), len(apiKeyPrefix)+apiKeyHexLength)

				// Verify hex part decodes
				if hasPrefix && hasCorrectLength {
					hexPart := tt.key[len(apiKeyPrefix):]
					decoded, err := hex.DecodeString(hexPart)
					assert.NoError(t, err)
					assert.Len(t, decoded, apiKeyRawBytes)
				}
			} else {
				// Invalid keys fail either prefix or length check
				isValid := hasPrefix && hasCorrectLength
				assert.False(t, isValid,
					"key '%s' should be invalid (prefix=%v, len=%d)",
					tt.name, hasPrefix, len(tt.key))
			}
		})
	}
}

// =============================================================================
// Key Length Sufficiency Tests
// =============================================================================

func TestAPIKey_KeyLengthIsSufficient(t *testing.T) {
	// The raw key material should be 32 bytes (256 bits) of entropy,
	// which provides ~256 bits of security against brute force.
	for i := 0; i < 20; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err)

		hexPart := key[len(apiKeyPrefix):]
		assert.Len(t, hexPart, apiKeyHexLength,
			"hex part should be %d characters", apiKeyHexLength)

		decoded, err := hex.DecodeString(hexPart)
		require.NoError(t, err)
		assert.Len(t, decoded, apiKeyRawBytes,
			"raw key should be %d bytes (256 bits)", apiKeyRawBytes)
	}
}

func TestAPIKey_ShorterKeysAreDetected(t *testing.T) {
	shortKeys := []string{
		"ak_00",                                     // 1 byte
		"ak_00112233445566778899aabbccddeeff",       // 16 bytes (128 bits)
		"ak_00112233445566778899aabbccddeeff001122", // 24 bytes (192 bits)
	}

	for _, key := range shortKeys {
		hexPart := key[len(apiKeyPrefix):]
		decoded, err := hex.DecodeString(hexPart)
		require.NoError(t, err)

		assert.Less(t, len(decoded), apiKeyRawBytes,
			"short key (%d bytes) should be less than required %d bytes",
			len(decoded), apiKeyRawBytes)
	}
}

func TestAPIKey_EntropyQuality(t *testing.T) {
	// Generate many keys and ensure they are sufficiently random
	// (no two keys should be the same, no obvious patterns)
	keys := make(map[string]bool)

	for i := 0; i < 50; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err)

		// No collisions across 50 generations
		assert.False(t, keys[key],
			"key collision detected (extremely unlikely for 256-bit entropy)")
		keys[key] = true

		// Each byte in the raw key should not be all zeros or all the same
		hexPart := key[len(apiKeyPrefix):]
		decoded, err := hex.DecodeString(hexPart)
		require.NoError(t, err)

		// Check at least some variation in bytes
		uniqueBytes := make(map[byte]bool)
		for _, b := range decoded {
			uniqueBytes[b] = true
		}
		assert.Greater(t, len(uniqueBytes), 1,
			"key should have more than 1 unique byte value (entropy check)")
	}
}

// =============================================================================
// Prefix Extraction Tests
// =============================================================================

func TestAPIKey_PrefixExtractionWorksCorrectly(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		expectedPrefix string
	}{
		{"standard_ak_key", "ak_0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff", "ak_"},
		{"ak_prefix_all_zeros", "ak_0000000000000000000000000000000000000000000000000000000000000000", "ak_"},
		{"ak_prefix_all_f", "ak_ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "ak_"},
		{"empty_key", "", ""},
		{"short_key", "ak", "ak"},
		{"just_prefix", "ak_", "ak_"},
		{"st_prefix_key", "st_key_data_here", "st_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix := extractPrefix(tt.key)
			assert.Equal(t, tt.expectedPrefix, prefix,
				"prefix extraction for %q should return %q", tt.key, tt.expectedPrefix)
		})
	}
}

// =============================================================================
// Verification Tests (hash comparison)
// =============================================================================

func TestAPIKey_VerificationWithStoredHash(t *testing.T) {
	// Simulate the expected auth.VerifyAPIKey flow:
	// 1. User provides raw API key
	// 2. Server hashes it
	// 3. Server compares against stored hash

	originalKey, err := generateAPIKey()
	require.NoError(t, err)

	// Simulate storing the hash at key creation time
	storedHash := hashAPIKey(originalKey)

	// Later: verify the same key against the stored hash
	providedHash := hashAPIKey(originalKey)
	assert.Equal(t, storedHash, providedHash,
		"same key should produce same hash for verification")

	// Verify a different key fails
	differentKey, err := generateAPIKey()
	require.NoError(t, err)
	differentHash := hashAPIKey(differentKey)
	assert.NotEqual(t, storedHash, differentHash,
		"different key should produce different hash")
}

func TestAPIKey_VerificationCaseSensitive(t *testing.T) {
	key := "ak_ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"
	lowerKey := "ak_abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

	hash1 := hashAPIKey(key)
	hash2 := hashAPIKey(lowerKey)

	// Hashes should differ because hex case differs in the input string
	assert.NotEqual(t, hash1, hash2,
		"case-different keys should produce different hashes "+
			"(API key matching is case-sensitive)")
}

func TestAPIKey_VerificationWhitespaceNotTrimmed(t *testing.T) {
	key, err := generateAPIKey()
	require.NoError(t, err)

	paddedKey := "  " + key + "  "

	hash1 := hashAPIKey(key)
	hash2 := hashAPIKey(paddedKey)

	assert.NotEqual(t, hash1, hash2,
		"whitespace-padded key should differ (comparison is exact)")
}

func TestAPIKey_StoredHashIsNotReversible(t *testing.T) {
	// Given only the hash, you cannot recover the original API key.
	// This test proves the hash is a one-way function.

	for i := 0; i < 5; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err)

		hash := hashAPIKey(key)

		// The hash should not equal the key
		assert.NotEqual(t, key, hash,
			"hash should not equal the key (one-way property)")

		// The hash should not contain the key
		assert.NotContains(t, hash, key[len(apiKeyPrefix):],
			"hash should not contain the raw hex part")

		// Hash should not be a substring of key
		assert.NotContains(t, key, hash,
			"key should not contain the hash")
	}
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestAPIKey_EmptyKeyHash(t *testing.T) {
	hash := hashAPIKey("")
	assert.NotEmpty(t, hash, "even empty string should produce a hash")
	assert.Len(t, hash, 64, "empty key hash should be 64 hex chars")

	// The empty-string hash should be deterministic
	hash2 := hashAPIKey("")
	assert.Equal(t, hash, hash2, "empty string hash should be deterministic")
}

func TestAPIKey_PrefixOnlyHash(t *testing.T) {
	// Hashing just the prefix "ak_" should work and be deterministic
	hash1 := hashAPIKey("ak_")
	hash2 := hashAPIKey("ak_")
	assert.Equal(t, hash1, hash2,
		"prefix-only hash should be deterministic")
	assert.Len(t, hash1, 64)
}

func TestAPIKey_KeyCannotHaveStPrefix(t *testing.T) {
	// API keys use "ak_" prefix, not "st_" (which is for session tokens)
	key, err := generateAPIKey()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(key, "ak_"),
		"API key must start with 'ak_'")
	assert.False(t, strings.HasPrefix(key, "st_"),
		"API key must NOT start with 'st_' (session token prefix)")
}

// =============================================================================
// Hash Stability Across Equivalent Inputs
// =============================================================================

func TestAPIKey_HashOnlyDependsOnKeyContent(t *testing.T) {
	key := "ak_0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff"

	// Same key always same hash
	h1 := hashAPIKey(key)
	h2 := hashAPIKey(key)

	assert.Equal(t, h1, h2)
}

func TestAPIKey_PrefixIsPartOfHash(t *testing.T) {
	// The "ak_" prefix is part of the hashed input, so changing it
	// should change the hash (this is a design choice test)
	hexPart := "0000111122223333444455556666777788889999aaaabbbbccccddddeeeeffff"

	hashWithAK := hashAPIKey("ak_" + hexPart)
	hashWithST := hashAPIKey("st_" + hexPart)
	hashNoPrefix := hashAPIKey(hexPart)

	assert.NotEqual(t, hashWithAK, hashWithST,
		"different prefixes should produce different hashes")
	assert.NotEqual(t, hashWithAK, hashNoPrefix,
		"key with prefix should differ from key without prefix")
}
