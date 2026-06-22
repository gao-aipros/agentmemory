package unit

import (
	"bytes"
	"log/slog"
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Task #14: Warn on config parse errors
// =============================================================================
// Bug: getEnvInt, getEnvBool, getEnvDuration silently use fallback when
// strconv parsing fails, with zero logging. The fix adds slog.Warn calls.
//
// These tests verify:
// 1. Fallback behavior still works correctly.
// 2. A warning is logged when a parse error occurs.

func TestConfigParseWarnsOnInvalidInt(t *testing.T) {
	os.Unsetenv("PORT")
	os.Setenv("PORT", "not-a-number")

	// Capture slog output
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	cfg := config.Load()

	// Fallback behavior still works
	assert.Equal(t, 8080, cfg.Port, "invalid PORT should fall back to 8080")

	// Warning should have been logged
	logOutput := buf.String()
	assert.NotEmpty(t, logOutput, "should have logged a warning for invalid int")
	assert.Contains(t, logOutput, "PORT", "warning should mention the env var name")
}

func TestConfigParseWarnsOnInvalidBool(t *testing.T) {
	os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "yes")

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	cfg := config.Load()

	// Fallback behavior still works
	assert.False(t, cfg.InjectContext, "invalid bool should fall back to false")

	// Warning should have been logged
	logOutput := buf.String()
	assert.NotEmpty(t, logOutput, "should have logged a warning for invalid bool")
	assert.Contains(t, logOutput, "AGENTMEMORY_INJECT_CONTEXT", "warning should mention the env var name")
}

func TestConfigParseWarnsOnInvalidDuration(t *testing.T) {
	os.Unsetenv("JWT_EXPIRY")
	os.Setenv("JWT_EXPIRY", "not-a-duration")

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	cfg := config.Load()

	// Fallback behavior still works
	assert.Equal(t, 24*3600*1000000000, int(cfg.JWTExpiry), "invalid duration should fall back to 24h") // 24h in nanoseconds

	// Warning should have been logged
	logOutput := buf.String()
	assert.NotEmpty(t, logOutput, "should have logged a warning for invalid duration")
	assert.Contains(t, logOutput, "JWT_EXPIRY", "warning should mention the env var name")
}

func TestConfigParseNoWarnOnValidValues(t *testing.T) {
	clearEnv()
	os.Unsetenv("PORT")
	os.Setenv("PORT", "9090")

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	cfg := config.Load()

	assert.Equal(t, 9090, cfg.Port, "valid PORT should be parsed correctly")

	// No warning should be logged for a valid value
	logOutput := buf.String()
	assert.Empty(t, logOutput, "should not log warnings for valid values")
}

func TestConfigParseWarnsMultipleFailures(t *testing.T) {
	// Test that multiple parse failures each produce a warning.
	os.Unsetenv("DB_MAX_CONNS")
	os.Setenv("DB_MAX_CONNS", "abc")
	os.Unsetenv("PORT")
	os.Setenv("PORT", "xyz")
	os.Unsetenv("JWT_EXPIRY")
	os.Setenv("JWT_EXPIRY", "bad")

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)
	prevLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prevLogger)

	cfg := config.Load()

	assert.Equal(t, 25, cfg.DBMaxConns)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 24*3600*1000000000, int(cfg.JWTExpiry))

	// Each failure should produce a warning
	logOutput := buf.String()
	assert.Contains(t, logOutput, "DB_MAX_CONNS")
	assert.Contains(t, logOutput, "PORT")
	assert.Contains(t, logOutput, "JWT_EXPIRY")
}
