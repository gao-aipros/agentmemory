package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMigrations reads all up migration SQL files and executes them against the test database.
func runMigrations(t *testing.T, db *TestDB) {
	t.Helper()

	migrations := []string{
		"001_initial_schema.up.sql",
		"002_observations.up.sql",
		"003_embeddings.up.sql",
		"004_compressed.up.sql",
		"005_summaries.up.sql",
		"006_memories.up.sql",
	}

	ctx := context.Background()
	for _, m := range migrations {
		migrationPath := filepath.Join("..", "..", "migrations", m)
		sqlBytes, err := os.ReadFile(migrationPath)
		require.NoError(t, err, "failed to read migration file: %s", migrationPath)

		_, err = db.Pool.Exec(ctx, string(sqlBytes))
		require.NoError(t, err, "failed to run migration: %s", m)
	}
}

// =============================================================================
// Table Existence Tests
// =============================================================================

func TestSchema_TablesExist(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	expectedTables := []string{"users", "api_keys", "teams", "team_members", "sessions"}

	ctx := context.Background()
	for _, table := range expectedTables {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		require.NoError(t, err, "failed to check table existence: %s", table)
		assert.True(t, exists, "table %s should exist", table)
	}
}

// =============================================================================
// Constraints Tests
// =============================================================================

func TestSchema_UniqueConstraints(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	// Verify email UNIQUE constraint on users
	expectedUniques := map[string][]string{
		"users":    {"users_email_key"},
		"api_keys": {"api_keys_key_hash_key"},
	}

	for table, constraintNames := range expectedUniques {
		for _, constraintName := range constraintNames {
			var exists bool
			err := db.Pool.QueryRow(ctx,
				`SELECT EXISTS (
					SELECT FROM information_schema.table_constraints
					WHERE constraint_schema = 'public'
					AND table_name = $1
					AND constraint_name = $2
					AND constraint_type = 'UNIQUE'
				)`,
				table, constraintName,
			).Scan(&exists)
			require.NoError(t, err, "failed to check unique constraint: %s.%s", table, constraintName)
			assert.True(t, exists, "unique constraint %s should exist on table %s", constraintName, table)
		}
	}
}

func TestSchema_ForeignKeys(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	type FKCheck struct {
		table      string
		constraint string
	}

	expectedFKs := []FKCheck{
		{"api_keys", "api_keys_user_id_fkey"},
		{"team_members", "team_members_team_id_fkey"},
		{"team_members", "team_members_user_id_fkey"},
		{"teams", "teams_owner_id_fkey"},
		{"sessions", "sessions_user_id_fkey"},
		{"sessions", "sessions_team_id_fkey"},
	}

	for _, fk := range expectedFKs {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT FROM information_schema.table_constraints
				WHERE constraint_schema = 'public'
				AND table_name = $1
				AND constraint_name = $2
				AND constraint_type = 'FOREIGN KEY'
			)`,
			fk.table, fk.constraint,
		).Scan(&exists)
		require.NoError(t, err, "failed to check FK: %s.%s", fk.table, fk.constraint)
		assert.True(t, exists, "foreign key %s should exist on table %s", fk.constraint, fk.table)
	}
}

func TestSchema_NotNullColumns(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	type NotNullCheck struct {
		table  string
		column string
	}

	expectedNotNulls := []NotNullCheck{
		// users
		{"users", "id"},
		{"users", "email"},
		{"users", "password_hash"},
		{"users", "name"},
		{"users", "totp_enabled"},
		{"users", "created_at"},
		// api_keys
		{"api_keys", "id"},
		{"api_keys", "user_id"},
		{"api_keys", "label"},
		{"api_keys", "key_hash"},
		{"api_keys", "created_at"},
		// teams
		{"teams", "id"},
		{"teams", "name"},
		{"teams", "owner_id"},
		{"teams", "default_visibility"},
		{"teams", "created_at"},
		// team_members
		{"team_members", "id"},
		{"team_members", "team_id"},
		{"team_members", "user_id"},
		{"team_members", "joined_at"},
		// sessions
		{"sessions", "id"},
		{"sessions", "user_id"},
		{"sessions", "started_at"},
		{"sessions", "status"},
	}

	for _, nn := range expectedNotNulls {
		var isNullable string
		err := db.Pool.QueryRow(ctx,
			`SELECT is_nullable
			 FROM information_schema.columns
			 WHERE table_schema = 'public'
			 AND table_name = $1
			 AND column_name = $2`,
			nn.table, nn.column,
		).Scan(&isNullable)
		require.NoError(t, err, "failed to check NOT NULL: %s.%s", nn.table, nn.column)
		assert.Equal(t, "NO", isNullable, "column %s.%s should be NOT NULL", nn.table, nn.column)
	}
}

// =============================================================================
// Index Tests
// =============================================================================

func TestSchema_IndexesExist(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	expectedIndexes := []string{
		"idx_api_keys_user_id",
		"idx_api_keys_key_hash",
		"idx_team_members_team_id",
		"idx_team_members_user_id",
		"idx_sessions_user_id",
		"idx_sessions_team_id",
		"idx_sessions_status",
		"idx_sessions_user_status",
	}

	for _, indexName := range expectedIndexes {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS (
				SELECT FROM pg_indexes
				WHERE schemaname = 'public'
				AND indexname = $1
			)`,
			indexName,
		).Scan(&exists)
		require.NoError(t, err, "failed to check index: %s", indexName)
		assert.True(t, exists, "index %s should exist", indexName)
	}
}

// =============================================================================
// Basic CRUD Smoke Tests
// =============================================================================

func TestSchema_BasicCRUD(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	// Insert a user
	var userID, userEmail, userName string
	err := db.Pool.QueryRow(ctx,
		"INSERT INTO users (id, email, password_hash, name) VALUES ('u1', 'test@example.com', 'hash123', 'Test User') RETURNING id, email, name",
	).Scan(&userID, &userEmail, &userName)
	require.NoError(t, err)
	assert.Equal(t, "u1", userID)
	assert.Equal(t, "test@example.com", userEmail)
	assert.Equal(t, "Test User", userName)

	// Insert an API key for the user
	var keyID, keyLabel string
	err = db.Pool.QueryRow(ctx,
		"INSERT INTO api_keys (id, user_id, label, key_hash) VALUES ('ak1', 'u1', 'My Key', 'hash_abc123') RETURNING id, label",
	).Scan(&keyID, &keyLabel)
	require.NoError(t, err)
	assert.Equal(t, "ak1", keyID)
	assert.Equal(t, "My Key", keyLabel)

	// Insert a team
	var teamID, teamName string
	err = db.Pool.QueryRow(ctx,
		"INSERT INTO teams (id, name, owner_id) VALUES ('t1', 'Test Team', 'u1') RETURNING id, name",
	).Scan(&teamID, &teamName)
	require.NoError(t, err)
	assert.Equal(t, "t1", teamID)
	assert.Equal(t, "Test Team", teamName)

	// Add user to team
	var memberID string
	err = db.Pool.QueryRow(ctx,
		"INSERT INTO team_members (id, team_id, user_id) VALUES ('tm1', 't1', 'u1') RETURNING id",
	).Scan(&memberID)
	require.NoError(t, err)
	assert.Equal(t, "tm1", memberID)

	// Create a session
	var sessionID, sessionStatus string
	err = db.Pool.QueryRow(ctx,
		"INSERT INTO sessions (id, user_id, team_id) VALUES ('s1', 'u1', 't1') RETURNING id, status",
	).Scan(&sessionID, &sessionStatus)
	require.NoError(t, err)
	assert.Equal(t, "s1", sessionID)
	assert.Equal(t, "active", sessionStatus)

	// Verify cascade: delete session first (no cascade on sessions.user_id)
	_, err = db.Pool.Exec(ctx, "DELETE FROM sessions WHERE id = 's1'")
	require.NoError(t, err)

	// Verify foreign key: deleting user should cascade to api_keys and team_members
	_, err = db.Pool.Exec(ctx, "DELETE FROM users WHERE id = 'u1'")
	require.NoError(t, err)

	// Verify cascade: api_key should be deleted
	var count int
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM api_keys WHERE id = 'ak1'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "api_key should be cascade-deleted with user")

	// Verify cascade: team_member should be deleted
	err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM team_members WHERE id = 'tm1'").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "team_member should be cascade-deleted with user")
}
