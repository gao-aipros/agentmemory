package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// TokenPrefix is prepended to all JWT tokens (session tokens).
	TokenPrefix = "st_"
)

// Claims represents the JWT claims for AgentMemory session tokens.
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken creates a new signed JWT for the given user ID.
// The token is prefixed with "st_" to distinguish session tokens from API keys.
func GenerateToken(userID string, expiry time.Duration, secret string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return TokenPrefix + signed, nil
}

// ValidateToken validates a JWT token string. It strips the "st_" prefix,
// parses and validates the token, and returns the claims if valid.
func ValidateToken(tokenString string, secret string) (*Claims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("token is empty")
	}

	// Strip the "st_" prefix
	if !strings.HasPrefix(tokenString, TokenPrefix) {
		return nil, fmt.Errorf("invalid token format: missing st_ prefix")
	}

	signed := strings.TrimPrefix(tokenString, TokenPrefix)

	token, err := jwt.ParseWithClaims(signed, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
