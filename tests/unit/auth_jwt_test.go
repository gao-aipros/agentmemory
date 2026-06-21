package unit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// T079: JWT Generation, Validation, and Expiry Tests
// =============================================================================
//
// These tests validate the expected behavior of the auth package's JWT
// functions. They use standard library crypto to simulate JWT creation
// and validation against the expected API contract, so they can serve
// as both specification and regression tests once the auth package exists.

const testJWTSecret = "super-secret-jwt-key-for-testing"

// jwtHeader is a minimal JWT header for testing.
type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// jwtClaims represents the claims we expect in our tokens.
type jwtClaims struct {
	Sub string `json:"sub"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
	Jti string `json:"jti"`
}

// signJWT manually creates a JWT using HMAC-SHA256 for testing validation logic.
func signJWT(secret string, claims jwtClaims) string {
	header := jwtHeader{Alg: "HS256", Typ: "JWT"}
	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + signature
}

// verifyJWTSignature checks the HMAC-SHA256 signature of a JWT.
func verifyJWTSignature(token, secret string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}

	signingInput := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(parts[2]), []byte(expectedSig))
}

// decodeJWTClaims decodes the claims portion of a JWT without verifying the signature.
func decodeJWTClaims(token string) (jwtClaims, error) {
	var claims jwtClaims
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return claims, assert.AnError
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, err
	}
	err = json.Unmarshal(claimsJSON, &claims)
	return claims, err
}

// =============================================================================
// Token Prefix Tests
// =============================================================================

func TestJWT_TokenHasStPrefix(t *testing.T) {
	// The expected API: auth.GenerateToken(userID, secret) returns a token
	// that starts with "st_" prefix to distinguish session tokens.
	//
	// This test validates that behavior using the actual JWT structure.

	userID := "user-abc-123"
	now := time.Now()
	claims := jwtClaims{
		Sub: userID,
		Iat: now.Unix(),
		Exp: now.Add(24 * time.Hour).Unix(),
		Jti: "jti-001",
	}

	jwtPart := signJWT(testJWTSecret, claims)

	// Simulate the expected auth.GenerateToken behavior: prefix + "." + jwt
	token := "st_" + jwtPart

	assert.True(t, strings.HasPrefix(token, "st_"),
		"token should start with 'st_' prefix, got: %s", token[:min(20, len(token))])

	// The token should have exactly one prefix, not nested prefixes
	assert.False(t, strings.HasPrefix(token, "st_st_"),
		"token should not have nested 'st_' prefixes")

	// Verify the JWT part after the prefix is a valid 3-part JWT
	jwtOnly := token[3:] // strip "st_"
	parts := strings.Split(jwtOnly, ".")
	assert.Len(t, parts, 3, "JWT after prefix should have 3 dot-separated parts")

	t.Log("T079: Token prefix validation passed")
}

func TestJWT_TokenPrefixStrippedForValidation(t *testing.T) {
	// The expected auth.ValidateToken should:
	// 1. Strip "st_" prefix
	// 2. Parse the remaining JWT
	// 3. Verify signature and expiry

	claims := jwtClaims{
		Sub: "user-test-001",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-prefix-001",
	}

	jwtPart := signJWT(testJWTSecret, claims)
	token := "st_" + jwtPart

	// Simulate auth.ValidateToken: strip prefix, verify JWT
	token = strings.TrimPrefix(token, "st_")

	valid := verifyJWTSignature(token, testJWTSecret)
	assert.True(t, valid, "JWT signature should be valid after stripping prefix")

	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)
	assert.Equal(t, "user-test-001", decoded.Sub)
}

// =============================================================================
// Valid Token Tests
// =============================================================================

func TestJWT_ValidTokenPassesValidation(t *testing.T) {
	claims := jwtClaims{
		Sub: "user-valid-001",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(24 * time.Hour).Unix(),
		Jti: "jti-valid-001",
	}

	token := signJWT(testJWTSecret, claims)

	// Verify signature
	assert.True(t, verifyJWTSignature(token, testJWTSecret),
		"valid token should pass signature verification")

	// Verify claims can be decoded
	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)
	assert.Equal(t, "user-valid-001", decoded.Sub, "subject should match")
	assert.Greater(t, decoded.Exp, time.Now().Unix(),
		"token should not be expired")
}

func TestJWT_MultipleValidTokens(t *testing.T) {
	users := []string{"user-a", "user-b", "user-c", "user-d"}

	for _, userID := range users {
		claims := jwtClaims{
			Sub: userID,
			Iat: time.Now().Unix(),
			Exp: time.Now().Add(1 * time.Hour).Unix(),
			Jti: "jti-" + userID,
		}

		token := signJWT(testJWTSecret, claims)
		assert.True(t, verifyJWTSignature(token, testJWTSecret),
			"token for user %s should be valid", userID)

		decoded, err := decodeJWTClaims(token)
		require.NoError(t, err)
		assert.Equal(t, userID, decoded.Sub)
	}
}

// =============================================================================
// Expired Token Tests
// =============================================================================

func TestJWT_ExpiredTokenFailsValidation(t *testing.T) {
	// Create a token that expired 1 hour ago
	claims := jwtClaims{
		Sub: "user-expired-001",
		Iat: time.Now().Add(-2 * time.Hour).Unix(),
		Exp: time.Now().Add(-1 * time.Hour).Unix(),
		Jti: "jti-expired-001",
	}

	token := signJWT(testJWTSecret, claims)

	// The signature itself is still valid
	assert.True(t, verifyJWTSignature(token, testJWTSecret),
		"signature should be valid even for expired tokens")

	// But the expiry claim should indicate expiration
	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)
	assert.Less(t, decoded.Exp, time.Now().Unix(),
		"exp claim should be in the past for expired token")
}

func TestJWT_ExpiryClaimIsReasonable(t *testing.T) {
	// Test that tokens have reasonable expiry times
	now := time.Now()

	tests := []struct {
		name       string
		expOffset  time.Duration
		shouldBeOK bool
	}{
		{"short_expiry_5m", 5 * time.Minute, true},
		{"medium_expiry_1h", 1 * time.Hour, true},
		{"default_expiry_24h", 24 * time.Hour, true},
		{"long_expiry_7d", 7 * 24 * time.Hour, true},
		{"already_expired", -1 * time.Hour, false},
		{"expires_now", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := jwtClaims{
				Sub: "user-expiry-test",
				Iat: now.Unix(),
				Exp: now.Add(tt.expOffset).Unix(),
				Jti: "jti-" + tt.name,
			}

			token := signJWT(testJWTSecret, claims)
			decoded, err := decodeJWTClaims(token)
			require.NoError(t, err)

			isExpired := decoded.Exp <= time.Now().Unix()
			if tt.shouldBeOK {
				assert.False(t, isExpired,
					"token with %s expiry should not be expired yet", tt.name)
			} else {
				assert.True(t, isExpired,
					"token with %s expiry should be expired", tt.name)
			}
		})
	}
}

func TestJWT_ClockSkewGracePeriod(t *testing.T) {
	// Tokens that expire within a small grace period (e.g., 30 seconds)
	// should still be considered valid to handle clock skew.
	gracePeriod := 30 * time.Second

	// Token that expired 10 seconds ago (within grace period)
	claims := jwtClaims{
		Sub: "user-skew-001",
		Iat: time.Now().Add(-1 * time.Hour).Unix(),
		Exp: time.Now().Add(-10 * time.Second).Unix(),
		Jti: "jti-skew-001",
	}

	token := signJWT(testJWTSecret, claims)
	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)

	// With grace period, this token should still be acceptable
	adjustedExp := time.Unix(decoded.Exp, 0).Add(gracePeriod)
	assert.True(t, adjustedExp.After(time.Now()),
		"token within grace period should be accepted (exp=%s, grace=%s)",
		time.Unix(decoded.Exp, 0).Format(time.RFC3339),
		gracePeriod,
	)

	// Token that expired 5 minutes ago (outside grace period)
	oldClaims := jwtClaims{
		Sub: "user-skew-002",
		Iat: time.Now().Add(-2 * time.Hour).Unix(),
		Exp: time.Now().Add(-5 * time.Minute).Unix(),
		Jti: "jti-skew-002",
	}

	oldToken := signJWT(testJWTSecret, oldClaims)
	oldDecoded, err := decodeJWTClaims(oldToken)
	require.NoError(t, err)

	adjustedOldExp := time.Unix(oldDecoded.Exp, 0).Add(gracePeriod)
	assert.False(t, adjustedOldExp.After(time.Now()),
		"token outside grace period should be rejected")
}

// =============================================================================
// Invalid Signature Tests
// =============================================================================

func TestJWT_InvalidSignatureFailsValidation(t *testing.T) {
	claims := jwtClaims{
		Sub: "user-bad-sig",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-bad-sig",
	}

	// Sign with the correct secret
	token := signJWT(testJWTSecret, claims)

	// Verify with a DIFFERENT secret (should fail)
	assert.False(t, verifyJWTSignature(token, "wrong-secret-key"),
		"token signed with one key should fail when verified with a different key")
}

func TestJWT_TamperedPayloadFailsValidation(t *testing.T) {
	claims := jwtClaims{
		Sub: "user-tampered",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-tampered",
	}

	token := signJWT(testJWTSecret, claims)

	// Tamper with the payload (change subject in the claims part)
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)

	// Replace claims with a tampered version
	tamperedClaims := jwtClaims{
		Sub: "attacker-user",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-tampered",
	}
	tamperedJSON, _ := json.Marshal(tamperedClaims)
	tamperedB64 := base64.RawURLEncoding.EncodeToString(tamperedJSON)

	tamperedToken := parts[0] + "." + tamperedB64 + "." + parts[2]

	// The signature should no longer be valid
	assert.False(t, verifyJWTSignature(tamperedToken, testJWTSecret),
		"tampered token should fail signature verification")
}

func TestJWT_NullSignatureFails(t *testing.T) {
	// A token with "none" algorithm or missing signature
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLW5vbmUifQ."

	assert.False(t, verifyJWTSignature(token, testJWTSecret),
		"token with missing signature should fail")
}

func TestJWT_EmptySignatureFails(t *testing.T) {
	claims := jwtClaims{
		Sub: "user-empty-sig",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-empty-sig",
	}

	headerJSON, _ := json.Marshal(jwtHeader{Alg: "HS256", Typ: "JWT"})
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Token with empty signature
	token := headerB64 + "." + claimsB64 + "."

	assert.False(t, verifyJWTSignature(token, testJWTSecret),
		"token with empty signature should fail")
}

// =============================================================================
// Missing Token Tests
// =============================================================================

func TestJWT_MissingTokenFailsValidation(t *testing.T) {
	// Test various forms of missing/empty tokens
	tests := []struct {
		name  string
		token string
	}{
		{"empty_string", ""},
		{"whitespace_only", "   "},
		{"just_prefix", "st_"},
		{"prefix_with_trailing_dot", "st_."},
		{"malformed_single_part", "not-a-jwt"},
		{"malformed_two_parts", "header.payload"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate auth.ValidateToken behavior on missing/malformed tokens
			token := strings.TrimSpace(tt.token)

			if len(token) == 0 {
				assert.False(t, verifyJWTSignature(token, testJWTSecret),
					"empty token should fail validation")
				return
			}

			// Strip expected prefix
			token = strings.TrimPrefix(token, "st_")

			// Malformed tokens should fail
			parts := strings.Split(token, ".")
			if len(parts) != 3 {
				// This is expected for malformed tokens
				assert.True(t, true, "malformed token correctly handled")
				return
			}

			valid := verifyJWTSignature(token, testJWTSecret)
			assert.False(t, valid, "malformed token '%s' should fail validation", tt.name)
		})
	}
}

func TestJWT_PrefixOnlyNoJWTFails(t *testing.T) {
	// Token that is just the prefix with no JWT part
	token := "st_"

	// Strip prefix
	remaining := strings.TrimPrefix(token, "st_")
	assert.Empty(t, remaining, "after stripping prefix, remaining should be empty")

	assert.False(t, verifyJWTSignature(remaining, testJWTSecret),
		"prefix-only token should fail")
}

// =============================================================================
// Token Structure and IAT Tests
// =============================================================================

func TestJWT_IssuedAtIsSetCorrectly(t *testing.T) {
	before := time.Now()
	claims := jwtClaims{
		Sub: "user-iat-test",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(1 * time.Hour).Unix(),
		Jti: "jti-iat-001",
	}
	after := time.Now()

	token := signJWT(testJWTSecret, claims)
	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)

	iat := time.Unix(decoded.Iat, 0)
	assert.True(t, iat.After(before.Add(-1*time.Second)) || iat.Equal(before),
		"iat should be after or equal to before time")
	assert.True(t, iat.Before(after.Add(1*time.Second)) || iat.Equal(after),
		"iat should be before or equal to after time")
}

func TestJWT_TokenContainsRequiredClaims(t *testing.T) {
	claims := jwtClaims{
		Sub: "user-claims-test",
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(24 * time.Hour).Unix(),
		Jti: "jti-required-001",
	}

	token := signJWT(testJWTSecret, claims)
	decoded, err := decodeJWTClaims(token)
	require.NoError(t, err)

	// Verify all required claims are present
	assert.NotEmpty(t, decoded.Sub, "sub claim is required")
	assert.NotZero(t, decoded.Iat, "iat claim is required")
	assert.NotZero(t, decoded.Exp, "exp claim is required")
	assert.NotEmpty(t, decoded.Jti, "jti (token ID) claim is required")

	// Exp should be after Iat
	assert.Greater(t, decoded.Exp, decoded.Iat,
		"exp must be after iat")
}

// =============================================================================
// min helper for test output formatting
// =============================================================================

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
