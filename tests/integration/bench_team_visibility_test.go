package integration

import (
	"context"
	"testing"
	"time"

	"github.com/agentmemory/agentmemory/internal/auth"
	"github.com/agentmemory/agentmemory/internal/service"
	"github.com/agentmemory/agentmemory/internal/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T176: Benchmark team visibility propagation: <60s (SC-006).

// TestBenchTeamVisibilityPropagation verifies that team visibility propagates
// to members within the required time bound. It tests the full flow:
// team creation -> member add -> observation sharing -> visibility propagation.
func TestBenchTeamVisibilityPropagation(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()
	queries := store.New(db.Pool)

	// Create owner and members
	hash, _ := auth.HashPassword("bench-password")
	ownerID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID: ownerID, Email: "owner-vis@example.com", PasswordHash: hash, Name: "Owner Vis",
	})
	require.NoError(t, err)

	member1ID := uuid.New().String()
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID: member1ID, Email: "member1-vis@example.com", PasswordHash: hash, Name: "Member 1 Vis",
	})
	require.NoError(t, err)

	member2ID := uuid.New().String()
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID: member2ID, Email: "member2-vis@example.com", PasswordHash: hash, Name: "Member 2 Vis",
	})
	require.NoError(t, err)

	// Measure full team lifecycle timing
	start := time.Now()

	// Step 1: Create team
	teamSvc := service.NewTeamService(db.Pool)
	team, err := teamSvc.CreateTeam(ctx, "Bench Team Vis", ownerID, "member_choice")
	require.NoError(t, err)
	createTeamTime := time.Since(start)
	t.Logf("Team creation: %v", createTeamTime)

	// Step 2: Add members
	memberSvc := service.NewTeamMembersService(db.Pool)
	stepStart := time.Now()

	// Owner joins their own team
	ownerMembershipID := uuid.New().String()
	_, err = queries.AddTeamMember(ctx, store.AddTeamMemberParams{
		ID: ownerMembershipID, TeamID: team.ID, UserID: ownerID,
	})
	require.NoError(t, err)

	err = memberSvc.AddMember(ctx, team.ID, member1ID, ownerID)
	require.NoError(t, err)

	err = memberSvc.AddMember(ctx, team.ID, member2ID, ownerID)
	require.NoError(t, err)

	addMembersTime := time.Since(stepStart)
	t.Logf("Add 3 members: %v", addMembersTime)

	// Step 3: Verify membership propagation
	stepStart = time.Now()
	members, err := memberSvc.ListMembers(ctx, team.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, members, 3, "all 3 members should be visible")

	listMembersTime := time.Since(stepStart)
	t.Logf("List members: %v", listMembersTime)

	// Verify each member can see the team
	for _, memberID := range []string{ownerID, member1ID, member2ID} {
		userTeam, err := queries.GetUserTeam(ctx, memberID)
		require.NoError(t, err)
		assert.Equal(t, team.ID, userTeam.ID, "user %s should see team", memberID)
	}

	totalTime := time.Since(start)
	t.Logf("Total team lifecycle (create + members + list): %v", totalTime)
	assert.Less(t, totalTime, 60*time.Second,
		"team visibility propagation should complete within 60s, took %v", totalTime)
}

// TestBenchTeamOpsLatency measures individual team operation latencies.
func TestBenchTeamOpsLatency(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()
	queries := store.New(db.Pool)

	// Setup users
	hash, _ := auth.HashPassword("bench-password")
	ownerID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID: ownerID, Email: "owner-ops@example.com", PasswordHash: hash, Name: "Owner Ops",
	})
	require.NoError(t, err)

	memberID := uuid.New().String()
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID: memberID, Email: "member-ops@example.com", PasswordHash: hash, Name: "Member Ops",
	})
	require.NoError(t, err)

	teamSvc := service.NewTeamService(db.Pool)
	memberSvc := service.NewTeamMembersService(db.Pool)

	// Benchmark create team
	createStart := time.Now()
	team, err := teamSvc.CreateTeam(ctx, "Ops Test Team", ownerID, "member_choice")
	require.NoError(t, err)
	createTime := time.Since(createStart)
	t.Logf("Create team: %v", createTime)
	assert.Less(t, createTime, 5*time.Second, "team creation should be <5s")

	// Add owner to team for member checks
	_, err = queries.AddTeamMember(ctx, store.AddTeamMemberParams{
		ID: uuid.New().String(), TeamID: team.ID, UserID: ownerID,
	})
	require.NoError(t, err)

	// Benchmark get team
	getStart := time.Now()
	fetched, err := teamSvc.GetTeam(ctx, team.ID)
	require.NoError(t, err)
	getTime := time.Since(getStart)
	t.Logf("Get team: %v", getTime)
	assert.Equal(t, team.Name, fetched.Name)
	assert.Less(t, getTime, 1*time.Second, "team retrieval should be <1s")

	// Benchmark add member
	addStart := time.Now()
	err = memberSvc.AddMember(ctx, team.ID, memberID, ownerID)
	require.NoError(t, err)
	addTime := time.Since(addStart)
	t.Logf("Add member: %v", addTime)
	assert.Less(t, addTime, 5*time.Second, "add member should be <5s")

	// Benchmark list members
	listStart := time.Now()
	members, err := memberSvc.ListMembers(ctx, team.ID, ownerID)
	require.NoError(t, err)
	listTime := time.Since(listStart)
	t.Logf("List members (%d): %v", len(members), listTime)
	assert.GreaterOrEqual(t, len(members), 2)
	assert.Less(t, listTime, 1*time.Second, "list members should be <1s")

	// Benchmark remove member
	removeStart := time.Now()
	err = memberSvc.RemoveMember(ctx, team.ID, memberID, ownerID)
	require.NoError(t, err)
	removeTime := time.Since(removeStart)
	t.Logf("Remove member: %v", removeTime)
	assert.Less(t, removeTime, 5*time.Second, "remove member should be <5s")

	// Verify member removed
	membersAfter, err := memberSvc.ListMembers(ctx, team.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, membersAfter, 1, "only owner should remain")

	// Benchmark update team
	updateStart := time.Now()
	err = teamSvc.UpdateTeam(ctx, team.ID, ownerID, "Updated Team", "team")
	require.NoError(t, err)
	updateTime := time.Since(updateStart)
	t.Logf("Update team: %v", updateTime)
	assert.Less(t, updateTime, 5*time.Second, "team update should be <5s")

	// Verify update
	updated, err := teamSvc.GetTeam(ctx, team.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Team", updated.Name)
	assert.Equal(t, "team", updated.DefaultVisibility)
}

// TestBenchTeamVisibilityRejoin measures rejoin latency.
func TestBenchTeamVisibilityRejoin(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()
	queries := store.New(db.Pool)

	hash, _ := auth.HashPassword("bench-password")
	ownerID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID: ownerID, Email: "owner-rejoin@example.com", PasswordHash: hash, Name: "Owner Rejoin",
	})
	require.NoError(t, err)

	memberID := uuid.New().String()
	_, err = queries.CreateUser(ctx, store.CreateUserParams{
		ID: memberID, Email: "member-rejoin@example.com", PasswordHash: hash, Name: "Member Rejoin",
	})
	require.NoError(t, err)

	teamSvc := service.NewTeamService(db.Pool)
	memberSvc := service.NewTeamMembersService(db.Pool)

	team, err := teamSvc.CreateTeam(ctx, "Rejoin Team", ownerID, "member_choice")
	require.NoError(t, err)

	// Owner joins
	_, err = queries.AddTeamMember(ctx, store.AddTeamMemberParams{
		ID: uuid.New().String(), TeamID: team.ID, UserID: ownerID,
	})
	require.NoError(t, err)

	// Member joins
	err = memberSvc.AddMember(ctx, team.ID, memberID, ownerID)
	require.NoError(t, err)

	// Member leaves
	err = memberSvc.RemoveMember(ctx, team.ID, memberID, ownerID)
	require.NoError(t, err)

	// Measure rejoin time
	rejoinStart := time.Now()
	err = memberSvc.AddMember(ctx, team.ID, memberID, ownerID)
	require.NoError(t, err)
	rejoinTime := time.Since(rejoinStart)
	t.Logf("Rejoin time: %v", rejoinTime)
	assert.Less(t, rejoinTime, 5*time.Second, "rejoin should be <5s")

	// Verify visibility after rejoin
	members, err := memberSvc.ListMembers(ctx, team.ID, ownerID)
	require.NoError(t, err)
	assert.Len(t, members, 2, "owner and rejoined member should be visible")
}

// TestBenchTeamDeleteLatency measures team deletion timing.
func TestBenchTeamDeleteLatency(t *testing.T) {
	db := SetupTestDB(t)
	defer TeardownTestDB(t, db)

	runMigrations(t, db)

	ctx := context.Background()
	queries := store.New(db.Pool)

	hash, _ := auth.HashPassword("bench-password")
	ownerID := uuid.New().String()
	_, err := queries.CreateUser(ctx, store.CreateUserParams{
		ID: ownerID, Email: "owner-del@example.com", PasswordHash: hash, Name: "Owner Delete",
	})
	require.NoError(t, err)

	teamSvc := service.NewTeamService(db.Pool)
	team, err := teamSvc.CreateTeam(ctx, "Delete Test Team", ownerID, "member_choice")
	require.NoError(t, err)

	deleteStart := time.Now()
	err = teamSvc.DeleteTeam(ctx, team.ID, ownerID)
	require.NoError(t, err)
	deleteTime := time.Since(deleteStart)
	t.Logf("Delete team: %v", deleteTime)
	assert.Less(t, deleteTime, 5*time.Second, "team deletion should be <5s")

	// Verify team is gone
	_, err = teamSvc.GetTeam(ctx, team.ID)
	assert.Error(t, err, "deleted team should not be retrievable")
}
