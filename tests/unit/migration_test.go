package unit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findMigrationsDir walks up from the working directory until it finds go.mod,
// then returns the migrations/ subdirectory under that module root.
func findMigrationsDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "migrations")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cannot find module root (go.mod) not found walking up from working directory")
		}
		dir = parent
	}
}

// readFile reads a file and fails the test if it cannot be read.
func readMigrationFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read %s", path)
	return string(content)
}

// =============================================================================
// Migration 009: Owner columns for compressed_observations, session_summaries, lessons
// =============================================================================

// TestMigration009OwnerColumns_FilesExist verifies that the 009_owner_columns
// .up.sql and .down.sql migration files exist.
func TestMigration009OwnerColumns_FilesExist(t *testing.T) {
	migrationsDir := findMigrationsDir(t)

	upPath := filepath.Join(migrationsDir, "009_owner_columns.up.sql")
	require.FileExists(t, upPath, "009_owner_columns.up.sql must exist")

	downPath := filepath.Join(migrationsDir, "009_owner_columns.down.sql")
	require.FileExists(t, downPath, "009_owner_columns.down.sql must exist")
}

// TestMigration009OwnerColumns_UpContent verifies that the UP migration
// contains the expected ALTER TABLE and CREATE INDEX statements for all
// three tables: compressed_observations, session_summaries, lessons.
func TestMigration009OwnerColumns_UpContent(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	upPath := filepath.Join(migrationsDir, "009_owner_columns.up.sql")
	content := readMigrationFile(t, upPath)

	// --- compressed_observations ---
	assert.Contains(t, content, "ALTER TABLE compressed_observations",
		"must alter compressed_observations table")
	assert.Contains(t, content, "ADD COLUMN owner_type TEXT NOT NULL DEFAULT 'user'",
		"compressed_observations must have owner_type column")
	assert.Contains(t, content, "ADD COLUMN owner_user_id TEXT",
		"compressed_observations must have owner_user_id column")
	assert.Contains(t, content, "ADD COLUMN owner_team_id TEXT",
		"compressed_observations must have owner_team_id column")
	assert.Contains(t, content, "idx_compressed_observations_owner_user_id",
		"must create index on compressed_observations(owner_user_id)")
	assert.Contains(t, content, "idx_compressed_observations_owner_team_id",
		"must create index on compressed_observations(owner_team_id)")

	// --- session_summaries ---
	assert.Contains(t, content, "ALTER TABLE session_summaries",
		"must alter session_summaries table")
	assert.Contains(t, content, "idx_session_summaries_owner_user_id",
		"must create index on session_summaries(owner_user_id)")
	assert.Contains(t, content, "idx_session_summaries_owner_team_id",
		"must create index on session_summaries(owner_team_id)")

	// --- lessons ---
	assert.Contains(t, content, "ALTER TABLE lessons",
		"must alter lessons table")
	assert.Contains(t, content, "idx_lessons_owner_user_id",
		"must create index on lessons(owner_user_id)")

	// Owner columns must NOT have FK constraints (REFERENCES).
	// FKs will be added to ALL tables together in a future migration (#38, #39).
	// The existing observations and memories tables use bare TEXT columns.
	assert.NotContains(t, content, "REFERENCES",
		"owner columns must not have REFERENCES clauses; FKs deferred to future migration")
}

// TestMigration009OwnerColumns_DownContent verifies that the DOWN migration
// reverses the UP migration by dropping indexes then columns.
func TestMigration009OwnerColumns_DownContent(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	downPath := filepath.Join(migrationsDir, "009_owner_columns.down.sql")
	content := readMigrationFile(t, downPath)

	assert.Contains(t, content, "DROP INDEX IF EXISTS",
		"down migration must drop indexes")

	// All three table indexes must be dropped
	assert.Contains(t, content, "idx_compressed_observations_owner_user_id",
		"must drop index on compressed_observations(owner_user_id)")
	assert.Contains(t, content, "idx_compressed_observations_owner_team_id",
		"must drop index on compressed_observations(owner_team_id)")
	assert.Contains(t, content, "idx_session_summaries_owner_user_id",
		"must drop index on session_summaries(owner_user_id)")
	assert.Contains(t, content, "idx_session_summaries_owner_team_id",
		"must drop index on session_summaries(owner_team_id)")
	assert.Contains(t, content, "idx_lessons_owner_user_id",
		"must drop index on lessons(owner_user_id)")

	// All three tables must have DROP COLUMN
	assert.Contains(t, content, "compressed_observations",
		"down migration must reference compressed_observations")
	assert.Contains(t, content, "session_summaries",
		"down migration must reference session_summaries")
	assert.Contains(t, content, "lessons",
		"down migration must reference lessons")
}
