package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			t.Fatal("cannot find module root (go.mod) walking up from working directory")
		}
		dir = parent
	}
}

func readMigrationFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read %s", path)
	return string(content)
}

func TestSchema_FilesExist(t *testing.T) {
	migrationsDir := findMigrationsDir(t)

	require.FileExists(t, filepath.Join(migrationsDir, "001_initial_schema.up.sql"),
		"001_initial_schema.up.sql must exist")
	require.FileExists(t, filepath.Join(migrationsDir, "001_initial_schema.down.sql"),
		"001_initial_schema.down.sql must exist")
}

func TestSchema_OnlyOneMigration(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	entries, err := os.ReadDir(migrationsDir)
	require.NoError(t, err)

	var upCount int
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upCount++
		}
	}
	assert.Equal(t, 1, upCount, "there should be exactly one .up.sql migration file (consolidated)")
}

func TestSchema_UpContent(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	content := readMigrationFile(t, filepath.Join(migrationsDir, "001_initial_schema.up.sql"))

	// All expected tables
	tables := []string{
		"users", "api_keys",
		"teams", "team_members",
		"sessions",
		"observations", "observation_embeddings",
		"compressed_observations", "compressed_embeddings",
		"session_summaries",
		"memories", "lessons", "lesson_reinforcements",
		"graph_nodes", "graph_edges",
		"crystals", "insights", "procedural_memories",
	}
	for _, tbl := range tables {
		assert.Contains(t, content, "CREATE TABLE IF NOT EXISTS "+tbl,
			"schema must create table %s", tbl)
	}

	// FKs in final form
	assert.Contains(t, content, "REFERENCES sessions(id) ON DELETE CASCADE",
		"observations.session_id must cascade")
	assert.Contains(t, content, "REFERENCES sessions(id) ON DELETE CASCADE UNIQUE",
		"session_summaries.session_id must cascade + unique")
	assert.Contains(t, content, "REFERENCES users(id) ON DELETE SET NULL",
		"sessions.user_id must SET NULL on delete")

	// Key indexes
	assert.Contains(t, content, "USING hnsw",
		"vector index must use HNSW")
	assert.Contains(t, content, "bm25",
		"full-text search must use BM25")
	assert.Contains(t, content, "uq_team_members_team_user",
		"team_members must have unique constraint")

	// Check constraints
	assert.Contains(t, content, "chk_crystals_visibility",
		"crystals must have named CHECK constraint")

	// Functions
	assert.Contains(t, content, "CREATE OR REPLACE FUNCTION bm25_search",
		"must define bm25_search function")
	assert.Contains(t, content, "CREATE OR REPLACE FUNCTION hybrid_search",
		"must define hybrid_search function")
}

func TestSchema_DownContent(t *testing.T) {
	migrationsDir := findMigrationsDir(t)
	content := readMigrationFile(t, filepath.Join(migrationsDir, "001_initial_schema.down.sql"))

	// All tables dropped in correct order
	for _, tbl := range []string{"users", "sessions", "observations", "memories", "lessons"} {
		assert.Contains(t, content, "DROP TABLE IF EXISTS "+tbl,
			"down must drop %s", tbl)
	}

	// Functions dropped
	assert.Contains(t, content, "DROP FUNCTION IF EXISTS hybrid_search",
		"down must drop hybrid_search function")
	assert.Contains(t, content, "DROP FUNCTION IF EXISTS bm25_search",
		"down must drop bm25_search function")
}
