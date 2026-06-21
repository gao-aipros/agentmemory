package config

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Default connection pool sizing.
const (
	DefaultMaxConns        = 25
	DefaultMinConns        = 5
	DefaultMaxConnLifetime = 30 * time.Minute
	DefaultMaxConnIdleTime = 5 * time.Minute
)

// PoolConfig holds the connection pool configuration.
type PoolConfig struct {
	MaxConns int32
	MinConns int32
}

// PoolConfigFromEnv reads pool configuration from environment variables
// with sensible defaults.
func PoolConfigFromEnv() PoolConfig {
	return PoolConfig{
		MaxConns: int32(getEnvInt("DB_MAX_CONNS", DefaultMaxConns)),
		MinConns: int32(getEnvInt("DB_MIN_CONNS", DefaultMinConns)),
	}
}

// ValidatePoolConfig returns an error if the pool configuration is invalid.
// maxConns must be >= minConns, and both must be positive.
func ValidatePoolConfig(maxConns, minConns int32) error {
	if maxConns <= 0 {
		return fmt.Errorf("maxConns must be positive, got %d", maxConns)
	}
	if minConns < 0 {
		return fmt.Errorf("minConns must be non-negative, got %d", minConns)
	}
	if maxConns < minConns {
		return fmt.Errorf("maxConns (%d) must be >= minConns (%d)", maxConns, minConns)
	}
	return nil
}

// NewPool creates a new pgxpool connection pool with the given database URL.
// Connection pool sizing is read from the environment (DB_MAX_CONNS, DB_MIN_CONNS).
func NewPool(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	if dbURL == "" {
		return nil, fmt.Errorf("dbURL must not be empty")
	}

	cfg := PoolConfigFromEnv()

	if err := ValidatePoolConfig(cfg.MaxConns, cfg.MinConns); err != nil {
		return nil, fmt.Errorf("invalid pool config: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns

	// Connection lifecycle: prevent stale connections from lingering indefinitely.
	// MaxConnLifetime forces connection recycling after 30 minutes.
	poolConfig.MaxConnLifetime = DefaultMaxConnLifetime
	// MaxConnIdleTime closes connections idle for more than 5 minutes.
	poolConfig.MaxConnIdleTime = DefaultMaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify the pool is healthy before returning.
	if err := Ping(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database after pool creation: %w", err)
	}

	return pool, nil
}

// Ping verifies the database connection pool is healthy by executing a simple query.
func Ping(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("pool must not be nil")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return pool.Ping(pingCtx)
}

// ClosePool gracefully shuts down the connection pool, waiting for active
// queries to complete within the given timeout.
func ClosePool(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}
	pool.Close()
}

// ClosePoolWithTimeout gracefully shuts down the connection pool with a context deadline.
// It waits for active queries to complete, then closes the pool.
func ClosePoolWithTimeout(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	if pool == nil {
		return nil
	}

	closeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		pool.Close()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-closeCtx.Done():
		return fmt.Errorf("timeout closing pool: %w", closeCtx.Err())
	}
}
