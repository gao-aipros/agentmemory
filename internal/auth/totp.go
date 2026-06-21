package auth

import (
	"fmt"

	"github.com/pquerna/otp/totp"
)

// GenerateSecret generates a new TOTP secret for a user.
// Returns the base32-encoded secret string.
func GenerateSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "AgentMemory",
		AccountName: "user",
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate TOTP secret: %w", err)
	}
	return key.Secret(), nil
}

// ValidateTOTP validates a 6-digit TOTP code against a secret.
// Returns true if the code is valid at the current time.
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}
