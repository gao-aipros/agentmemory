package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLessonsByTeam_Limit(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	ctx := context.Background()
	runMigrations(t, db)

	userID := uuid.New().String()
	_, err := db.Pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, $2, $3, $4)`,
		userID, "limit-lessons@example.com", "hash", "Limit Lessons User")
	require.NoError(t, err)

	teamID := uuid.New().String()
	_, err = db.Pool.Exec(ctx, `INSERT INTO teams (id, name, owner_id, default_visibility) VALUES ($1, $2, $3, 'member_choice')`,
		teamID, "Lessons Team", userID)
	require.NoError(t, err)

	queries := store.New(db.Pool)

	// Insert 5 lessons
	for i := 0; i < 5; i++ {
		_, err := queries.InsertLesson(ctx, store.InsertLessonParams{
			ID:         uuid.New().String(),
			TeamID:     &teamID,
			Visibility: "team",
			Content:    fmt.Sprintf("lesson-%d", i),
		})
		require.NoError(t, err)
	}

	// Query with limit=3 — should return exactly 3
	lessons, err := queries.ListLessonsByTeam(ctx, store.ListLessonsByTeamParams{
		TeamID: &teamID,
		Limit:  3,
	})
	require.NoError(t, err)
	assert.Len(t, lessons, 3)
}
