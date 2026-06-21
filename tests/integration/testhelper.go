package integration

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
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

// SetupTestDB and TeardownTestDB are defined in main_test.go.
// They use a shared ParadeDB container with per-test database isolation,
// eliminating the ~5s container startup cost per test.
