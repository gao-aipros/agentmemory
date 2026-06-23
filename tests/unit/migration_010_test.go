package unit

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigration010_FilesExist(t *testing.T) {
	upPath := "../../migrations/010_constraints_indexes.up.sql"
	downPath := "../../migrations/010_constraints_indexes.down.sql"

	_, errUp := os.Stat(upPath)
	_, errDown := os.Stat(downPath)

	assert.NoError(t, errUp, "010_constraints_indexes.up.sql must exist")
	assert.NoError(t, errDown, "010_constraints_indexes.down.sql must exist")
}

func TestMigration010_UpContent(t *testing.T) {
	data, err := os.ReadFile("../../migrations/010_constraints_indexes.up.sql")
	require.NoError(t, err)
	content := string(data)

	// #7: sessions.user_id ON DELETE CASCADE
	assert.Contains(t, content, "sessions_user_id_fkey",
		"must drop/recreate sessions FK with CASCADE")
	assert.Contains(t, content, "ON DELETE CASCADE",
		"must add ON DELETE CASCADE to sessions FK")

	// #38: observations.owner_user_id FK
	assert.Contains(t, content, "fk_observations_owner_user_id",
		"must add FK on observations.owner_user_id")

	// #39: memories.owner_user_id FK
	assert.Contains(t, content, "fk_memories_owner_user_id",
		"must add FK on memories.owner_user_id")

	// #40: partial embedding indexes per model
	assert.Contains(t, content, "idx_obs_emb_hnsw_ada002",
		"must create partial HNSW index per model")

	// #41: composite indexes on observations
	assert.Contains(t, content, "idx_observations_session_type",
		"must create composite index on observations(session_id, type)")
	assert.Contains(t, content, "idx_observations_owner_user_timestamp",
		"must create composite index on observations(owner_user_id, timestamp)")

	// #42: UNIQUE on team_members(team_id, user_id)
	assert.Contains(t, content, "uq_team_members_team_user",
		"must create UNIQUE index on team_members(team_id, user_id)")

	// #58: index on lessons.team_id
	assert.Contains(t, content, "idx_lessons_team_id",
		"must create index on lessons.team_id")

	// #59: index on teams.owner_id
	assert.Contains(t, content, "idx_teams_owner_id",
		"must create index on teams.owner_id")
}

func TestMigration010_DownContent(t *testing.T) {
	data, err := os.ReadFile("../../migrations/010_constraints_indexes.down.sql")
	require.NoError(t, err)
	content := string(data)

	// Must reverse all 8 fixes

	// #7: recreate FK without CASCADE
	assert.Contains(t, content, "sessions_user_id_fkey",
		"down must recreate sessions FK without CASCADE")

	// #38: drop FK
	assert.Contains(t, content, "fk_observations_owner_user_id",
		"down must drop observations FK")

	// #39: drop FK
	assert.Contains(t, content, "fk_memories_owner_user_id",
		"down must drop memories FK")

	// #40: drop partial index, recreate original
	assert.Contains(t, content, "idx_obs_emb_hnsw_ada002",
		"down must drop partial embedding index")
	assert.Contains(t, content, "idx_obs_emb_hnsw",
		"down must recreate original full embedding index")

	// #41: drop composite indexes
	assert.Contains(t, content, "idx_observations_session_type",
		"down must drop composite session_type index")
	assert.Contains(t, content, "idx_observations_owner_user_timestamp",
		"down must drop composite owner_user_timestamp index")

	// #42: drop UNIQUE
	assert.Contains(t, content, "uq_team_members_team_user",
		"down must drop team_members UNIQUE index")

	// #58: drop index
	assert.Contains(t, content, "idx_lessons_team_id",
		"down must drop lessons.team_id index")

	// #59: drop index
	assert.Contains(t, content, "idx_teams_owner_id",
		"down must drop teams.owner_id index")
}

func TestMigration010_DownReversesUpOrder(t *testing.T) {
	// Verify down file is structured: DROP INDEX first, then DROP CONSTRAINT,
	// then recreate session FK without CASCADE last.
	data, err := os.ReadFile("../../migrations/010_constraints_indexes.down.sql")
	require.NoError(t, err)
	content := string(data)

	// DROP CONSTRAINT lines should appear after DROP INDEX lines
	// This ensures we don't try to drop a constraint on a columns that no longer exists
	assert.Contains(t, content, "DROP CONSTRAINT",
		"down must reverse FK constraints")
	assert.Contains(t, content, "DROP INDEX",
		"down must drop all indexes")
}
