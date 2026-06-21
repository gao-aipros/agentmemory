package integration

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedContainer *postgres.PostgresContainer
	sharedConnStr   string
	sharedMu        sync.Mutex
	testDBCounter   int
)

// TestMain starts a single shared ParadeDB container that all tests reuse.
// Each test still gets its own database for schema isolation.
func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		ParadeDBImage,
		postgres.WithUsername(DBUser),
		postgres.WithPassword(DBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("failed to start shared ParadeDB: %v", err)
	}
	sharedContainer = pgContainer

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %v", err)
	}
	sharedConnStr = connStr

	log.Printf("Shared ParadeDB container ready (saves ~5s per test)")

	code := m.Run()

	if err := pgContainer.Terminate(ctx); err != nil {
		log.Printf("warning: terminate shared container: %v", err)
	}
	os.Exit(code)
}

// SetupTestDB creates a fresh database on the shared ParadeDB container.
// Each test gets complete schema isolation via its own database, avoiding
// the 5s container startup cost per test.
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()

	sharedMu.Lock()
	testDBCounter++
	dbName := fmt.Sprintf("%s_%d", DBName, testDBCounter)
	sharedMu.Unlock()

	// Connect with retry — shared container may be briefly unavailable under load
	var adminPool *pgxpool.Pool
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		adminPool, err = pgxpool.New(ctx, sharedConnStr)
		if err == nil {
			if pingErr := adminPool.Ping(ctx); pingErr == nil {
				break
			}
			adminPool.Close()
		}
		if attempt < 4 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	if err != nil {
		t.Fatalf("failed to connect to admin DB after retries: %v", err)
	}

	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		adminPool.Close()
		t.Fatalf("failed to create test DB %s: %v", dbName, err)
	}
	adminPool.Close()

	// Replace the default 'postgres' database with the test-specific database
	testConnStr := strings.Replace(sharedConnStr, "/postgres?", "/"+dbName+"?", 1)
	pool, err := createPool(ctx, testConnStr)
	if err != nil {
		t.Fatalf("failed to connect to test DB %s: %v", dbName, err)
	}

	// Enable extensions on the test database
	for _, ext := range []string{"pg_search", "vector"} {
		if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", ext)); err != nil {
			pool.Close()
			t.Fatalf("failed to enable extension %s: %v", ext, err)
		}
	}

	tdb := &TestDB{
		Container: sharedContainer,
		ConnStr:   testConnStr,
		Pool:      pool,
	}

	// Register cleanup to drop the test database
	t.Cleanup(func() {
		pool.Close()
		adminCleanup, err := pgxpool.New(context.Background(), sharedConnStr)
		if err == nil {
			_, _ = adminCleanup.Exec(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			adminCleanup.Close()
		}
	})

	return tdb
}

// TeardownTestDB is a no-op with shared container — cleanup happens in t.Cleanup.
func TeardownTestDB(t *testing.T, db *TestDB) {
	t.Helper()
	// Cleanup is registered in SetupTestDB via t.Cleanup
}

// createPool creates a pgxpool from a connection string.
func createPool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	config.MaxConns = 5
	config.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return pool, nil
}
