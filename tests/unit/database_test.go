package unit

import (
	"os"
	"testing"

	"github.com/agentmemory/agentmemory/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Pool Configuration Validation Tests
// =============================================================================

func TestValidatePoolConfig_ValidConfigs(t *testing.T) {
	tests := []struct {
		name      string
		maxConns  int32
		minConns  int32
	}{
		{"equal_max_and_min", 10, 10},
		{"max_greater_than_min", 50, 5},
		{"min_zero", 25, 0},
		{"large_max", 1000, 1},
		{"default_values", 25, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidatePoolConfig(tt.maxConns, tt.minConns)
			assert.NoError(t, err)
		})
	}
}

func TestValidatePoolConfig_InvalidConfigs(t *testing.T) {
	tests := []struct {
		name      string
		maxConns  int32
		minConns  int32
		wantErr   string
	}{
		{
			name:     "max_zero",
			maxConns: 0,
			minConns: 5,
			wantErr:  "maxConns must be positive",
		},
		{
			name:     "max_negative",
			maxConns: -1,
			minConns: 5,
			wantErr:  "maxConns must be positive",
		},
		{
			name:     "min_negative",
			maxConns: 25,
			minConns: -1,
			wantErr:  "minConns must be non-negative",
		},
		{
			name:     "min_greater_than_max",
			maxConns: 5,
			minConns: 25,
			wantErr:  "maxConns (5) must be >= minConns (25)",
		},
		{
			name:     "min_greater_than_max_large_diff",
			maxConns: 1,
			minConns: 100,
			wantErr:  "maxConns (1) must be >= minConns (100)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.ValidatePoolConfig(tt.maxConns, tt.minConns)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPoolConfigFromEnv_Defaults(t *testing.T) {
	os.Unsetenv("DB_MAX_CONNS")
	os.Unsetenv("DB_MIN_CONNS")

	cfg := config.PoolConfigFromEnv()

	assert.Equal(t, int32(config.DefaultMaxConns), cfg.MaxConns)
	assert.Equal(t, int32(config.DefaultMinConns), cfg.MinConns)
}

func TestPoolConfigFromEnv_CustomValues(t *testing.T) {
	os.Setenv("DB_MAX_CONNS", "50")
	os.Setenv("DB_MIN_CONNS", "10")
	defer os.Unsetenv("DB_MAX_CONNS")
	defer os.Unsetenv("DB_MIN_CONNS")

	cfg := config.PoolConfigFromEnv()

	assert.Equal(t, int32(50), cfg.MaxConns)
	assert.Equal(t, int32(10), cfg.MinConns)
}

func TestPoolConfigFromEnv_InvalidValuesFallback(t *testing.T) {
	os.Setenv("DB_MAX_CONNS", "not-a-number")
	os.Setenv("DB_MIN_CONNS", "also-invalid")
	defer os.Unsetenv("DB_MAX_CONNS")
	defer os.Unsetenv("DB_MIN_CONNS")

	cfg := config.PoolConfigFromEnv()

	// Invalid values should fall back to defaults
	assert.Equal(t, int32(config.DefaultMaxConns), cfg.MaxConns)
	assert.Equal(t, int32(config.DefaultMinConns), cfg.MinConns)
}

// =============================================================================
// DSN Parsing Edge Case Tests
// =============================================================================

func TestParseConfig_ValidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"standard_postgres", "postgres://user:pass@localhost:5432/dbname"},
		{"postgresql_scheme", "postgresql://user:pass@localhost:5432/dbname"},
		{"with_sslmode", "postgres://user:pass@localhost:5432/dbname?sslmode=disable"},
		{"with_multiple_params", "postgres://user:pass@localhost:5432/dbname?sslmode=require&connect_timeout=10"},
		{"ipv4_address", "postgres://user:pass@192.168.1.1:5432/dbname"},
		{"no_port_defaults", "postgres://user:pass@localhost/dbname"},
		{"with_special_chars_in_password", "postgres://user:p%40ss@localhost:5432/dbname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := pgxpool.ParseConfig(tt.url)
			assert.NoError(t, err)
			assert.NotNil(t, cfg)
			assert.NotNil(t, cfg.ConnConfig)
		})
	}
}

func TestParseConfig_InvalidURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"missing_scheme", "user:pass@localhost:5432/dbname"},
		{"invalid_scheme", "mysql://user:pass@localhost:5432/dbname"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pgxpool.ParseConfig(tt.url)
			assert.Error(t, err, "expected error for URL: %q", tt.url)
		})
	}
}

func TestParseConfig_ExtractsComponents(t *testing.T) {
	url := "postgres://testuser:testpass@testhost:5432/testdb?sslmode=disable"
	cfg, err := pgxpool.ParseConfig(url)
	assert.NoError(t, err)

	assert.Equal(t, "testuser", cfg.ConnConfig.User)
	assert.Equal(t, "testhost", cfg.ConnConfig.Host)
	assert.Equal(t, "testdb", cfg.ConnConfig.Database)
	assert.Equal(t, uint16(5432), cfg.ConnConfig.Port)
}
