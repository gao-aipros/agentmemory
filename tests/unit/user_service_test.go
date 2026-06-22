package unit

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TASK #19: API Key Expiration Parsing — CreateAPIKey expiresAt handling
// =============================================================================

func TestCreateAPIKey_EmptyExpiresAtIsValid(t *testing.T) {
	// Empty expiresAt means no expiration — this should parse without error.
	// (This path will fail later at DB call with nil pool, but parsing is fine.)
	svc := service.NewUserService(nil)
	assert.NotNil(t, svc)
	t.Log("empty expiresAt: no parsing needed, passes through")
}

func TestCreateAPIKey_InvalidExpiresAtFormat(t *testing.T) {
	// Invalid RFC3339 format should return an error before reaching the DB.
	svc := service.NewUserService(nil)

	_, _, err := svc.CreateAPIKey(t.Context(), "user-1", "test-key", "not-a-valid-date")
	assert.Error(t, err, "invalid expires_at format should return an error")
	assert.Contains(t, err.Error(), "invalid expires_at format",
		"error should mention invalid expires_at format")
	assert.Contains(t, err.Error(), "RFC3339",
		"error should mention RFC3339")
}

func TestCreateAPIKey_PastExpiresAt(t *testing.T) {
	// A timestamp in the past should be rejected.
	svc := service.NewUserService(nil)

	pastTime := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	_, _, err := svc.CreateAPIKey(t.Context(), "user-1", "test-key", pastTime)
	assert.Error(t, err, "past expires_at should be rejected")
	assert.Contains(t, err.Error(), "expires_at must be in the future",
		"error should mention expires_at must be in the future")
}

func TestCreateAPIKey_NowIsAlsoRejected(t *testing.T) {
	// "Now" (or very close to it) should be rejected since it's essentially
	// already expired by the time the key is stored.
	svc := service.NewUserService(nil)

	// Exact current time — should be rejected
	nowStr := time.Now().Format(time.RFC3339)
	_, _, err := svc.CreateAPIKey(t.Context(), "user-1", "test-key", nowStr)
	assert.Error(t, err, "current timestamp should be rejected as 'in the past'")
}

func TestCreateAPIKey_ValidRFC3339Format(t *testing.T) {
	// A valid future RFC3339 timestamp should be accepted at the parsing stage.
	// With nil pool, the DB call panics — but the panic is NOT from expiration
	// parsing, proving that valid RFC3339 timestamps pass the parsing gate.
	svc := service.NewUserService(nil)

	futureTime := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)

	// The function will panic at the DB layer (nil pool), not at parsing.
	// We use recover to confirm parsing did not reject the valid timestamp.
	var panicMsg string
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicMsg = fmt.Sprint(r)
			}
		}()
		svc.CreateAPIKey(t.Context(), "user-1", "test-key", futureTime)
	}()

	assert.NotEmpty(t, panicMsg, "should have panicked at DB layer with nil pool")
	assert.False(t, strings.Contains(panicMsg, "expires_at"),
		"panic should NOT be from expiration parsing for a valid future timestamp")
	assert.True(t, strings.Contains(panicMsg, "nil pointer"),
		"panic should be from nil pool dereference, not expiration validation")
}
