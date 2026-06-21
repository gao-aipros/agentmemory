package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// ParadeDBImage is the ParadeDB Docker image to use for integration tests.
	ParadeDBImage = "paradedb/paradedb:0.24.1-pg18"

	// DBName, DBUser, DBPassword are the default credentials for the test database.
	DBName     = "agentmemory_test"
	DBUser     = "agentmemory"
	DBPassword = "testpassword"
)

// TestDB wraps a testcontainers ParadeDB instance and provides a pgxpool connection.
type TestDB struct {
	Container *postgres.PostgresContainer
	ConnStr   string
	Pool      *pgxpool.Pool
}

// SetupTestDB starts a ParadeDB container, enables extensions, and returns
// a connection string. The caller must call TeardownTestDB to clean up.
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		ParadeDBImage,
		postgres.WithDatabase(DBName),
		postgres.WithUsername(DBUser),
		postgres.WithPassword(DBPassword),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start ParadeDB container: %v", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	// Create a connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create connection pool: %v", err)
	}

	// Enable required extensions
	extensions := []string{"pg_search", "vector"}
	for _, ext := range extensions {
		_, err := pool.Exec(ctx, fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", ext))
		if err != nil {
			t.Fatalf("failed to enable extension %s: %v", ext, err)
		}
	}

	// Verify extensions are enabled
	var extName string
	for _, ext := range extensions {
		err = pool.QueryRow(ctx, "SELECT extname FROM pg_extension WHERE extname = $1", ext).Scan(&extName)
		if err != nil {
			t.Fatalf("extension %s not found after creation: %v", ext, err)
		}
	}

	t.Logf("ParadeDB container started: %s", connStr)

	return &TestDB{
		Container: pgContainer,
		ConnStr:   connStr,
		Pool:      pool,
	}
}

// TeardownTestDB cleans up the test database and container.
func TeardownTestDB(t *testing.T, db *TestDB) {
	t.Helper()

	if db == nil {
		return
	}

	if db.Pool != nil {
		db.Pool.Close()
	}

	if db.Container != nil {
		ctx := context.Background()
		if err := db.Container.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate container: %v", err)
		}
	}
}
