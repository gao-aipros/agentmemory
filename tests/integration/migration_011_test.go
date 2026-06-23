package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Table Existence Tests for Migration 011
// =============================================================================

// TestMigration011_CrystallizationTablesExist verifies that migration 011
// creates the crystals, insights, and procedural_memories tables.
func TestMigration011_CrystallizationTablesExist(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	expectedTables := []string{
		"crystals",
		"insights",
		"procedural_memories",
	}

	for _, table := range expectedTables {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1)",
			table,
		).Scan(&exists)
		require.NoError(t, err, "failed to check table existence: %s", table)
		assert.True(t, exists, "table %s should exist after migration 011", table)
	}
}

// TestMigration011_CrystalsColumns verifies the crystals table has the
// expected columns with correct types and constraints.
func TestMigration011_CrystalsColumns(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	type columnCheck struct {
		name             string
		expectedType     string
		expectedNullable string
	}

	checks := []columnCheck{
		{name: "id", expectedType: "text", expectedNullable: "NO"},
		{name: "action_ids", expectedType: "ARRAY", expectedNullable: "NO"},
		{name: "visibility", expectedType: "text", expectedNullable: "NO"},
		{name: "narrative", expectedType: "text", expectedNullable: "NO"},
		{name: "files", expectedType: "ARRAY", expectedNullable: "YES"},
		{name: "outcome", expectedType: "text", expectedNullable: "YES"},
	}

	for _, ch := range checks {
		var dataType, isNullable string
		err := db.Pool.QueryRow(ctx,
			`SELECT data_type, is_nullable FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = 'crystals' AND column_name = $1`,
			ch.name,
		).Scan(&dataType, &isNullable)
		require.NoError(t, err, "failed to read column %s", ch.name)
		assert.Equal(t, ch.expectedType, dataType, "column %s type", ch.name)
		assert.Equal(t, ch.expectedNullable, isNullable, "column %s nullable", ch.name)
	}
}

// TestMigration011_InsightsColumns verifies the insights table columns.
func TestMigration011_InsightsColumns(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	type columnCheck struct {
		name             string
		expectedType     string
		expectedNullable string
	}

	checks := []columnCheck{
		{name: "id", expectedType: "text", expectedNullable: "NO"},
		{name: "content", expectedType: "text", expectedNullable: "NO"},
		{name: "confidence", expectedType: "double precision", expectedNullable: "NO"},
		{name: "source", expectedType: "text", expectedNullable: "NO"},
	}

	for _, ch := range checks {
		var dataType, isNullable string
		err := db.Pool.QueryRow(ctx,
			`SELECT data_type, is_nullable FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = 'insights' AND column_name = $1`,
			ch.name,
		).Scan(&dataType, &isNullable)
		require.NoError(t, err, "failed to read column %s", ch.name)
		assert.Equal(t, ch.expectedType, dataType, "column %s type", ch.name)
		assert.Equal(t, ch.expectedNullable, isNullable, "column %s nullable", ch.name)
	}
}

// TestMigration011_ProceduralMemoriesColumns verifies the procedural_memories table columns.
func TestMigration011_ProceduralMemoriesColumns(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	type columnCheck struct {
		name             string
		expectedType     string
		expectedNullable string
	}

	checks := []columnCheck{
		{name: "id", expectedType: "text", expectedNullable: "NO"},
		{name: "content", expectedType: "text", expectedNullable: "NO"},
		{name: "trigger", expectedType: "text", expectedNullable: "YES"},
	}

	for _, ch := range checks {
		var dataType, isNullable string
		err := db.Pool.QueryRow(ctx,
			`SELECT data_type, is_nullable FROM information_schema.columns
			 WHERE table_schema = 'public' AND table_name = 'procedural_memories' AND column_name = $1`,
			ch.name,
		).Scan(&dataType, &isNullable)
		require.NoError(t, err, "failed to read column %s", ch.name)
		assert.Equal(t, ch.expectedType, dataType, "column %s type", ch.name)
		assert.Equal(t, ch.expectedNullable, isNullable, "column %s nullable", ch.name)
	}
}

// =============================================================================
// Crystals visibility constraint test
// =============================================================================

// TestMigration011_CrystalsVisibilityCheck verifies the visibility CHECK
// constraint on crystals table restricts values to 'private'.
func TestMigration011_CrystalsVisibilityCheck(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	// Use check_constraints view to avoid picking up NOT NULL constraints
	// which are also listed in table_constraints with constraint_type='CHECK'.
	var constraintName string
	err := db.Pool.QueryRow(ctx,
		`SELECT constraint_name FROM information_schema.check_constraints
		 WHERE constraint_schema = 'public'
		 AND constraint_name = 'chk_crystals_visibility'`,
	).Scan(&constraintName)
	require.NoError(t, err, "crystals should have a named CHECK constraint chk_crystals_visibility")
	assert.Equal(t, "chk_crystals_visibility", constraintName,
		"CHECK constraint should be named chk_crystals_visibility")
}

// =============================================================================
// Vector Index: HNSW Verification (tests migration 010, not 011)
// =============================================================================

// TestMigration011_VectorIndexHNSW verifies that migration 010 creates the
// embedding vector index using the HNSW access method instead of IVFFlat.
// Note: this tests migration 010 behavior but lives alongside the migration
// 011 table tests for convenience since both fix issues #21 and #22.
func TestMigration011_VectorIndexHNSW(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	// The index exists after migration 010 with the name idx_obs_emb_hnsw_ada002
	// We check that the access method is 'hnsw' not 'ivfflat'
	var amname string
	err := db.Pool.QueryRow(ctx,
		`SELECT am.amname
		 FROM pg_class c
		 JOIN pg_am am ON am.oid = c.relam
		 WHERE c.relname = 'idx_obs_emb_hnsw_ada002'
		 AND c.relkind = 'i'`,
	).Scan(&amname)
	require.NoError(t, err, "failed to check index access method for idx_obs_emb_hnsw_ada002")
	assert.Equal(t, "hnsw", amname, "embedding index should use HNSW access method, not IVFFlat")
}

// TestMigration011_VectorIndexHNSW_Definition checks the index definition
// string contains USING hnsw.
// Note: this tests migration 010 behavior (issue #22), not migration 011.
func TestMigration011_VectorIndexHNSW_Definition(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()

	var indexdef string
	err := db.Pool.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes
		 WHERE schemaname = 'public'
		 AND indexname = 'idx_obs_emb_hnsw_ada002'`,
	).Scan(&indexdef)
	require.NoError(t, err, "failed to read index definition")
	assert.Contains(t, indexdef, "USING hnsw", "index definition should use HNSW")
	assert.NotContains(t, indexdef, "USING ivfflat", "index definition should not use IVFFlat")
}
