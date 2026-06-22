package unit

import (
	"os"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/stretchr/testify/assert"
)

func clearEnv() {
	os.Unsetenv("DB_URL")
	os.Unsetenv("DB_MAX_CONNS")
	os.Unsetenv("DB_MIN_CONNS")
	os.Unsetenv("PORT")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("JWT_EXPIRY")
	os.Unsetenv("AGENTMEMORY_INJECT_CONTEXT")
	os.Unsetenv("AGENTMEMORY_SHARE_CONSOLIDATED")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("EMBEDDING_PROVIDER")
	os.Unsetenv("EMBEDDING_MODEL")
	os.Unsetenv("LLM_PROVIDER")
	os.Unsetenv("LLM_MODEL")
}

func TestLoad_Defaults(t *testing.T) {
	clearEnv()

	cfg := config.Load()

	assert.Empty(t, cfg.DBURL, "DB_URL should default to empty string")
	assert.Equal(t, 25, cfg.DBMaxConns, "DB_MAX_CONNS should default to 25")
	assert.Equal(t, 5, cfg.DBMinConns, "DB_MIN_CONNS should default to 5")
	assert.Equal(t, 8080, cfg.Port, "PORT should default to 8080")
	assert.Empty(t, cfg.JWTSecret, "JWT_SECRET should default to empty string")
	assert.Equal(t, 24*time.Hour, cfg.JWTExpiry, "JWT_EXPIRY should default to 24h")
	assert.False(t, cfg.InjectContext, "AGENTMEMORY_INJECT_CONTEXT should default to false")
	assert.False(t, cfg.ShareConsolidated, "AGENTMEMORY_SHARE_CONSOLIDATED should default to false")
	assert.Equal(t, "info", cfg.LogLevel, "LOG_LEVEL should default to info")
	assert.Empty(t, cfg.EmbeddingProvider, "EMBEDDING_PROVIDER should default to empty")
	assert.Empty(t, cfg.EmbeddingModel, "EMBEDDING_MODEL should default to empty")
	assert.Empty(t, cfg.LLMProvider, "LLM_PROVIDER should default to empty")
	assert.Empty(t, cfg.LLMModel, "LLM_MODEL should default to empty")
}

func TestLoad_EnvOverrides(t *testing.T) {
	clearEnv()

	os.Setenv("DB_URL", "postgres://localhost:5432/testdb")
	os.Setenv("DB_MAX_CONNS", "50")
	os.Setenv("DB_MIN_CONNS", "10")
	os.Setenv("PORT", "9090")
	os.Setenv("JWT_SECRET", "super-secret-key")
	os.Setenv("JWT_EXPIRY", "12h")
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "true")
	os.Setenv("AGENTMEMORY_SHARE_CONSOLIDATED", "true")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("EMBEDDING_PROVIDER", "openai")
	os.Setenv("EMBEDDING_MODEL", "text-embedding-3-small")
	os.Setenv("LLM_PROVIDER", "anthropic")
	os.Setenv("LLM_MODEL", "claude-sonnet-4-20250514")

	cfg := config.Load()

	assert.Equal(t, "postgres://localhost:5432/testdb", cfg.DBURL)
	assert.Equal(t, 50, cfg.DBMaxConns)
	assert.Equal(t, 10, cfg.DBMinConns)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "super-secret-key", cfg.JWTSecret)
	assert.Equal(t, 12*time.Hour, cfg.JWTExpiry)
	assert.True(t, cfg.InjectContext)
	assert.True(t, cfg.ShareConsolidated)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, "openai", cfg.EmbeddingProvider)
	assert.Equal(t, "text-embedding-3-small", cfg.EmbeddingModel)
	assert.Equal(t, "anthropic", cfg.LLMProvider)
	assert.Equal(t, "claude-sonnet-4-20250514", cfg.LLMModel)
}

func TestLoad_InvalidValuesFallbackToDefaults(t *testing.T) {
	// When invalid values are provided, the config should fall back to defaults.
	clearEnv()

	os.Setenv("DB_MAX_CONNS", "not-a-number")
	os.Setenv("PORT", "invalid-port")
	os.Setenv("JWT_EXPIRY", "not-a-duration")
	os.Setenv("AGENTMEMORY_INJECT_CONTEXT", "yes") // invalid bool

	cfg := config.Load()

	// Invalid values should fall back to defaults
	assert.Equal(t, 25, cfg.DBMaxConns, "invalid DB_MAX_CONNS should fall back to 25")
	assert.Equal(t, 8080, cfg.Port, "invalid PORT should fall back to 8080")
	assert.Equal(t, 24*time.Hour, cfg.JWTExpiry, "invalid JWT_EXPIRY should fall back to 24h")
	assert.False(t, cfg.InjectContext, "invalid bool should fall back to false")
}

func TestLoad_BooleanVariants(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{"true_lower", "true", true},
		{"true_upper", "TRUE", true},
		{"true_mixed", "True", true},
		{"false_lower", "false", false},
		{"false_upper", "FALSE", false},
		{"one", "1", true},
		{"zero", "0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv()
			os.Setenv("AGENTMEMORY_INJECT_CONTEXT", tt.value)
			cfg := config.Load()
			assert.Equal(t, tt.expected, cfg.InjectContext,
				"AGENTMEMORY_INJECT_CONTEXT=%s should parse as %v", tt.value, tt.expected)
		})
	}
}

func TestLoad_DurationVariants(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected time.Duration
	}{
		{"hours", "2h", 2 * time.Hour},
		{"minutes", "90m", 90 * time.Minute},
		{"seconds", "3600s", 3600 * time.Second},
		{"mixed", "1h30m", 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv()
			os.Setenv("JWT_EXPIRY", tt.value)
			cfg := config.Load()
			assert.Equal(t, tt.expected, cfg.JWTExpiry,
				"JWT_EXPIRY=%s should parse as %v", tt.value, tt.expected)
		})
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	clearEnv()

	// Only override a subset of values
	os.Setenv("PORT", "3000")
	os.Setenv("LOG_LEVEL", "warn")

	cfg := config.Load()

	// Overridden values
	assert.Equal(t, 3000, cfg.Port)
	assert.Equal(t, "warn", cfg.LogLevel)

	// Defaults should still apply for others
	assert.Equal(t, 25, cfg.DBMaxConns)
	assert.Equal(t, 5, cfg.DBMinConns)
	assert.False(t, cfg.InjectContext)
}

// Verify the Config struct is not nil after Load
func TestLoad_ReturnsNonNilConfig(t *testing.T) {
	clearEnv()
	cfg := config.Load()
	assert.NotNil(t, cfg, "Load() should return a non-nil Config")
}

// =============================================================================
// TASK #6: GetJWTSecret — fail loudly when JWT_SECRET is not set
// =============================================================================

func TestGetJWTSecret_EmptyEnvReturnsError(t *testing.T) {
	clearEnv()

	secret, err := config.GetJWTSecret()
	assert.Error(t, err, "GetJWTSecret should return an error when JWT_SECRET is empty")
	assert.Empty(t, secret, "secret should be empty when an error is returned")
	assert.Contains(t, err.Error(), "JWT_SECRET", "error message should mention JWT_SECRET")
}

func TestGetJWTSecret_SetEnvReturnsValue(t *testing.T) {
	clearEnv()
	os.Setenv("JWT_SECRET", "my-production-secret")

	secret, err := config.GetJWTSecret()
	assert.NoError(t, err, "GetJWTSecret should not error when JWT_SECRET is set")
	assert.Equal(t, "my-production-secret", secret, "should return the env var value")
}

func TestGetJWTSecret_NoFallbackToHardcoded(t *testing.T) {
	clearEnv()

	_, err := config.GetJWTSecret()
	assert.Error(t, err, "GetJWTSecret MUST NOT fall back to a hardcoded default")
	// The old hardcoded value should never appear in error or return
	assert.NotContains(t, err.Error(), "agentmemory-dev-secret", "error must not leak old hardcoded secret")
}
